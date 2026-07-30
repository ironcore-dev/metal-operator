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
	FailureResetDelay  time.Duration
	Options            bmc.Options
	ManagerNamespace   string
	EventURL           string
	ResetWaitTime      time.Duration
	ReconnectInterval  time.Duration
	DNSRecordTemplate  string
	Conditions         *conditionutils.Accessor
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

	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.DefaultProtocol, r.SkipCertValidation, r.Options)
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

	switch r.resetWaitState(bmcObj) {
	case resetWaitPending:
		log.V(1).Info("Skipped BMC reconciliation while waiting for BMC reset to complete")
		return ctrl.Result{RequeueAfter: r.ReconnectInterval}, nil
	case resetWaitExpired:
		// The reset wait window elapsed but ConditionReset is still True. Clear
		// it so shouldResetBMC can schedule future auto-resets; otherwise the
		// stuck True condition blocks all recovery while the BMC stays down.
		if err := r.clearResetCondition(ctx, bmcObj, ReasonResetTimeout, "BMC reset timed out waiting for reconnect"); err != nil {
			return ctrl.Result{}, err
		}
	}

	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.DefaultProtocol, r.SkipCertValidation, r.Options, bmcutils.BMCConnectivityCheckOption)
	if err != nil {
		if r.shouldResetBMC(bmcObj) {
			log.V(1).Info("BMC needs reset, resetting", "BMC", bmcObj.Name)
			if err := r.resetBMC(ctx, bmcObj, bmcClient, ReasonAutoReset, bmcAutoResetMessage); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reset BMC: %w", err)
			}
			log.V(1).Info("BMC reset initiated", "BMC", bmcObj.Name)
			return ctrl.Result{
				RequeueAfter: r.ReconnectInterval,
			}, nil
		}
		return ctrl.Result{RequeueAfter: r.ReconnectInterval}, r.updateReadyConditionOnBMCFailure(ctx, bmcObj, err)
	}
	defer bmcClient.Logout()

	if modified, err := r.handleAnnotationOperations(ctx, bmcObj, bmcClient); err != nil || modified {
		return ctrl.Result{}, err
	}

	// Mark BMC connected and clear any in-flight reset in a single status patch.
	bmcBase := bmcObj.DeepCopy()
	if err := r.Conditions.UpdateSlice(
		&bmcObj.Status.Conditions,
		ConditionReady,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(ReasonConnected),
		conditionutils.UpdateMessage("BMC is connected"),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set BMC connected condition: %w", err)
	}
	if found, _ := r.Conditions.FindSlice(bmcObj.Status.Conditions, ConditionReset, &metav1.Condition{}); found {
		if err := r.Conditions.UpdateSlice(
			&bmcObj.Status.Conditions,
			ConditionReset,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(ReasonResetComplete),
			conditionutils.UpdateMessage("BMC reset is complete"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set BMC reset complete condition: %w", err)
		}
	}
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch BMC conditions: %w", err)
	}

	if err := r.patchBMCStatus(ctx, bmcClient, bmcObj); err != nil {
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

func (r *BMCReconciler) patchBMCStatus(ctx context.Context, bmcClient bmc.BMC, bmcObj *metalv1alpha1.BMC) error {
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
	if value != schemas.GracefulRestartResetType {
		log.V(1).Info("Unsupported operation annotation", "Operation", operation, "RedfishResetType", value)
		return false, nil
	}

	// Consume the annotation before issuing the side-effecting reset, so a
	// crash or failed Redfish call cannot leave it behind as stale state. Once
	// resetBMC sets ConditionReset, the condition is the sole source of truth
	// for the in-flight reset.
	bmcBase := bmcObj.DeepCopy()
	metautils.DeleteAnnotation(bmcObj, metalv1alpha1.OperationAnnotation)
	if err := r.Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return false, fmt.Errorf("failed to remove operation annotation: %w", err)
	}
	log.V(1).Info("Removed operation annotation", "Operation", operation)

	log.V(1).Info("Handling operation", "Operation", operation, "RedfishResetType", value)
	if err := r.resetBMC(ctx, bmcObj, bmcClient, ReasonUserReset, bmcUserResetMessage); err != nil {
		return false, fmt.Errorf("failed to reset BMC: %w", err)
	}
	log.Info("Handled operation", "Operation", operation)
	return true, nil
}

