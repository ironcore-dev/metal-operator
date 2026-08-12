// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/serverevents"
	"github.com/ironcore-dev/metal-operator/pkg/bmcutils"

	"github.com/stmcginnis/gofish/schemas"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	BMCFinalizer = "metal.ironcore.dev/bmc"

	bmcUserResetMessage = "BMC reset initiated by user. Waiting for it to come back online."
	bmcAutoResetMessage = "BMC reset initiated automatically after repeated connection failures. Waiting for it to come back online."
)

// BMCReconciler reconciles a BMC object
type BMCReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	DefaultProtocol    metalv1alpha1.ProtocolScheme
	SkipCertValidation bool
	// BMCFailureResetDelay defines the duration after which a BMC will be reset upon repeated connection failures.
	BMCFailureResetDelay time.Duration
	BMCOptions           bmc.Options
	ManagerNamespace     string
	EventURL             string
	// BMCResetWaitTime defines the duration to wait after a BMC reset before attempting reconciliation again.
	BMCResetWaitTime time.Duration
	// BMCClientRetryInterval defines the duration to requeue reconciliation after a BMC client error/reset/unavailablility.
	BMCClientRetryInterval time.Duration
	// DNSRecordTemplatePath is the path to the file containing the DNSRecord template.
	DNSRecordTemplate string
	Conditions        *conditionutils.Accessor

	// SSHResetTimeout defines the timeout for SSH reset operations (dial + command execution).
	SSHResetTimeout time.Duration
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BMCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, req.NamespacedName, bmcObj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, bmcObj)
}

func (r *BMCReconciler) reconcileExists(ctx context.Context, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	if !bmcObj.DeletionTimestamp.IsZero() {
		return r.delete(ctx, bmcObj)
	}
	return r.reconcile(ctx, bmcObj)
}

func (r *BMCReconciler) delete(ctx context.Context, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Deleting BMC")
	if bmcObj.Spec.BMCSettingRef != nil {
		bmcSettings := &metalv1alpha1.BMCSettings{}
		if err := r.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.BMCSettingRef.Name}, bmcSettings); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get BMCSettings for BMC: %w", err)
		}
		if err := r.Delete(ctx, bmcSettings); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete referred BMCSettings. %w", err)
		}
	}

	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.DefaultProtocol, r.SkipCertValidation, r.BMCOptions)
	if err == nil {
		defer bmcClient.Logout()
		if err := r.deleteEventSubscription(ctx, bmcClient, bmcObj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete event subscriptions: %w", err)
		}
	}

	if _, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, bmcObj, BMCFinalizer); err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Deleted BMC")
	return ctrl.Result{}, nil
}

