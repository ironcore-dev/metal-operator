// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/template"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/ironcore-dev/metal-operator/internal/serverevents"

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

	// Certificate management condition types
	bmcCertificateInstalledConditionType = "CertificateInstalled"
	bmcCertificateExpiringConditionType  = "CertificateExpiring"

	// Certificate management reasons
	bmcCertificateInstalledReason     = "CertificateInstalled"
	bmcCertificateExpiringSoonReason  = "CertificateExpiringSoon"
	bmcCSRGenerationFailedReason      = "CSRGenerationFailed"
	bmcCertificateInstallFailedReason = "CertificateInstallFailed"
	bmcCSRExpiredReason               = "CSRExpired"
	bmcCSRDeniedReason                = "CSRDenied"
	bmcCertificateRequestedReason     = "CertificateRequested"
	bmcCertificateValidReason         = "CertificateValid"

	bmcUserResetMessage = "BMC reset initiated by user. Waiting for it to come back online."
	bmcAutoResetMessage = "BMC reset initiated automatically after repeated connection failures. Waiting for it to come back online."

	// DefaultCertificateRenewalThreshold is the default time before certificate expiration to trigger renewal.
	// Set to 30 days (720 hours) which is 1/3 of a typical 90-day certificate lifetime.
	DefaultCertificateRenewalThreshold = 720 * time.Hour

	// DefaultCertificateSignerName is the default signer for BMC certificates.
	DefaultCertificateSignerName = "metal.ironcore.dev/bmc-https"
)