func (r *BMCReconciler) updateReadyConditionOnBMCFailure(ctx context.Context, bmcObj *metalv1alpha1.BMC, err error) error {
	reason, message := ReasonUnknownError, fmt.Sprintf("BMC connection error: %v", err)
	if httpErr, ok := errors.AsType[*schemas.Error](err); ok {
		switch httpErr.HTTPReturnedStatusCode {
		case 401:
			reason, message = ReasonAuthenticationFailed, "BMC credentials are invalid"
		case 500:
			reason, message = ReasonInternalError, "BMC internal server error"
		case 503:
			reason, message = ReasonConnectionFailed, "BMC service unavailable"
		}
	}
	if err := r.patchCondition(ctx, bmcObj, ConditionReady, corev1.ConditionFalse, reason, message); err != nil {
		return fmt.Errorf("failed to set BMC ready condition: %w", err)
	}
	return err
}

// resetWaitState reports the state of an in-flight BMC reset.
type resetWaitState int

const (
	resetWaitNone    resetWaitState = iota // no ConditionReset, or it is not True
	resetWaitPending                       // ConditionReset is True and within the wait window
	resetWaitExpired                       // ConditionReset is True but the wait window has elapsed
)

func (r *BMCReconciler) resetWaitState(bmcObj *metalv1alpha1.BMC) resetWaitState {
	condition := &metav1.Condition{}
	found, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, ConditionReset, condition)
	if err != nil || !found || condition.Status != metav1.ConditionTrue {
		return resetWaitNone
	}
	// give bmc some time to start the reset process
	if time.Since(condition.LastTransitionTime.Time) < r.ResetWaitTime {
		return resetWaitPending
	}
	return resetWaitExpired
}

func (r *BMCReconciler) shouldResetBMC(bmcObj *metalv1alpha1.BMC) bool {
	if r.FailureResetDelay == 0 {
		return false
	}
	if resetStatus, _ := r.Conditions.FindSliceStatus(bmcObj.Status.Conditions, ConditionReset); resetStatus == corev1.ConditionTrue {
		return false
	}
	readyCondition := &metav1.Condition{}
	found, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, ConditionReady, readyCondition)
	if err != nil || !found {
		return false
	}
	if readyCondition.Status == metav1.ConditionFalse &&
		(readyCondition.Reason == ReasonInternalError || readyCondition.Reason == ReasonConnectionFailed) &&
		time.Since(readyCondition.LastTransitionTime.Time) > r.FailureResetDelay {
		return true
	}
	return false
}

func (r *BMCReconciler) resetBMC(ctx context.Context, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC, reason, message string) error {
	log := ctrl.LoggerFrom(ctx)
	if err := r.patchCondition(ctx, bmcObj, ConditionReset, corev1.ConditionTrue, reason, message); err != nil {
		return fmt.Errorf("failed to set BMC resetting condition: %w", err)
	}
	if bmcClient == nil {
		return r.failReset(ctx, bmcObj, fmt.Errorf("could not reset BMC %s: no client connection", bmcObj.Name))
	}
	err := bmcClient.ResetManager(ctx, bmcObj.Spec.BMCUUID, schemas.GracefulRestartResetType)
	if err == nil {
		log.Info("Successfully reset BMC via Redfish", "BMC", bmcObj.Name)
		return nil
	}
	if httpErr, ok := errors.AsType[*schemas.Error](err); ok {
		// only retryable on 5xx; anything else is a permanent failure for this attempt
		if httpErr.HTTPReturnedStatusCode >= 500 && httpErr.HTTPReturnedStatusCode < 600 {
			log.V(1).Info("BMC reset returned a retryable 5xx, leaving reset condition set", "BMC", bmcObj.Name)
			return nil
		}
		return r.failReset(ctx, bmcObj, fmt.Errorf("could not reset BMC: %w", err))
	}
	return r.failReset(ctx, bmcObj, fmt.Errorf("could not reset BMC, unknown error: %w", err))
}

// failReset clears the in-flight reset condition and joins the clearing error
// with the reset failure.
func (r *BMCReconciler) failReset(ctx context.Context, bmcObj *metalv1alpha1.BMC, err error) error {
	return errors.Join(
		r.clearResetCondition(ctx, bmcObj, ReasonResetFailed, "BMC reset failed"),
		err,
	)
}

func (r *BMCReconciler) clearResetCondition(ctx context.Context, bmcObj *metalv1alpha1.BMC, reason, message string) error {
	found, err := r.Conditions.FindSlice(bmcObj.Status.Conditions, ConditionReset, &metav1.Condition{})
	if err != nil {
		return fmt.Errorf("failed to find condition %s: %w", ConditionReset, err)
	}
	if !found {
		return nil
	}
	return r.patchCondition(ctx, bmcObj, ConditionReset, corev1.ConditionFalse, reason, message)
}

func (r *BMCReconciler) patchCondition(ctx context.Context, bmcObj *metalv1alpha1.BMC, conditionType string, status corev1.ConditionStatus, reason, message string) error {
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