func (r *BMCReconciler) reconcile(ctx context.Context, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling BMC")
	if shouldIgnoreReconciliation(bmcObj) {
		log.V(1).Info("Skipped BMC reconciliation")
		return ctrl.Result{}, nil
	}
	// SSH reset takes priority — process it even during the waitForBMCReset window so the
	// reset actually runs instead of being deferred until the window expires.
	if r.hasSSHResetAnnotation(bmcObj) {
		log.V(1).Info("SSH reset annotation detected on unresponsive BMC", "BMC", bmcObj.Name)
		bmcBase := bmcObj.DeepCopy()
		metautils.DeleteAnnotation(bmcObj, metalv1alpha1.OperationAnnotation)
		if err := r.Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove SSH reset annotation: %w", err)
		}
		if err := r.resetBMCViaSSH(ctx, bmcObj.Name); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reset BMC via SSH: %w", err)
		}
		log.V(1).Info("BMC SSH reset completed", "BMC", bmcObj.Name)
		return ctrl.Result{RequeueAfter: r.BMCClientRetryInterval}, nil
	}
	if r.waitForBMCReset(bmcObj, r.BMCResetWaitTime) {
		log.V(1).Info("Skipped BMC reconciliation while waiting for BMC reset to complete")
		if err := r.patchBMCStatePending(ctx, bmcObj); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{
			RequeueAfter: r.BMCClientRetryInterval,
		}, nil
	}
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.DefaultProtocol, r.SkipCertValidation, r.BMCOptions, bmcutils.BMCConnectivityCheckOption)
	if err != nil {
		if r.shouldResetBMC(bmcObj) {
			log.V(1).Info("BMC needs reset, resetting", "BMC", bmcObj.Name)
			if err := r.resetBMC(ctx, bmcObj, bmcClient, nil, ReasonAutoReset, bmcAutoResetMessage); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reset BMC: %w", err)
			}
			log.V(1).Info("BMC reset initiated", "BMC", bmcObj.Name)
			return ctrl.Result{
				RequeueAfter: r.BMCClientRetryInterval,
			}, nil
		}
		// User-requested reset annotation while BMC is offline — attempt Redfish (will 5xx → schedule SSH).
		if r.hasGracefulRestartAnnotation(bmcObj) {
			log.V(1).Info("Reset annotation detected on unresponsive BMC, attempting reset", "BMC", bmcObj.Name)
			connErr := err // preserve before inner scopes shadow it
			bmcBase := bmcObj.DeepCopy()
			metautils.DeleteAnnotation(bmcObj, metalv1alpha1.OperationAnnotation)
			if err := r.Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove operation annotation: %w", err)
			}
			if err := r.resetBMC(ctx, bmcObj, bmcClient, connErr, ReasonUserReset, bmcUserResetMessage); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reset BMC: %w", err)
			}
			return ctrl.Result{RequeueAfter: r.BMCClientRetryInterval}, nil
		}
		return ctrl.Result{RequeueAfter: r.BMCClientRetryInterval}, r.updateReadyConditionOnBMCFailure(ctx, bmcObj, err)
	}
	defer bmcClient.Logout()

	// if BMC reset was issued and is successful, ensure to remove previous reset annotation
	if modified, err := r.handlePreviousBMCResetAnnotations(ctx, bmcObj); err != nil || modified {
		return ctrl.Result{}, err
	}

	if modified, err := r.handleAnnotationOperations(ctx, bmcObj, bmcClient); err != nil || modified {
		return ctrl.Result{}, err
	}

	if err := r.updateConditions(ctx, bmcObj, true, ConditionReady, corev1.ConditionTrue, ReasonConnected, "BMC is connected"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set BMC connected condition: %w", err)
	}
	if err := r.updateConditions(ctx, bmcObj, false, ConditionReset, corev1.ConditionFalse, ReasonResetComplete, "BMC reset is complete"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set BMC reset complete condition: %w", err)
	}

	if err := r.updateBMCStatusDetails(ctx, bmcClient, bmcObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update BMC status: %w", err)
	}
	log.V(1).Info("Updated BMC status", "State", bmcObj.Status.State)

	// Create DNS record for the bmc if template is configured
	if r.ManagerNamespace != "" && r.DNSRecordTemplate != "" {
		if err := r.createDNSRecord(ctx, bmcObj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create DNS record for BMC %s: %w", bmcObj.Name, err)
		}
	}

	if err := r.discoverServers(ctx, bmcClient, bmcObj); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to discover servers: %w", err)
	}
	log.V(1).Info("Discovered servers")

	if modified, err := r.handleEventSubscriptions(ctx, bmcClient, bmcObj); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Reconciled BMC")
	return ctrl.Result{}, nil
}