// legacyBMCConditionReasons maps old condition reason strings to their new values.
// TODO: Remove this migration in the next release once all CRs have been reconciled.
var legacyBMCConditionReasons = map[string]string{
	"BMCConnected": ReasonConnected,
}

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

	// Certificate management defaults (applied to BMCs that don't specify these fields)
	DefaultCertificateManagementEnabled bool
	DefaultCertificateSignerName        string
	DefaultCertificateApprovalMode      metalv1alpha1.CertificateApprovalPolicy
	DefaultCertificateRenewalThreshold  time.Duration
	DefaultCertificateSubject           *metalv1alpha1.CertificateSubject
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs/finalizers,verbs=update
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/approval,verbs=update
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=signers,resourceNames=metal.ironcore.dev/bmc-https,verbs=approve
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BMCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, req.NamespacedName, bmcObj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// TODO: Remove this migration in the next release once all CRs have been reconciled.
	bmcBase := bmcObj.DeepCopy()
	if migrateConditionReasons(bmcObj.Status.Conditions, legacyBMCConditionReasons) {
		log := ctrl.LoggerFrom(ctx)
		log.Info("Migrated legacy condition reasons on BMC")
		if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to migrate legacy conditions: %w", err)
		}
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
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.DefaultProtocol, r.SkipCertValidation, r.BMCOptions, bmcutils.BMCConnectivityCheckOption)
	if err != nil {
		if r.shouldResetBMC(bmcObj) {
			log.V(1).Info("BMC needs reset, resetting", "BMC", bmcObj.Name)
			if err := r.resetBMC(ctx, bmcObj, bmcClient, ReasonAutoReset, bmcAutoResetMessage); err != nil {
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

	if err := r.updateConditions(ctx, bmcObj, true, ConditionReady, corev1.ConditionTrue, ReasonConnected, "BMC is connected"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set BMC connected condition: %w", err)
	}
	if err := r.updateConditions(ctx, bmcObj, false, ConditionReset, corev1.ConditionFalse, "ResetComplete", "BMC reset is complete"); err != nil {
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

	// Handle certificate management
	// Certificate reconciliation errors should not prevent BMC from reaching Ready state
	if err := r.reconcileCertificate(ctx, bmcObj, bmcClient); err != nil {
		log.Error(err, "Failed to reconcile certificate")
		// Update condition to reflect certificate error, but continue reconciliation
		if condErr := r.updateConditions(ctx, bmcObj, true, bmcCertificateInstalledConditionType,
			corev1.ConditionFalse, bmcCSRGenerationFailedReason,
			fmt.Sprintf("Certificate reconciliation failed: %v", err)); condErr != nil {
			log.Error(condErr, "Failed to update certificate condition")
		}
	}
	if err := r.checkCertificateExpiration(ctx, bmcObj); err != nil {
		log.Error(err, "Failed to check certificate expiration")
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
	var value schemas.ResetType
	if value, ok = metalv1alpha1.AnnotationToRedfishMapping[operation]; !ok {
		log.V(1).Info("Unknown operation annotation, ignoring", "Operation", operation, "Supported Operations", schemas.GracefulRestartResetType)
		return false, nil
	}
	switch value {
	case schemas.GracefulRestartResetType:
		log.V(1).Info("Handling operation", "Operation", operation, "RedfishResetType", value)
		if err := r.resetBMC(ctx, bmcObj, bmcClient, ReasonUserReset, bmcUserResetMessage); err != nil {
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
	if err := r.updateConditions(ctx, bmcObj, true, ConditionReset, corev1.ConditionTrue, reason, message); err != nil {
		return fmt.Errorf("failed to set BMC resetting condition: %w", err)
	}
	var err error
	if bmcClient != nil {
		if err = bmcClient.ResetManager(ctx, bmcObj.Spec.BMCUUID, schemas.GracefulRestartResetType); err == nil {
			log.Info("Successfully reset BMC via Redfish", "BMC", bmcObj.Name)
			return r.updateBMCState(ctx, bmcObj, metalv1alpha1.BMCStatePending)
		}
	}
	// BMC Unavailable, currently can not perform reset. try to reset with ssh when available
	log.Error(err, "Failed to reset BMC via Redfish, falling back to reset via SSH", "BMC", bmcObj.Name)
	if httpErr, ok := err.(*schemas.Error); ok {
		// only handle 5xx errors
		if httpErr.HTTPReturnedStatusCode < 500 || httpErr.HTTPReturnedStatusCode >= 600 {
			return errors.Join(r.updateBMCState(ctx, bmcObj, metalv1alpha1.BMCStatePending), fmt.Errorf("could not reset BMC: %w", err))
		}
	} else {
		return fmt.Errorf("could not reset BMC, unknown error: %w", err)
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

func (r *BMCReconciler) applyCertificateDefaults(bmcObj *metalv1alpha1.BMC) {
	if bmcObj.Spec.CertificateManagementPolicy == nil && r.DefaultCertificateManagementEnabled {
		bmcObj.Spec.CertificateManagementPolicy = ptr.To(metalv1alpha1.CertificateManagementPolicyAutomatic)
	}
}

func (r *BMCReconciler) reconcileCertificate(ctx context.Context, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) error {
	log := ctrl.LoggerFrom(ctx)

	r.applyCertificateDefaults(bmcObj)

	if bmcObj.Spec.CertificateManagementPolicy == nil ||
		*bmcObj.Spec.CertificateManagementPolicy != metalv1alpha1.CertificateManagementPolicyAutomatic {
		return nil
	}
	if bmcObj.Status.CertificateSigningRequestRef != nil {
		return r.reconcilePendingCSR(ctx, bmcObj, bmcClient)
	}
	if bmcObj.Status.CertificateSecretRef != nil {
		needsRenewal, err := r.needsCertificateRenewal(ctx, bmcObj)
		if err != nil {
			return err
		}
		if !needsRenewal {
			return nil
		}
		log.Info("Certificate needs renewal")
	}

	return r.initiateCertificateRequest(ctx, bmcObj, bmcClient)
}

func (r *BMCReconciler) initiateCertificateRequest(ctx context.Context, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Initiating certificate request for BMC")

	commonName := bmcObj.Status.IP.String()
	if bmcObj.Spec.Hostname != nil && *bmcObj.Spec.Hostname != "" {
		commonName = *bmcObj.Spec.Hostname
	}

	csrReq := bmc.CSRRequest{
		CommonName:       commonName,
		KeyPairAlgorithm: "RSA2048",
		AlternativeNames: []string{commonName, bmcObj.Status.IP.String()},
	}

	if r.DefaultCertificateSubject != nil {
		csrReq.Organization = r.DefaultCertificateSubject.Organization
		csrReq.OrganizationalUnit = r.DefaultCertificateSubject.OrganizationalUnit
		csrReq.Country = r.DefaultCertificateSubject.Country
		csrReq.State = r.DefaultCertificateSubject.State
		csrReq.City = r.DefaultCertificateSubject.Locality
	}

	log.Info("Generating CSR on BMC", "commonName", commonName)
	csrResp, err := bmcClient.GenerateCSR(ctx, csrReq)
	if err != nil {
		_ = r.updateConditions(ctx, bmcObj, true, bmcCertificateInstalledConditionType,
			corev1.ConditionFalse, bmcCSRGenerationFailedReason,
			fmt.Sprintf("Failed to generate CSR on BMC: %v", err))
		return err
	}

	if err := r.validateBMCCSR(csrResp.CSRString, csrReq); err != nil {
		_ = r.updateConditions(ctx, bmcObj, true, bmcCertificateInstalledConditionType,
			corev1.ConditionFalse, bmcCSRGenerationFailedReason,
			fmt.Sprintf("Invalid CSR from BMC: %v", err))
		return fmt.Errorf("CSR validation failed: %w", err)
	}

	signerName := r.DefaultCertificateSignerName
	if signerName == "" {
		signerName = DefaultCertificateSignerName
	}

	k8sCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("bmc-%s-%s", bmcObj.Name, string(bmcObj.UID[:8])),
			Labels: map[string]string{
				"metal.ironcore.dev/bmc": bmcObj.Name,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:    []byte(csrResp.CSRString),
			SignerName: signerName,
			Usages: []certificatesv1.KeyUsage{
				certificatesv1.UsageDigitalSignature,
				certificatesv1.UsageKeyEncipherment,
				certificatesv1.UsageServerAuth,
			},
			ExpirationSeconds: ptr.To(int32(90 * 24 * 60 * 60)), // 90 days
		},
	}

	if err := controllerutil.SetControllerReference(bmcObj, k8sCSR, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference on CSR: %w", err)
	}

	if err := r.Create(ctx, k8sCSR); err != nil {
		if apierrors.IsAlreadyExists(err) {
			log.Info("CSR already exists, validating ownership and content", "name", k8sCSR.Name)

			existingCSR := &certificatesv1.CertificateSigningRequest{}
			if err := r.Get(ctx, client.ObjectKey{Name: k8sCSR.Name}, existingCSR); err != nil {
				return fmt.Errorf("failed to fetch existing CSR: %w", err)
			}

			if !metav1.IsControlledBy(existingCSR, bmcObj) {
				return fmt.Errorf("CSR %s exists but is owned by a different controller - possible name collision attack", k8sCSR.Name)
			}

			if !bytes.Equal(existingCSR.Spec.Request, k8sCSR.Spec.Request) {
				log.Info("CSR exists with different content, deleting and recreating",
					"name", k8sCSR.Name,
					"reason", "CSR content mismatch")
				if err := r.Delete(ctx, existingCSR); err != nil {
					return fmt.Errorf("failed to delete stale CSR: %w", err)
				}
				if err := r.Create(ctx, k8sCSR); err != nil {
					return fmt.Errorf("failed to recreate CSR after delete: %w", err)
				}
			} else {
				log.V(1).Info("Reusing existing CSR with matching content", "name", k8sCSR.Name)
				k8sCSR = existingCSR
			}
		} else {
			return fmt.Errorf("failed to create CSR: %w", err)
		}
	}

	approvalPolicy := r.DefaultCertificateApprovalMode
	if approvalPolicy == "" {
		approvalPolicy = metalv1alpha1.CertificateApprovalPolicyExternal
	}

	if approvalPolicy == metalv1alpha1.CertificateApprovalPolicyAuto {
		log.Info("SECURITY: Auto-approving CSR - ensure this BMC is in a trusted environment",
			"bmc", bmcObj.Name,
			"bmcIP", bmcObj.Status.IP,
			"commonName", commonName,
			"warning", "Auto-approval should only be used with verified, trusted BMC hardware")
		if err := r.approveCSR(ctx, k8sCSR); err != nil {
			return fmt.Errorf("failed to auto-approve CSR: %w", err)
		}
	}

	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.CertificateSigningRequestRef = &k8sCSR.Name
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return err
	}

	log.Info("CertificateSigningRequest created", "name", k8sCSR.Name)
	_ = r.updateConditions(ctx, bmcObj, true, bmcCertificateInstalledConditionType,
		corev1.ConditionFalse, bmcCertificateRequestedReason,
		"Certificate signing request submitted")

	return nil
}

// reconcilePendingCSR handles a pending CertificateSigningRequest.
func (r *BMCReconciler) reconcilePendingCSR(ctx context.Context, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) error {
	log := ctrl.LoggerFrom(ctx)

	csrName := *bmcObj.Status.CertificateSigningRequestRef
	k8sCSR := &certificatesv1.CertificateSigningRequest{}
	if err := r.Get(ctx, client.ObjectKey{Name: csrName}, k8sCSR); err != nil {
		if apierrors.IsNotFound(err) {
			return r.clearCSRReference(ctx, bmcObj)
		}
		return err
	}

	if r.isCertificateDenied(k8sCSR) {
		return r.handleDeniedCSR(ctx, bmcObj, k8sCSR, csrName)
	}

	if !r.isCertificateApproved(k8sCSR) {
		log.V(1).Info("Waiting for CertificateSigningRequest approval", "name", csrName)
		return nil
	}

	if len(k8sCSR.Status.Certificate) == 0 {
		return r.handlePendingSignature(ctx, bmcObj, k8sCSR, csrName)
	}

	return r.installCertificate(ctx, bmcObj, k8sCSR, bmcClient)
}

func (r *BMCReconciler) clearCSRReference(ctx context.Context, bmcObj *metalv1alpha1.BMC) error {
	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.CertificateSigningRequestRef = nil
	return r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase))
}

func (r *BMCReconciler) handleDeniedCSR(ctx context.Context, bmcObj *metalv1alpha1.BMC, k8sCSR *certificatesv1.CertificateSigningRequest, csrName string) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("CertificateSigningRequest was denied, will retry", "name", csrName)

	if err := r.Delete(ctx, k8sCSR); err != nil {
		log.Error(err, "Failed to delete denied CSR")
	}

	if err := r.clearCSRReference(ctx, bmcObj); err != nil {
		return err
	}

	_ = r.updateConditions(ctx, bmcObj, true, bmcCertificateInstalledConditionType,
		corev1.ConditionFalse, bmcCSRDeniedReason,
		"CSR was denied, will retry on next reconciliation")
	return nil
}

func (r *BMCReconciler) handlePendingSignature(ctx context.Context, bmcObj *metalv1alpha1.BMC, k8sCSR *certificatesv1.CertificateSigningRequest, csrName string) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Waiting for CertificateSigningRequest to be signed", "name", csrName)

	if r.isCSRExpired(k8sCSR) {
		log.Info("CertificateSigningRequest expired before signing, recreating", "name", csrName)

		if err := r.Delete(ctx, k8sCSR); err != nil {
			log.Error(err, "Failed to delete expired CSR")
		}

		if err := r.clearCSRReference(ctx, bmcObj); err != nil {
			return err
		}

		_ = r.updateConditions(ctx, bmcObj, true, bmcCertificateInstalledConditionType,
			corev1.ConditionFalse, bmcCSRExpiredReason,
			"CSR expired before signing, will recreate")
	}

	return nil
}

func (r *BMCReconciler) installCertificate(ctx context.Context, bmcObj *metalv1alpha1.BMC, k8sCSR *certificatesv1.CertificateSigningRequest, bmcClient bmc.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Installing certificate on BMC")

	block, _ := pem.Decode(k8sCSR.Status.Certificate)
	if block == nil {
		return fmt.Errorf("invalid PEM-encoded certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	if err := r.validateCertificateAgainstCSR(k8sCSR.Status.Certificate, k8sCSR.Spec.Request); err != nil {
		return fmt.Errorf("certificate validation failed: %w", err)
	}

	if time.Now().After(cert.NotAfter) {
		return fmt.Errorf("received expired certificate")
	}
	if time.Now().Before(cert.NotBefore) {
		return fmt.Errorf("received certificate not yet valid")
	}

	expectedCN := bmcObj.Status.IP.String()
	if bmcObj.Spec.Hostname != nil && *bmcObj.Spec.Hostname != "" {
		expectedCN = *bmcObj.Spec.Hostname
	}

	expectedSANs := map[string]bool{
		bmcObj.Status.IP.String(): true,
	}
	if bmcObj.Spec.Hostname != nil && *bmcObj.Spec.Hostname != "" {
		expectedSANs[*bmcObj.Spec.Hostname] = true
	}

	foundValidSAN := false
	for _, dnsName := range cert.DNSNames {
		if expectedSANs[dnsName] {
			foundValidSAN = true
			break
		}
	}
	if !foundValidSAN {
		for _, ipAddr := range cert.IPAddresses {
			if expectedSANs[ipAddr.String()] {
				foundValidSAN = true
				break
			}
		}
	}

	if !foundValidSAN {
		return fmt.Errorf("certificate does not contain any expected SANs: %v (found DNS: %v, IPs: %v)",
			expectedSANs, cert.DNSNames, cert.IPAddresses)
	}

	if cert.Subject.CommonName != expectedCN {
		log.Info("Certificate CN does not match expected value (SANs are valid)",
			"expected", expectedCN, "actual", cert.Subject.CommonName)
	}

	if !slices.Contains(cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
		return fmt.Errorf("certificate missing ServerAuth extended key usage")
	}

	if err := bmcClient.InstallCertificate(ctx, string(k8sCSR.Status.Certificate), bmc.CertificateTypeHTTPS); err != nil {
		_ = r.updateConditions(ctx, bmcObj, true, bmcCertificateInstalledConditionType,
			corev1.ConditionFalse, bmcCertificateInstallFailedReason,
			fmt.Sprintf("Failed to install certificate: %v", err))
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("bmc-%s-cert", bmcObj.Name),
			Namespace: r.ManagerNamespace,
		},
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Labels["metal.ironcore.dev/bmc"] = bmcObj.Name
		secret.Type = corev1.SecretTypeTLS
		secret.Data = map[string][]byte{
			"tls.crt": k8sCSR.Status.Certificate,
		}
		return controllerutil.SetControllerReference(bmcObj, secret, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to create or update certificate secret: %w", err)
	}
	log.Info("Certificate secret created or updated", "operation", opResult)

	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: secret.Name}
	bmcObj.Status.CertificateSigningRequestRef = nil

	if err := r.updateCertificateInfo(ctx, bmcObj, bmcClient); err != nil {
		log.Error(err, "Failed to update certificate info")
	}

	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return err
	}

	log.Info("Certificate installed successfully")
	_ = r.updateConditions(ctx, bmcObj, true, bmcCertificateInstalledConditionType,
		corev1.ConditionTrue, bmcCertificateInstalledReason,
		"Certificate installed on BMC")

	return nil
}

func (r *BMCReconciler) checkCertificateExpiration(ctx context.Context, bmcObj *metalv1alpha1.BMC) error {
	log := ctrl.LoggerFrom(ctx)

	if bmcObj.Status.CertificateInfo == nil {
		return nil
	}

	renewalThreshold := r.DefaultCertificateRenewalThreshold
	if renewalThreshold == 0 {
		renewalThreshold = DefaultCertificateRenewalThreshold
	}

	if bmcObj.Status.CertificateInfo.NotAfter != nil {
		expiryTime := bmcObj.Status.CertificateInfo.NotAfter.Time
		timeUntilExpiry := time.Until(expiryTime)

		if timeUntilExpiry < renewalThreshold {
			log.Info("Certificate expiring soon", "expiresIn", timeUntilExpiry)
			_ = r.updateConditions(ctx, bmcObj, true, bmcCertificateExpiringConditionType,
				corev1.ConditionTrue, bmcCertificateExpiringSoonReason,
				fmt.Sprintf("Certificate expires in %s", timeUntilExpiry))

			if bmcObj.Spec.CertificateManagementPolicy != nil &&
				*bmcObj.Spec.CertificateManagementPolicy == metalv1alpha1.CertificateManagementPolicyAutomatic {
				bmcBase := bmcObj.DeepCopy()
				bmcObj.Status.CertificateSecretRef = nil
				if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
					return err
				}
			}
		} else {
			_ = r.updateConditions(ctx, bmcObj, true, bmcCertificateExpiringConditionType,
				corev1.ConditionFalse, bmcCertificateValidReason,
				fmt.Sprintf("Certificate valid until %s", expiryTime))
		}
	}

	return nil
}

