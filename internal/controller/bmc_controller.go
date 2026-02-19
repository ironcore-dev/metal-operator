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
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	BMCFinalizer = "metal.ironcore.dev/bmc"

	bmcResetConditionType = "Reset"
	bmcReadyConditionType = "Ready"

	bmcAuthenticationFailedReason = "AuthenticationFailed"
	bmcInternalErrorReason        = "InternalServerError"
	bmcUnknownErrorReason         = "UnknownError"
	bmcConnectionFailedReason     = "ConnectionFailed"
	bmcUserResetReason            = "UserRequested"
	bmcAutoResetReason            = "AutoResetting"
	bmcConnectedReason            = "BMCConnected"

	bmcUserResetMessage = "BMC reset initiated by user. Waiting for it to come back online."
	bmcAutoResetMessage = "BMC reset initiated automatically after repeated connection failures. Waiting for it to come back online."
)

// BMCReconciler reconciles a BMC object
type BMCReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Insecure bool
	// BMCFailureResetDelay defines the duration after which a BMC will be reset upon repeated connection failures.
	BMCFailureResetDelay time.Duration
	BMCOptions           bmc.Options
	ManagerNamespace     string
	// BMCResetWaitTime defines the duration to wait after a BMC reset before attempting reconciliation again.
	BMCResetWaitTime time.Duration
	// BMCClientRetryInterval defines the duration to requeue reconciliation after a BMC client error/reset/unavailablility.
	BMCClientRetryInterval time.Duration
	// DNSRecordTemplatePath is the path to the file containing the DNSRecord template.
	DNSRecordTemplate string
	Conditions        *conditionutils.Accessor
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
	if r.waitForBMCReset(bmcObj, r.BMCResetWaitTime) {
		log.V(1).Info("Skipped BMC reconciliation while waiting for BMC reset to complete")
		err := r.updateBMCState(ctx, bmcObj, metalv1alpha1.BMCStatePending)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{
			RequeueAfter: r.BMCClientRetryInterval,
		}, nil
	}
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCOptions, bmcutils.BMCConnectivityCheckOption)
	if err != nil {
		if r.shouldResetBMC(bmcObj) {
			log.V(1).Info("BMC needs reset, resetting", "BMC", bmcObj.Name)
			if err := r.resetBMC(ctx, bmcObj, bmcClient, bmcAutoResetReason, bmcAutoResetMessage); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reset BMC: %w", err)
			}
			log.V(1).Info("BMC reset initiated", "BMC", bmcObj.Name)
			return ctrl.Result{
				RequeueAfter: r.BMCClientRetryInterval,
			}, nil
		}
		err = r.updateReadyConditionOnBMCFailure(ctx, bmcObj, err)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	defer bmcClient.Logout()

	// if BMC reset was issued and is successful, ensure to remove previous reset annotation
	if modified, err := r.handlePreviousBMCResetAnnotations(ctx, bmcObj); err != nil || modified {
		return ctrl.Result{}, err
	}

	if modified, err := r.handleAnnotationOperations(ctx, bmcObj, bmcClient); err != nil || modified {
		return ctrl.Result{}, err
	}

	if err := r.updateConditions(ctx, bmcObj, true, bmcReadyConditionType, corev1.ConditionTrue, bmcConnectedReason, "BMC is connected"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set BMC connected condition: %w", err)
	}
	if err := r.updateConditions(ctx, bmcObj, false, bmcResetConditionType, corev1.ConditionFalse, "ResetComplete", "BMC reset is complete"); err != nil {
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
	// parse time to metav1.Time: ISO 8601 format
	lastResetTime := &metav1.Time{}
	if manager.LastResetTime != "" {
		t, err := time.Parse(time.RFC3339, manager.LastResetTime)
		if err == nil {
			lastResetTime = &metav1.Time{Time: t}
		}
	}
	if manager != nil {
		bmcBase := bmcObj.DeepCopy()
		bmcObj.Status.Manufacturer = manager.Manufacturer
		bmcObj.Status.State = metalv1alpha1.BMCState(string(manager.Status.State))
		// Set power state, or unknown if not available from BMC
		if manager.PowerState != "" {
			bmcObj.Status.PowerState = metalv1alpha1.BMCPowerState(string(manager.PowerState))
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
	} else {
		log.V(1).Info("Manager details not available for BMC", "BMC", bmcObj.Name)
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
			server.Spec.UUID = ""
			server.Spec.SystemUUID = strings.ToLower(s.UUID)
			server.Spec.SystemURI = s.URI
			server.Spec.BMCRef = &corev1.LocalObjectReference{Name: bmcObj.Name}
			return controllerutil.SetControllerReference(bmcObj, server, r.Scheme)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create or patch server %s: %w", server.Name, err))
			continue
		}
		log.V(1).Info("Created or patched Server", "Server", server.Name, "Operation", opResult)
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred during server discovery: %v", errs)
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
	var value redfish.ResetType
	if value, ok = metalv1alpha1.AnnotationToRedfishMapping[operation]; !ok {
		log.V(1).Info("Unknown operation annotation, ignoring", "Operation", operation, "Supported Operations", redfish.GracefulRestartResetType)
		return false, nil
	}
	switch value {
	case redfish.GracefulRestartResetType:
		log.V(1).Info("Handling operation", "Operation", operation, "RedfishResetType", value)
		if err := r.resetBMC(ctx, bmcObj, bmcClient, bmcUserResetReason, bmcUserResetMessage); err != nil {
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
	httpErr := &common.Error{}
	if errors.As(err, &httpErr) {
		// only handle 5xx errors
		switch httpErr.HTTPReturnedStatusCode {
		case 401:
			// Unauthorized error, likely due to bad credentials
			if err := r.updateConditions(ctx, bmcObj, true, bmcReadyConditionType, corev1.ConditionFalse, bmcAuthenticationFailedReason, "BMC credentials are invalid"); err != nil {
				return fmt.Errorf("failed to set BMC unauthorized condition: %w", err)
			}

		case 500:
			// Internal Server Error, might be transient
			if err := r.updateConditions(ctx, bmcObj, true, bmcReadyConditionType, corev1.ConditionFalse, bmcInternalErrorReason, "BMC internal server error"); err != nil {
				return fmt.Errorf("failed to set BMC internal server error condition: %w", err)
			}
		case 503:
			// Service Unavailable, might be transient
			if err := r.updateConditions(ctx, bmcObj, true, bmcReadyConditionType, corev1.ConditionFalse, bmcConnectionFailedReason, "BMC service unavailable"); err != nil {
				return fmt.Errorf("failed to set BMC service unavailable condition: %w", err)
			}
		default:
			if err := r.updateConditions(ctx, bmcObj, true, bmcReadyConditionType, corev1.ConditionFalse, bmcUnknownErrorReason, fmt.Sprintf("BMC connection error: %v", err)); err != nil {
				return fmt.Errorf("failed to set BMC error condition: %w", err)
			}
		}
	} else {
		if err := r.updateConditions(ctx, bmcObj, true, bmcReadyConditionType, corev1.ConditionFalse, bmcUnknownErrorReason, fmt.Sprintf("BMC connection error: %v", err)); err != nil {
			return fmt.Errorf("failed to set BMC error condition: %w", err)
		}
	}
	return err
}

func (r *BMCReconciler) waitForBMCReset(bmcObj *metalv1alpha1.BMC, delay time.Duration) bool {
	condition := &metav1.Condition{}
	found, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, bmcResetConditionType, condition)
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
	found, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, bmcResetConditionType, condition)
	if err != nil || !found {
		return false, nil
	}
	if condition.Status == metav1.ConditionTrue {
		if operation, ok := bmcObj.GetAnnotations()[metalv1alpha1.OperationAnnotation]; ok && operation == metalv1alpha1.GracefulRestartBMC {
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
	found, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, bmcResetConditionType, bmcResetCondition)
	if err != nil || (found && bmcResetCondition.Status == metav1.ConditionTrue) {
		return false
	}
	readyCondition := &metav1.Condition{}
	found, err = r.Conditions.FindSlice(bmcObj.Status.Conditions, bmcReadyConditionType, readyCondition)
	if err != nil || !found {
		return false
	}
	if readyCondition.Status == metav1.ConditionFalse && (readyCondition.Reason == bmcInternalErrorReason || readyCondition.Reason == bmcConnectionFailedReason) {
		if time.Since(readyCondition.LastTransitionTime.Time) > r.BMCFailureResetDelay {
			return true
		}
	}
	return false
}

func (r *BMCReconciler) updateBMCState(ctx context.Context, bmcObj *metalv1alpha1.BMC, state metalv1alpha1.BMCState) error {
	if bmcObj.Status.State == state {
		return nil
	}
	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.State = state
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to patch BMC state to Pending: %w", err)
	}
	return nil
}

func (r *BMCReconciler) resetBMC(ctx context.Context, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC, reason, message string) error {
	log := ctrl.LoggerFrom(ctx)
	if err := r.updateConditions(ctx, bmcObj, true, bmcResetConditionType, corev1.ConditionTrue, reason, message); err != nil {
		return fmt.Errorf("failed to set BMC resetting condition: %w", err)
	}
	var err error
	if bmcClient != nil {
		if err = bmcClient.ResetManager(ctx, bmcObj.Spec.BMCUUID, redfish.GracefulRestartResetType); err == nil {
			log.Info("Successfully reset BMC via Redfish", "BMC", bmcObj.Name)
			return r.updateBMCState(ctx, bmcObj, metalv1alpha1.BMCStatePending)
		}
	}
	// BMC Unavailable, currently can not perform reset. try to reset with ssh when available
	log.Error(err, "failed to reset BMC via Redfish, falling back to rest via ssh", "BMC", bmcObj.Name)
	if httpErr, ok := err.(*common.Error); ok {
		// only handle 5xx errors
		if httpErr.HTTPReturnedStatusCode < 500 || httpErr.HTTPReturnedStatusCode >= 600 {
			return errors.Join(r.updateBMCState(ctx, bmcObj, metalv1alpha1.BMCStatePending), fmt.Errorf("cannot reset bmc: %w", err))
		}
	} else {
		return fmt.Errorf("cannot reset bmc, unknown error: %w", err)
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

func (r *BMCReconciler) enqueueBMCByEndpoint(ctx context.Context, obj client.Object) []ctrl.Request {
	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{Name: obj.(*metalv1alpha1.Endpoint).Name},
		},
	}
}

func (r *BMCReconciler) enqueueBMCByBMCSecret(ctx context.Context, obj client.Object) []ctrl.Request {
	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{Name: obj.(*metalv1alpha1.BMCSecret).Name},
		},
	}
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