func (r *BMCReconciler) updateBMCStatusDetails(ctx context.Context, bmcClient bmc.BMC, bmcObj *metalv1alpha1.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	var (
		ip         metalv1alpha1.IP
		macAddress string
	)
	if bmcObj.Spec.EndpointRef != nil {
		endpoint := &metalv1alpha1.Endpoint{}
		if err := r.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.EndpointRef.Name}, endpoint); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get Endpoints for BMC: %w", err)
		}
		ip = endpoint.Spec.IP
		macAddress = endpoint.Spec.MACAddress
		log.V(1).Info("Got Endpoints for BMC", "Endpoints", endpoint.Name)
	}

	if bmcObj.Spec.Endpoint != nil {
		ip = bmcObj.Spec.Endpoint.IP
		macAddress = bmcObj.Spec.Endpoint.MACAddress
	}

	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.IP = ip
	bmcObj.Status.MACAddress = macAddress
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to patch IP and MAC address status: %w", err)
	}

	manager, err := bmcClient.GetManager(bmcObj.Spec.BMCUUID)
	if err != nil {
		return fmt.Errorf("failed to get manager details for BMC %s: %w", bmcObj.Name, err)
	}
	// GetManager returns (nil, err) on failure, so manager is non-nil here.
	// parse time to metav1.Time: ISO 8601 format
	lastResetTime := &metav1.Time{}
	if manager.LastResetTime != "" {
		t, err := time.Parse(time.RFC3339, manager.LastResetTime)
		if err == nil {
			lastResetTime = &metav1.Time{Time: t}
		}
	}
	bmcBase = bmcObj.DeepCopy()
	bmcObj.Status.Manufacturer = manager.Manufacturer
	bmcObj.Status.State = metalv1alpha1.BMCState(manager.Status.State)
	// Set power state, or unknown if not available from BMC
	if manager.PowerState != "" {
		bmcObj.Status.PowerState = metalv1alpha1.BMCPowerState(manager.PowerState)
	} else {
		bmcObj.Status.PowerState = metalv1alpha1.UnknownPowerState
		log.V(1).Info("Power state not reported by BMC, setting to unknown", "BMC", bmcObj.Name)
	}
	bmcObj.Status.FirmwareVersion = manager.FirmwareVersion
	bmcObj.Status.SerialNumber = manager.SerialNumber
	bmcObj.Status.SKU = manager.PartNumber
	bmcObj.Status.Model = manager.Model
	bmcObj.Status.LastResetTime = lastResetTime
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to patch manager details for BMC %s: %w", bmcObj.Name, err)
	}
	return nil
}