func (r *BMCReconciler) approveCSR(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) error {
	allowedSigners := []string{
		DefaultCertificateSignerName,
	}

	if !slices.Contains(allowedSigners, csr.Spec.SignerName) {
		return fmt.Errorf("controller not authorized to approve signer: %s (only allowed: %v)",
			csr.Spec.SignerName, allowedSigners)
	}

	csr.Status.Conditions = append(csr.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
		Type:               certificatesv1.CertificateApproved,
		Status:             corev1.ConditionTrue,
		Reason:             "AutoApproved",
		Message:            fmt.Sprintf("Auto-approved by BMC controller for BMC %s", csr.Labels["metal.ironcore.dev/bmc"]),
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, csr); err != nil {
		return err
	}

	return nil
}

func (r *BMCReconciler) isCertificateApproved(csr *certificatesv1.CertificateSigningRequest) bool {
	for _, condition := range csr.Status.Conditions {
		if condition.Type == certificatesv1.CertificateApproved {
			return condition.Status == corev1.ConditionTrue
		}
		if condition.Type == certificatesv1.CertificateDenied {
			return false
		}
	}
	return false
}

func (r *BMCReconciler) isCertificateDenied(csr *certificatesv1.CertificateSigningRequest) bool {
	for _, condition := range csr.Status.Conditions {
		if condition.Type == certificatesv1.CertificateDenied {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (r *BMCReconciler) isCSRExpired(csr *certificatesv1.CertificateSigningRequest) bool {
	// CSR expires based on spec.expirationSeconds after creation
	if csr.Spec.ExpirationSeconds == nil {
		return false // No expiration set
	}

	expirationDuration := time.Duration(*csr.Spec.ExpirationSeconds) * time.Second
	expirationTime := csr.CreationTimestamp.Add(expirationDuration)

	return time.Now().After(expirationTime)
}

func (r *BMCReconciler) needsCertificateRenewal(ctx context.Context, bmcObj *metalv1alpha1.BMC) (bool, error) {
	if bmcObj.Status.CertificateSecretRef == nil {
		return true, nil
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      bmcObj.Status.CertificateSecretRef.Name,
		Namespace: r.ManagerNamespace,
	}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}

	// Parse certificate and check expiration
	certPEM := secret.Data["tls.crt"]
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return true, nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return true, nil
	}

	renewalThreshold := r.DefaultCertificateRenewalThreshold
	if renewalThreshold == 0 {
		renewalThreshold = DefaultCertificateRenewalThreshold
	}

	timeUntilExpiry := time.Until(cert.NotAfter)
	return timeUntilExpiry < renewalThreshold, nil
}

func (r *BMCReconciler) updateCertificateInfo(ctx context.Context, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) error {
	certs, err := bmcClient.GetCertificates(ctx)
	if err != nil {
		return err
	}

	// Find HTTPS certificate
	for _, cert := range certs {
		if cert.Type == bmc.CertificateTypeHTTPS {
			notBefore, _ := time.Parse(time.RFC3339, cert.ValidNotBefore)
			notAfter, _ := time.Parse(time.RFC3339, cert.ValidNotAfter)

			bmcObj.Status.CertificateInfo = &metalv1alpha1.CertificateInfo{
				Issuer:       cert.Issuer,
				Subject:      cert.Subject,
				NotBefore:    &metav1.Time{Time: notBefore},
				NotAfter:     &metav1.Time{Time: notAfter},
				SerialNumber: cert.SerialNumber,
				Thumbprint:   cert.Fingerprint,
			}
			break
		}
	}

	return nil
}

// validateBMCCSR validates the CSR received from BMC hardware.
func (r *BMCReconciler) validateBMCCSR(csrPEM string, expectedReq bmc.CSRRequest) error {
	csrBlock, _ := pem.Decode([]byte(csrPEM))
	if csrBlock == nil {
		return fmt.Errorf("invalid PEM format")
	}
	if csrBlock.Type != "CERTIFICATE REQUEST" {
		return fmt.Errorf("invalid PEM type: expected CERTIFICATE REQUEST, got %s", csrBlock.Type)
	}

	parsedCSR, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CSR: %w", err)
	}

	if err := parsedCSR.CheckSignature(); err != nil {
		return fmt.Errorf("invalid CSR signature: %w", err)
	}

	// Validate CommonName matches what we requested
	if parsedCSR.Subject.CommonName != expectedReq.CommonName {
		return fmt.Errorf("CN mismatch: expected %s, got %s (possible MITM/spoofing attack)",
			expectedReq.CommonName, parsedCSR.Subject.CommonName)
	}

	// Validate SANs (Subject Alternative Names)
	expectedSANs := make(map[string]bool)
	for _, san := range expectedReq.AlternativeNames {
		expectedSANs[san] = true
	}

	// Check DNS SANs
	for _, dnsName := range parsedCSR.DNSNames {
		if !expectedSANs[dnsName] {
			return fmt.Errorf("unexpected DNS SAN in CSR: %s", dnsName)
		}
		delete(expectedSANs, dnsName)
	}

	// Check IP SANs
	for _, ipAddr := range parsedCSR.IPAddresses {
		ipStr := ipAddr.String()
		if !expectedSANs[ipStr] {
			return fmt.Errorf("unexpected IP SAN in CSR: %s", ipStr)
		}
		delete(expectedSANs, ipStr)
	}

	// All expected SANs should be present
	if len(expectedSANs) > 0 {
		missing := []string{}
		for san := range expectedSANs {
			missing = append(missing, san)
		}
		return fmt.Errorf("missing expected SANs in CSR: %v", missing)
	}

	// Validate key strength
	switch pub := parsedCSR.PublicKey.(type) {
	case *rsa.PublicKey:
		if pub.N.BitLen() < 2048 {
			return fmt.Errorf("RSA key too weak: %d bits (minimum 2048)", pub.N.BitLen())
		}
	case *ecdsa.PublicKey:
		if pub.Curve.Params().BitSize < 256 {
			return fmt.Errorf("ECDSA key too weak: %d bits (minimum 256)", pub.Curve.Params().BitSize)
		}
	default:
		return fmt.Errorf("unsupported key type: %T", pub)
	}

	return nil
}

// validateCertificateAgainstCSR validates that a signed certificate matches the original CSR.
func (r *BMCReconciler) validateCertificateAgainstCSR(certPEM []byte, csrPEM []byte) error {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return fmt.Errorf("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	csrBlock, _ := pem.Decode(csrPEM)
	if csrBlock == nil {
		return fmt.Errorf("invalid CSR PEM in K8s CSR object")
	}
	csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CSR: %w", err)
	}

	// Marshal public keys for comparison
	certPubKey, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal certificate public key: %w", err)
	}
	csrPubKey, err := x509.MarshalPKIXPublicKey(csr.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal CSR public key: %w", err)
	}

	// Compare public keys
	if !bytes.Equal(certPubKey, csrPubKey) {
		return fmt.Errorf("certificate public key does not match CSR public key (possible key substitution attack)")
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMC{}).
		Owns(&metalv1alpha1.Server{}).
		Owns(&corev1.Secret{}).
		Owns(&certificatesv1.CertificateSigningRequest{}).
		Watches(&metalv1alpha1.Endpoint{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByEndpoint)).
		Watches(&metalv1alpha1.BMCSecret{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByBMCSecret)).
		Complete(r)
}