func (r *BMCReconciler) discoverServers(ctx context.Context, bmcClient bmc.BMC, bmcObj *metalv1alpha1.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	servers, err := bmcClient.GetSystems(ctx)
	if err != nil {
		return fmt.Errorf("failed to get servers from BMC %s: %w", bmcObj.Name, err)
	}
	var errs []error
	for i, s := range servers {
		server := &metalv1alpha1.Server{}
		server.Name = bmcutils.GetServerNameFromBMCandIndex(i, bmcObj)
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, server, func() error {
			metautils.SetLabels(server, bmcObj.Labels)
			server.Spec.SystemUUID = strings.ToLower(s.UUID)
			server.Spec.SystemURI = s.URI
			server.Spec.BMCRef = &corev1.LocalObjectReference{Name: bmcObj.Name}
			return controllerutil.SetControllerReference(bmcObj, server, r.Scheme)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create or patch server %s: %w", server.Name, err))
			continue
		}
		switch opResult {
		case controllerutil.OperationResultCreated:
			log.V(1).Info("Created Server", "Server", server.Name)
		case controllerutil.OperationResultUpdated:
			log.V(1).Info("Updated Server", "Server", server.Name)
		default:
			log.V(1).Info("Server already up to date", "Server", server.Name)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred during server discovery: %w", errors.Join(errs...))
	}
	return nil
}

// DNSRecordTemplateData contains the data used to render the DNS record YAML template
type DNSRecordTemplateData struct {
	Name      string
	Namespace string
	metalv1alpha1.BMCSpec
	metalv1alpha1.BMCStatus
	Labels map[string]string
}

// createDNSRecord creates a DNS record resource from a YAML template loaded from ConfigMap
func (r *BMCReconciler) createDNSRecord(ctx context.Context, bmcObj *metalv1alpha1.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	templateData := DNSRecordTemplateData{
		Namespace: r.ManagerNamespace,
		Name:      bmcObj.Name,
		BMCSpec:   bmcObj.Spec,
		BMCStatus: bmcObj.Status,
		Labels:    bmcObj.Labels,
	}
	tmpl, err := template.New("dnsRecord").
		Option("missingkey=error").
		Parse(r.DNSRecordTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse DNS record template: %w", err)
	}

	var renderedYAML bytes.Buffer
	if err := tmpl.Execute(&renderedYAML, templateData); err != nil {
		return fmt.Errorf("failed to render DNS record template: %w", err)
	}
	dnsRecord := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(renderedYAML.Bytes(), dnsRecord); err != nil {
		return fmt.Errorf("failed to unmarshal DNS record YAML: %w", err)
	}

	gvk := dnsRecord.GroupVersionKind()
	if gvk.Version == "" || gvk.Kind == "" {
		return fmt.Errorf("template is missing apiVersion or kind")
	}
	if dnsRecord.GetName() == "" {
		return fmt.Errorf("DNS record template must specify a name")
	}
	if dnsRecord.GetNamespace() == "" {
		dnsRecord.SetNamespace(r.ManagerNamespace)
	}

	if err := controllerutil.SetControllerReference(bmcObj, dnsRecord, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	dnsRecordApply := client.ApplyConfigurationFromUnstructured(dnsRecord)
	if err := r.Apply(ctx, dnsRecordApply, fieldOwner, client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to apply DNS record: %w", err)
	}

	log.Info("Created or patched DNS record", "RecordName", dnsRecord.GetName())
	return nil
}

func (r *BMCReconciler) handleAnnotationOperations(ctx context.Context, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	operation, ok := bmcObj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !ok {
		return false, nil
	}
	var value schemas.ResetType
	if value, ok = metalv1alpha1.AnnotationToRedfishMapping[operation]; !ok {
		log.V(1).Info("Unknown operation annotation, ignoring", "Operation", operation, "Supported Operations", schemas.GracefulRestartResetType)
		return false, nil
	}
	switch value {
	case schemas.GracefulRestartResetType:
		log.V(1).Info("Handling operation", "Operation", operation, "RedfishResetType", value)
		if err := r.resetBMC(ctx, bmcObj, bmcClient, nil, ReasonUserReset, bmcUserResetMessage); err != nil {
			return false, fmt.Errorf("failed to reset BMC: %w", err)
		}
		log.Info("Handled operation", "Operation", operation)
	default:
		log.V(1).Info("Unsupported operation annotation", "Operation", operation, "RedfishResetType", value)
		return false, nil
	}
	bmcBase := bmcObj.DeepCopy()
	metautils.DeleteAnnotation(bmcObj, metalv1alpha1.OperationAnnotation)
	if err := r.Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return false, fmt.Errorf("failed to remove operation annotation: %w", err)
	}
	log.V(1).Info("Removed operation annotation", "Operation", operation)
	return true, nil
}

func (r *BMCReconciler) updateReadyConditionOnBMCFailure(ctx context.Context, bmcObj *metalv1alpha1.BMC, err error) error {
	httpErr := &schemas.Error{}
	if errors.As(err, &httpErr) {
		// only handle 5xx errors
		switch httpErr.HTTPReturnedStatusCode {
		case 401:
			// Unauthorized error, likely due to bad credentials
			if err := r.updateConditions(ctx, bmcObj, true, ConditionReady, corev1.ConditionFalse, ReasonAuthenticationFailed, "BMC credentials are invalid"); err != nil {
				return fmt.Errorf("failed to set BMC unauthorized condition: %w", err)
			}

		case 500:
			// Internal Server Error, might be transient
			if err := r.updateConditions(ctx, bmcObj, true, ConditionReady, corev1.ConditionFalse, ReasonInternalError, "BMC internal server error"); err != nil {
				return fmt.Errorf("failed to set BMC internal server error condition: %w", err)
			}
		case 503:
			// Service Unavailable, might be transient
			if err := r.updateConditions(ctx, bmcObj, true, ConditionReady, corev1.ConditionFalse, ReasonConnectionFailed, "BMC service unavailable"); err != nil {
				return fmt.Errorf("failed to set BMC service unavailable condition: %w", err)
			}
		default:
			if err := r.updateConditions(ctx, bmcObj, true, ConditionReady, corev1.ConditionFalse, ReasonUnknownError, fmt.Sprintf("BMC connection error: %v", err)); err != nil {
				return fmt.Errorf("failed to set BMC error condition: %w", err)
			}
		}
	} else {
		if err := r.updateConditions(ctx, bmcObj, true, ConditionReady, corev1.ConditionFalse, ReasonUnknownError, fmt.Sprintf("BMC connection error: %v", err)); err != nil {
			return fmt.Errorf("failed to set BMC error condition: %w", err)
		}
	}
	return err
}

func (r *BMCReconciler) waitForBMCReset(bmcObj *metalv1alpha1.BMC, delay time.Duration) bool {
	condition := &metav1.Condition{}
	found, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, ConditionReset, condition)
	if err != nil || !found {
		return false
	}
	if condition.Status == metav1.ConditionTrue {
		// give bmc some time to start the reset process
		if time.Since(condition.LastTransitionTime.Time) < delay {
			return true
		}
	}
	return false
}

func (r *BMCReconciler) handlePreviousBMCResetAnnotations(ctx context.Context, bmcObj *metalv1alpha1.BMC) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	condition := &metav1.Condition{}
	found, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, ConditionReset, condition)
	if err != nil || !found {
		return false, nil
	}
	if condition.Status == metav1.ConditionTrue {
		if operation, ok := bmcObj.GetAnnotations()[metalv1alpha1.OperationAnnotation]; ok &&
			(operation == metalv1alpha1.GracefulRestartBMC || operation == metalv1alpha1.ForceSSHResetBMC) {
			bmcBase := bmcObj.DeepCopy()
			metautils.DeleteAnnotation(bmcObj, metalv1alpha1.OperationAnnotation)
			if err := r.Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
				return false, fmt.Errorf("failed to remove operation annotation from previous reset: %w", err)
			}
			log.V(1).Info("Removed operation annotation from previous reset", "Operation", operation)
			return true, nil
		}
	}
	return false, nil
}

func (r *BMCReconciler) shouldResetBMC(bmcObj *metalv1alpha1.BMC) bool {
	if r.BMCFailureResetDelay == 0 {
		return false
	}
	bmcResetCondition := &metav1.Condition{}
	found, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, ConditionReset, bmcResetCondition)
	if err != nil || (found && bmcResetCondition.Status == metav1.ConditionTrue) {
		return false
	}
	readyCondition := &metav1.Condition{}
	found, err = r.Conditions.FindSlice(bmcObj.Status.Conditions, ConditionReady, readyCondition)
	if err != nil || !found {
		return false
	}
	if readyCondition.Status == metav1.ConditionFalse && (readyCondition.Reason == ReasonInternalError || readyCondition.Reason == ReasonConnectionFailed) {
		if time.Since(readyCondition.LastTransitionTime.Time) > r.BMCFailureResetDelay {
			return true
		}
	}
	return false
}

func (r *BMCReconciler) patchBMCStatePending(ctx context.Context, bmcObj *metalv1alpha1.BMC) error {
	if bmcObj.Status.State == metalv1alpha1.BMCStatePending {
		return nil
	}
	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.State = metalv1alpha1.BMCStatePending
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to patch BMC state to Pending: %w", err)
	}
	return nil
}

func (r *BMCReconciler) resetBMC(ctx context.Context, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC, clientErr error, reason, message string) error {
	log := ctrl.LoggerFrom(ctx)
	if err := r.updateConditions(ctx, bmcObj, true, ConditionReset, corev1.ConditionTrue, reason, message); err != nil {
		return fmt.Errorf("failed to set BMC resetting condition: %w", err)
	}
	if bmcClient == nil {
		// No Redfish client available. If the connection error was a 5xx (e.g. 503
		// from the BMC during gofish connect), schedule an SSH retry.
		if clientErr != nil {
			if httpErr, ok := errors.AsType[*schemas.Error](clientErr); ok &&
				httpErr.HTTPReturnedStatusCode >= 500 && httpErr.HTTPReturnedStatusCode < 600 {
				bmcBase := bmcObj.DeepCopy()
				metautils.SetAnnotation(bmcObj, metalv1alpha1.OperationAnnotation, metalv1alpha1.ForceSSHResetBMC)
				if patchErr := r.Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); patchErr != nil {
					return fmt.Errorf("failed to set SSH reset annotation: %w", patchErr)
				}
				log.Info("Scheduled SSH-based BMC reset due to connection 5xx error", "BMC", bmcObj.Name)
				return r.patchBMCStatePending(ctx, bmcObj)
			}
		}
		return errors.Join(
			r.patchBMCStatePending(ctx, bmcObj),
			fmt.Errorf("could not reset BMC %s: no client connection", bmcObj.Name),
		)
	}
	if err := bmcClient.ResetManager(ctx, bmcObj.Spec.BMCUUID, schemas.GracefulRestartResetType); err == nil {
		log.Info("Successfully reset BMC via Redfish", "BMC", bmcObj.Name)
		return r.patchBMCStatePending(ctx, bmcObj)
	} else if httpErr, ok := errors.AsType[*schemas.Error](err); ok {
		// only retryable on 5xx; anything else is a permanent failure for this attempt
		if httpErr.HTTPReturnedStatusCode < 500 || httpErr.HTTPReturnedStatusCode >= 600 {
			return errors.Join(r.patchBMCStatePending(ctx, bmcObj), fmt.Errorf("could not reset BMC: %w", err))
		}
		// 5xx — schedule SSH reset via annotation so the next reconcile handles it
		// in-cluster state drives the retry; no separate goroutine needed.
		bmcBase := bmcObj.DeepCopy()
		metautils.SetAnnotation(bmcObj, metalv1alpha1.OperationAnnotation, metalv1alpha1.ForceSSHResetBMC)
		if patchErr := r.Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); patchErr != nil {
			return fmt.Errorf("failed to set SSH reset annotation: %w", patchErr)
		}
		log.Info("Scheduled SSH-based BMC reset due to Redfish 5xx error", "BMC", bmcObj.Name)
		return r.patchBMCStatePending(ctx, bmcObj)
	} else {
		return fmt.Errorf("could not reset BMC, unknown error: %w", err)
	}
}

func (r *BMCReconciler) hasSSHResetAnnotation(bmcObj *metalv1alpha1.BMC) bool {
	operation, ok := bmcObj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	return ok && operation == metalv1alpha1.ForceSSHResetBMC
}

func (r *BMCReconciler) hasGracefulRestartAnnotation(bmcObj *metalv1alpha1.BMC) bool {
	operation, ok := bmcObj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	return ok && operation == metalv1alpha1.GracefulRestartBMC
}

func (r *BMCReconciler) resetBMCViaSSH(ctx context.Context, bmcName string) error {
	log := ctrl.LoggerFrom(ctx).WithValues("BMC", bmcName)
	log.V(1).Info("Starting SSH-based BMC reset")

	currentBMC := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, client.ObjectKey{Name: bmcName}, currentBMC); err != nil {
		return fmt.Errorf("failed to fetch BMC object for SSH reset: %w", err)
	}
	address, err := bmcutils.GetBMCAddressForBMC(ctx, r.Client, currentBMC)
	if err != nil {
		_ = r.updateConditions(ctx, currentBMC, true, ConditionReset, corev1.ConditionFalse, ReasonConnectionFailed, fmt.Sprintf("Failed to get BMC address: %v", err))
		return fmt.Errorf("failed to get BMC address for SSH reset: %w", err)
	}
	manufacturer := currentBMC.Status.Manufacturer
	if manufacturer == "" {
		log.V(1).Info("BMC manufacturer not available, attempting to get manufacturer from Server")
		serverList := &metalv1alpha1.ServerList{}
		if err := r.List(ctx, serverList, client.MatchingFields{bmcRefField: currentBMC.Name}); err != nil {
			log.Error(err, "Failed to list Servers for BMC to get manufacturer fallback")
		} else if len(serverList.Items) > 0 && serverList.Items[0].Status.Manufacturer != "" {
			manufacturer = serverList.Items[0].Status.Manufacturer
			log.V(1).Info("Using manufacturer from Server as fallback", "manufacturer", manufacturer, "server", serverList.Items[0].Name)
		}
	}
	if manufacturer == "" {
		_ = r.updateConditions(ctx, currentBMC, true, ConditionReset, corev1.ConditionFalse, ReasonInternalError, "BMC manufacturer not available")
		return fmt.Errorf("BMC manufacturer not available for SSH reset")
	}
	username, password, err := bmcutils.GetBMCCredentialsForBMCSecretName(ctx, r.Client, currentBMC.Spec.BMCSecretRef.Name)
	if err != nil {
		_ = r.updateConditions(ctx, currentBMC, true, ConditionReset, corev1.ConditionFalse, ReasonAuthenticationFailed, fmt.Sprintf("Failed to get credentials: %v", err))
		return fmt.Errorf("failed to get BMC credentials for SSH reset: %w", err)
	}
	if err := bmcutils.SSHResetBMCFunc(ctx, address, manufacturer, username, password, r.SSHResetTimeout); err != nil {
		_ = r.updateConditions(ctx, currentBMC, true, ConditionReset, corev1.ConditionFalse, ReasonInternalError, fmt.Sprintf("SSH reset failed: %v", err))
		return fmt.Errorf("SSH reset failed: %w", err)
	}
	log.Info("Successfully reset BMC via SSH")

	// ConditionReset is not cleared here — it is cleared when the BMC reconnects
	// successfully via handlePreviousBMCResetAnnotations.
	if err := r.Get(ctx, client.ObjectKey{Name: bmcName}, currentBMC); err != nil {
		return fmt.Errorf("failed to re-fetch BMC after SSH reset: %w", err)
	}
	bmcBase := currentBMC.DeepCopy()
	now := metav1.Now()
	currentBMC.Status.LastResetTime = &now
	if err := r.Status().Patch(ctx, currentBMC, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to patch LastResetTime after SSH reset: %w", err)
	}
	return nil
}

func (r *BMCReconciler) updateConditions(ctx context.Context, bmcObj *metalv1alpha1.BMC, createIfNotFound bool, conditionType string, status corev1.ConditionStatus, reason, message string) error {
	condition := &metav1.Condition{}
	ok, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, conditionType, condition)
	if err != nil {
		return fmt.Errorf("failed to find condition %s: %w", conditionType, err)
	}
	if !ok && !createIfNotFound {
		// condition not found and not allowed to create
		return nil
	}
	bmcBase := bmcObj.DeepCopy()
	if err := r.Conditions.UpdateSlice(
		&bmcObj.Status.Conditions,
		conditionType,
		conditionutils.UpdateStatus(status),
		conditionutils.UpdateReason(reason),
		conditionutils.UpdateMessage(message),
	); err != nil {
		return fmt.Errorf("failed to patch condition %s: %w", conditionType, err)
	}
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to patch BMC conditions: %w", err)
	}
	return nil
}

func (r *BMCReconciler) handleEventSubscriptions(ctx context.Context, bmcClient bmc.BMC, bmcObj *metalv1alpha1.BMC) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if r.EventURL == "" {
		return false, nil
	}
	log.V(1).Info("Handling event subscriptions for BMC", "bmcName", bmcObj.Name, "bmcIP", bmcObj.Status.IP)
	modified := false

	if bmcObj.Status.MetricsReportSubscriptionLink == "" {
		link, err := serverevents.SubscribeMetricsReport(ctx, r.EventURL, bmcObj.Name, bmcClient)
		if err != nil {
			return false, fmt.Errorf("failed to subscribe to server metrics report for BMC %s (%s): %w", bmcObj.Name, bmcObj.Status.IP, err)
		}
		bmcBase := bmcObj.DeepCopy()
		bmcObj.Status.MetricsReportSubscriptionLink = link
		modified = true
		if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
			return false, fmt.Errorf("failed to patch server status with subscription links: %w", err)
		}
		log.Info("Event subscription established", "bmcName", bmcObj.Name, "bmcIP", bmcObj.Status.IP, "type", "metrics", "link", link)
	}
	if bmcObj.Status.EventsSubscriptionLink == "" {
		link, err := serverevents.SubscribeEvents(ctx, r.EventURL, bmcObj.Name, bmcClient)
		if err != nil {
			return false, fmt.Errorf("failed to subscribe to server alerts for BMC %s (%s): %w", bmcObj.Name, bmcObj.Status.IP, err)
		}
		bmcBase := bmcObj.DeepCopy()
		bmcObj.Status.EventsSubscriptionLink = link
		modified = true
		if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
			return false, fmt.Errorf("failed to patch server status with subscription links: %w", err)
		}
		log.Info("Event subscription established", "bmcName", bmcObj.Name, "bmcIP", bmcObj.Status.IP, "type", "events", "link", link)
	}
	return modified, nil
}

func (r *BMCReconciler) deleteEventSubscription(ctx context.Context, bmcClient bmc.BMC, bmcObj *metalv1alpha1.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	if r.EventURL == "" {
		return nil
	}
	if bmcObj.Status.MetricsReportSubscriptionLink != "" {
		if err := bmcClient.DeleteEventSubscription(ctx, bmcObj.Status.MetricsReportSubscriptionLink); err != nil {
			return fmt.Errorf("failed to unsubscribe from server metrics report: %w", err)
		}
		log.V(1).Info("Unsubscribed from server metrics report")
	}
	if bmcObj.Status.EventsSubscriptionLink != "" {
		if err := bmcClient.DeleteEventSubscription(ctx, bmcObj.Status.EventsSubscriptionLink); err != nil {
			return fmt.Errorf("failed to unsubscribe from server events: %w", err)
		}
		log.V(1).Info("Unsubscribed from server events")
	}
	return nil
}

func (r *BMCReconciler) enqueueBMCByEndpoint(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	bmcList := &metalv1alpha1.BMCList{}
	if err := r.List(ctx, bmcList); err != nil {
		log.Error(err, "Failed to list BMCs for Endpoint watch", "endpoint", obj.GetName())
		return nil
	}
	var reqs []ctrl.Request
	for _, bmcObj := range bmcList.Items {
		if bmcObj.Spec.EndpointRef != nil && bmcObj.Spec.EndpointRef.Name == obj.GetName() {
			reqs = append(reqs, ctrl.Request{NamespacedName: types.NamespacedName{Name: bmcObj.Name}})
		}
	}
	return reqs
}

func (r *BMCReconciler) enqueueBMCByBMCSecret(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	bmcList := &metalv1alpha1.BMCList{}
	if err := r.List(ctx, bmcList); err != nil {
		log.Error(err, "Failed to list BMCs for BMCSecret watch", "bmcSecret", obj.GetName())
		return nil
	}
	var reqs []ctrl.Request
	for _, bmcObj := range bmcList.Items {
		if bmcObj.Spec.BMCSecretRef.Name == obj.GetName() {
			reqs = append(reqs, ctrl.Request{NamespacedName: types.NamespacedName{Name: bmcObj.Name}})
		}
	}
	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMC{}).
		Owns(&metalv1alpha1.Server{}).
		Watches(&metalv1alpha1.Endpoint{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByEndpoint)).
		Watches(&metalv1alpha1.BMCSecret{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByBMCSecret)).
		Complete(r)
}
