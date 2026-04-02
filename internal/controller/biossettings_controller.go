// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/stmcginnis/gofish/schemas"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

const (
	BIOSSettingsFinalizer = "metal.ironcore.dev/biossettings"

	ConditionBIOSVersionUpgrade        = "BIOSVersionUpgrade"
	ConditionPendingSettingsCheck      = "PendingSettingsCheck"
	ConditionDuplicateKeys             = "DuplicateKeys"
	ConditionUpdateStarted             = "UpdateStarted"
	ConditionUpdateTimedOut            = "UpdateTimedOut"
	ConditionServerPowerOn             = "ServerPowerOn"
	ConditionSettingsUpdateIssued      = "SettingsUpdateIssued"
	ConditionUnexpectedPendingSettings = "UnexpectedPendingSettings"
	ConditionRebootRequired            = "RebootRequired"
	ConditionRebootPowerOff            = "RebootPowerOff"
	ConditionRebootPowerOn             = "RebootPowerOn"
	ConditionSettingsVerified          = "SettingsVerified"
	ConditionSettingsInvalid           = "SettingsInvalid"

	ReasonVersionMismatch   = "VersionMismatch"
	ReasonPendingSettings   = "PendingSettingsFound"
	ReasonDuplicateKeys     = "DuplicateKeysFound"
	ReasonUpdateStarted     = "UpdateStarted"
	ReasonTimedOut          = "TimedOut"
	ReasonPoweredOn         = "PoweredOn"
	ReasonUpdateIssued      = "UpdateIssued"
	ReasonUpdateSkipped     = "UpdateSkipped"
	ReasonUnexpectedPending = "UnexpectedPendingSettings"
	ReasonRebootRequired    = "RebootRequired"
	ReasonRebootNotRequired = "RebootNotRequired"
	ReasonPoweredOff        = "PoweredOff"
	ReasonVerified          = "Verified"
	ReasonNotYetVerified    = "NotYetVerified"
	ReasonInvalidSettings   = "InvalidSettings"
)

// BIOSSettingsReconciler reconciles a BIOSSettings object
type BIOSSettingsReconciler struct {
	client.Client
	ManagerNamespace            string
	DefaultProtocol             metalv1alpha1.ProtocolScheme
	SkipCertValidation          bool
	Scheme                      *runtime.Scheme
	BMCOptions                  bmc.Options
	ResyncInterval              time.Duration
	TimeoutExpiry               time.Duration
	Conditions                  *conditionutils.Accessor
	DefaultFailedAutoRetryCount int32
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=BMC,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BIOSSettingsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	biosSettings := &metalv1alpha1.BIOSSettings{}
	if err := r.Get(ctx, req.NamespacedName, biosSettings); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, biosSettings)
}

func (r *BIOSSettingsReconciler) reconcileExists(ctx context.Context, settings *metalv1alpha1.BIOSSettings) (ctrl.Result, error) {
	if r.shouldDelete(ctx, settings) {
		return r.delete(ctx, settings)
	}
	return r.reconcile(ctx, settings)
}

func (r *BIOSSettingsReconciler) shouldDelete(ctx context.Context, settings *metalv1alpha1.BIOSSettings) bool {
	log := ctrl.LoggerFrom(ctx)
	if settings.DeletionTimestamp.IsZero() {
		return false
	}
	log.V(1).Info("Reconciling BIOSSettings")

	if controllerutil.ContainsFinalizer(settings, BIOSSettingsFinalizer) &&
		settings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
		if _, err := GetServerByName(ctx, r.Client, settings.Spec.ServerRef.Name); apierrors.IsNotFound(err) {
			log.V(1).Info("Server not found, proceeding with deletion", "Server", settings.Spec.ServerRef.Name)
			return true
		}
		log.V(1).Info("Postponed delete as BIOSSettings update is in progress")
		return false
	}
	return true
}

func (r *BIOSSettingsReconciler) delete(ctx context.Context, settings *metalv1alpha1.BIOSSettings) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Deleting BIOSSettings")
	if err := r.cleanupReferences(ctx, settings); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to cleanup references: %w", err)
	}
	log.V(1).Info("Ensured references were removed")

	log.V(1).Info("Ensuring finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, settings, BIOSSettingsFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer was removed")

	log.V(1).Info("Deleted BIOSSettings")
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) removeServerMaintenance(ctx context.Context, settings *metalv1alpha1.BIOSSettings) error {
	log := ctrl.LoggerFrom(ctx)
	if settings.Spec.ServerMaintenanceRef == nil {
		return nil
	}
	maintenance, err := GetServerMaintenanceForObjectReference(ctx, r.Client, settings.Spec.ServerMaintenanceRef)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get ServerMaintenance for BIOSSettings: %w", err)
	}

	var condition *metav1.Condition
	if err == nil && maintenance.DeletionTimestamp.IsZero() {
		if metav1.IsControlledBy(maintenance, settings) {
			log.V(1).Info("Deleting ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(maintenance), "State", maintenance.Status.State)
			condition, err = GetCondition(r.Conditions, settings.Status.Conditions, ServerMaintenanceConditionDeleted)
			if err != nil {
				return fmt.Errorf("failed to get the delete condition for ServerMaintenance: %w", err)
			}
			if err := r.Conditions.Update(
				condition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(ServerMaintenanceReasonDeleted),
				conditionutils.UpdateMessage(fmt.Sprintf("Deleting %s", maintenance.Name)),
			); err != nil {
				return fmt.Errorf("failed to update deleting ServerMaintenance condition: %w", err)
			}
			if err := r.Delete(ctx, maintenance); err != nil {
				return err
			}
		} else {
			log.V(1).Info("ServerMaintenance is owned by someone else", "ServerMaintenance", client.ObjectKeyFromObject(maintenance), "State", maintenance.Status.State)
		}
	}

	if apierrors.IsNotFound(err) || err == nil {
		log.V(1).Info("Cleaned up ServerMaintenance ref in BIOSSettings as the object is gone")
		if err := r.patchMaintenanceRef(ctx, settings, nil); err != nil {
			return fmt.Errorf("failed to remove the ServerMaintenance reference in BIOSSettings status: %w", err)
		}
		if err := r.updateStatus(ctx, settings, settings.Status.State, condition); err != nil {
			return fmt.Errorf("failed to patch BIOSSettings conditions: %w", err)
		}
	}
	return nil
}

func (r *BIOSSettingsReconciler) cleanupReferences(ctx context.Context, settings *metalv1alpha1.BIOSSettings) (err error) {
	log := ctrl.LoggerFrom(ctx)
	if settings.Spec.ServerRef == nil {
		log.V(1).Info("BIOSSettings does not have a ServerRef")
		return nil
	}

	server, err := GetServerByName(ctx, r.Client, settings.Spec.ServerRef.Name)
	if apierrors.IsNotFound(err) {
		log.V(1).Info("Referred Server is gone")
		return nil
	}
	if err != nil {
		return err
	}

	if server.Spec.BIOSSettingsRef == nil {
		log.V(1).Info("Server does not have a BIOSSettingsRef")
		return nil
	}

	if server.Spec.BIOSSettingsRef.Name != settings.Name {
		return nil
	}

	return r.patchBIOSSettingsRefForServer(ctx, server, nil)
}

func (r *BIOSSettingsReconciler) reconcile(ctx context.Context, settings *metalv1alpha1.BIOSSettings) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if shouldIgnoreReconciliation(settings) {
		log.V(1).Info("Skipped BIOSSettings reconciliation")
		return ctrl.Result{}, nil
	}

	base := settings.DeepCopy()
	if settings.Spec.ServerMaintenanceRef != nil && clearDeprecatedObjectRefFields(settings.Spec.ServerMaintenanceRef) {
		if err := r.Patch(ctx, settings, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to clear deprecated ObjectReference fields on BIOSSettings: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if settings.Spec.ServerRef == nil {
		log.V(1).Info("BIOSSettings does not have a ServerRef")
		return ctrl.Result{}, nil
	}

	server, err := GetServerByName(ctx, r.Client, settings.Spec.ServerRef.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Server not found", "ServerName", settings.Spec.ServerRef.Name)
			if err := r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStatePending, nil); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if server.Spec.BIOSSettingsRef == nil {
		if err := r.patchBIOSSettingsRefForServer(ctx, server, settings); err != nil {
			return ctrl.Result{}, err
		}
	} else if server.Spec.BIOSSettingsRef.Name != settings.Name {
		referredBIOSSetting, err := r.getBIOSSettingsByName(ctx, server.Spec.BIOSSettingsRef.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.V(1).Info("Referred server contained reference to non-existing BIOSSettings object, updated reference")
				if err := r.patchBIOSSettingsRefForServer(ctx, server, settings); err != nil {
					return ctrl.Result{}, err
				}
				// Need to requeue to make sure that reconcile re-happens here. Updating server object does not trigger reconcile here.
				return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
			}
			log.V(1).Info("Server contained a reference to a different BIOSSettings object", "BIOSSettings", server.Spec.BIOSSettingsRef.Name)
			return ctrl.Result{}, err
		}
		// Check if the current BIOSSettings version is newer and update reference if it is newer
		// TODO: Handle version checks correctly
		if referredBIOSSetting.Spec.Version < settings.Spec.Version {
			log.V(1).Info("Updated BIOSSettings reference to the latest BIOS version")
			if err := r.patchBIOSSettingsRefForServer(ctx, server, settings); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, settings, BIOSSettingsFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.DefaultProtocol, r.SkipCertValidation, r.BMCOptions)
	if err != nil {
		if errors.As(err, &bmcutils.BMCUnAvailableError{}) {
			log.V(1).Info("BMC is not available", "BMC", server.Spec.BMCRef.Name, "Server", server.Name, "Message", err.Error())
			return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client for server: %w", err)
	}
	defer bmcClient.Logout()

	return r.ensureBIOSSettingsStateTransition(ctx, bmcClient, settings, server)
}

func (r *BIOSSettingsReconciler) ensureBIOSSettingsStateTransition(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	switch settings.Status.State {
	case "", metalv1alpha1.BIOSSettingsStatePending:
		return r.handleSettingPendingState(ctx, bmcClient, settings, server)
	case metalv1alpha1.BIOSSettingsStateInProgress:
		return r.handleSettingInProgressState(ctx, bmcClient, settings, server)
	case metalv1alpha1.BIOSSettingsStateApplied:
		return r.handleAppliedState(ctx, bmcClient, settings, server)
	case metalv1alpha1.BIOSSettingsStateFailed:
		return r.handleFailedState(ctx, settings, server)
	default:
		return ctrl.Result{}, fmt.Errorf("invalid BIOSSettings state: %s", settings.Status.State)
	}
}

func (r *BIOSSettingsReconciler) handleSettingPendingState(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if len(settings.Spec.SettingsFlow) == 0 {
		log.V(1).Info("Skipped BIOSSettings because no settings flow found")
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateApplied, nil)
	}

	pendingSettings, err := r.getPendingBIOSSettings(ctx, bmcClient, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}

	if len(pendingSettings) > 0 {
		log.V(1).Info("Pending BIOS setting tasks found", "TaskCount", len(pendingSettings))

		condition, err := GetCondition(r.Conditions, settings.Status.Conditions, ConditionPendingSettingsCheck)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get condition for pending BIOSSettings state: %w", err)
		}

		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonPendingSettings),
			conditionutils.UpdateMessage(fmt.Sprintf("Found pending BIOS settings (%d)", len(pendingSettings))),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update pending BIOSSettings condition: %w", err)
		}

		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, condition)
	}

	// Verify that no duplicate names or duplicate settings keys are found
	allNames := map[string]struct{}{}
	allSettingsNames := map[string]struct{}{}
	duplicateNames := make([]string, 0, len(settings.Spec.SettingsFlow))
	var duplicateSettingsKeys []string
	for _, flowItem := range settings.Spec.SettingsFlow {
		if _, ok := allNames[flowItem.Name]; ok {
			duplicateNames = append(duplicateNames, flowItem.Name)
		}
		allNames[flowItem.Name] = struct{}{}
		for key := range flowItem.Settings {
			if _, ok := allSettingsNames[key]; ok {
				duplicateSettingsKeys = append(duplicateSettingsKeys, key)
			}
			allSettingsNames[key] = struct{}{}
		}
	}

	if len(duplicateNames) > 0 || len(duplicateSettingsKeys) > 0 {
		log.V(1).Info("Found duplicate keys", "DuplicateNameCount", len(duplicateNames), "DuplicateSettingsCount", len(duplicateSettingsKeys))
		condition, err := GetCondition(r.Conditions, settings.Status.Conditions, ConditionDuplicateKeys)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get condition for duplicate keys: %w", err)
		}
		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonDuplicateKeys),
			conditionutils.UpdateMessage(fmt.Sprintf("Found duplicate names (%d) and settings keys (%d)", len(duplicateNames), len(duplicateSettingsKeys))),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update duplicate keys condition: %w", err)
		}
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, condition)
	}

	// Check if all settings have been applied
	biosVersion, settingsDiff, err := r.getBIOSVersionAndSettingsDiff(ctx, bmcClient, settings, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOSSettings: %w", err)
	}
	// If settings match, complete the BIOS tasks regardless of BIOS version.
	// If conditions are present, skip this shortcut to capture all condition states (e.g. verifySetting, reboot).
	if len(settingsDiff) == 0 && len(settings.Status.Conditions) == 0 {
		condition, err := GetCondition(r.Conditions, settings.Status.Conditions, ConditionSettingsVerified)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get condition for verified BIOSSettings: %w", err)
		}
		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonVerified),
			conditionutils.UpdateMessage("Required BIOS settings have been verified on the server"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update verify BIOSSettings condition: %w", err)
		}
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateApplied, condition)
	}

	var state = metalv1alpha1.BIOSSettingsStateInProgress
	var condition *metav1.Condition
	if biosVersion != settings.Spec.Version {
		versionCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, ConditionBIOSVersionUpgrade)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get condition for pending BIOS version update: %w", err)
		}
		if versionCondition.Status == metav1.ConditionTrue {
			log.V(1).Info("Pending BIOS version upgrade", "currentVersion", biosVersion, "requiredVersion", settings.Spec.Version)
			return ctrl.Result{}, nil
		}
		if err := r.Conditions.Update(
			versionCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonVersionMismatch),
			conditionutils.UpdateMessage(fmt.Sprintf("Waiting to update BIOS version: %s, current version: %s", settings.Spec.Version, biosVersion)),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update pending BIOS version condition: %w", err)
		}
		state = metalv1alpha1.BIOSSettingsStatePending
		condition = versionCondition
	}
	return ctrl.Result{}, r.updateStatus(ctx, settings, state, condition)
}

func (r *BIOSSettingsReconciler) handleSettingInProgressState(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if result, err := r.requestMaintenanceForServer(ctx, settings, server); result.RequeueAfter > 0 || err != nil {
		return result, err
	}

	condition, err := GetCondition(r.Conditions, settings.Status.Conditions, ServerMaintenanceConditionWaiting)
	if err != nil {
		return ctrl.Result{}, err
	}
	if ok := r.isServerInMaintenance(ctx, settings, server); !ok {
		log.V(1).Info("Server is not yet in Maintenance status, skipping")
		if condition.Status != metav1.ConditionTrue {
			if err := r.Conditions.Update(
				condition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(ServerMaintenanceReasonWaiting),
				conditionutils.UpdateMessage(fmt.Sprintf("Waiting for approval of %v", settings.Spec.ServerMaintenanceRef.Name)),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update ServerMaintenance waiting condition: %w", err)
			}
			if err := r.updateStatus(ctx, settings, settings.Status.State, condition); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to patch BIOSSettings conditions: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}
	// Once in maintenance, clear the waiting condition if present
	if condition.Reason != ServerMaintenanceReasonApproved {
		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(ServerMaintenanceReasonApproved),
			conditionutils.UpdateMessage("Server is now in Maintenance mode"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update ServerMaintenance approved condition: %w", err)
		}
		if err := r.updateStatus(ctx, settings, settings.Status.State, condition); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch BIOSSettings conditions: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if result, err := r.handleBMCReset(ctx, bmcClient, settings, server); result.RequeueAfter > 0 || err != nil {
		return result, err
	}

	settingsFlow := append([]metalv1alpha1.SettingsFlowItem{}, settings.Spec.SettingsFlow...)

	sort.Slice(settingsFlow, func(i, j int) bool {
		return settingsFlow[i].Priority < settingsFlow[j].Priority
	})

	// Loop through all the sequence in priority order and verify/apply the settings
	for _, settingsFlowItem := range settingsFlow {
		currentSettingsFlowStatus := r.getFlowItemFromSettingsStatus(settings, &settingsFlowItem)

		result, err := r.processFlowItem(ctx, bmcClient, settings, &settingsFlowItem, currentSettingsFlowStatus, server)
		if result.RequeueAfter > 0 || err != nil {
			return result, err
		}
	}

	return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateApplied, nil)
}

func (r *BIOSSettingsReconciler) processFlowItem(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// If the flow status does not exist yet, create it
	if flowStatus == nil {
		flowStatus = &metalv1alpha1.BIOSSettingsFlowStatus{
			Priority: flowItem.Priority,
			Name:     flowItem.Name,
		}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateInProgress, flowStatus, nil)
	}

	// If not InProgress, check whether settings still match or need re-application
	if flowStatus.State != metalv1alpha1.BIOSSettingsFlowStateInProgress {
		settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get current BIOS settings difference: %w", err)
		}
		if len(settingsDiff) > 0 {
			log.V(1).Info("Found BIOSSettings difference on Server", "Server", server.Name, "SettingsDifference", settingsDiff)
		}

		if len(settingsDiff) == 0 {
			if flowStatus.State == metalv1alpha1.BIOSSettingsFlowStateApplied {
				return ctrl.Result{}, nil // Already applied, move on
			}
			// Mark completed and move on
			condition, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionSettingsVerified)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to get condition for verified BIOSSettings: %w", err)
			}
			if err := r.Conditions.Update(
				condition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(ReasonVerified),
				conditionutils.UpdateMessage("Required BIOS settings have been re-verified on the server"),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update verified BIOSSettings condition: %w", err)
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateApplied, flowStatus, condition)
		}

		// Settings differ and previously applied — need to reapply
		if flowStatus.State == metalv1alpha1.BIOSSettingsFlowStateApplied {
			return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateInProgress, flowStatus, nil)
		}
	}

	// Apply settings and verify
	result, err := r.applySettingUpdate(ctx, bmcClient, settings, flowItem, flowStatus, server)
	if result.RequeueAfter > 0 || err != nil {
		return result, err
	}

	return r.verifySettingsUpdateComplete(ctx, bmcClient, settings, flowItem, flowStatus, server)
}

func (r *BIOSSettingsReconciler) handleBMCReset(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	resetBMC, err := GetCondition(r.Conditions, settings.Status.Conditions, BMCConditionReset)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get condition for BMC reset: %w", err)
	}

	if resetBMC.Status != metav1.ConditionTrue {
		// Reset the BMC to make sure it is in a stable state.
		// This avoids problems with some BMCs that hang up in subsequent operations.
		if resetBMC.Reason != BMCReasonReset {
			if err := resetBMCOfServer(ctx, r.Client, server, bmcClient); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reset BMC: %w", err)
			}
			if err := r.Conditions.Update(
				resetBMC,
				conditionutils.UpdateStatus(corev1.ConditionFalse),
				conditionutils.UpdateReason(BMCReasonReset),
				conditionutils.UpdateMessage("Issued BMC reset to stabilize BMC of the server"),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update BMC reset condition: %w", err)
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateStatus(ctx, settings, settings.Status.State, resetBMC)
		} else if server.Spec.BMCRef != nil {
			// Wait until the BMC resource annotation is removed
			bmcObj := &metalv1alpha1.BMC{}
			if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.BMCRef.Name}, bmcObj); err != nil {
				return ctrl.Result{}, err
			}
			annotations := bmcObj.GetAnnotations()
			if op, ok := annotations[metalv1alpha1.OperationAnnotation]; ok {
				if op == metalv1alpha1.GracefulRestartBMC {
					log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
					return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
				}
			}
		}
		if err := r.Conditions.Update(
			resetBMC,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BMCReasonReset),
			conditionutils.UpdateMessage("BMC reset to stabilize BMC of the server is completed"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update BMC reset completed condition: %w", err)
		}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateStatus(ctx, settings, settings.Status.State, resetBMC)
	}
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) determineRebootRequirement(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Checking if current settings require server reboot")
	settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings difference: %w", err)
	}
	if len(settingsDiff) > 0 {
		log.V(1).Info("Found BIOSSettings difference on server", "Server", server.Name, "SettingsDifference", settingsDiff)
	}

	resetReq, err := bmcClient.CheckBiosAttributes(settingsDiff)
	if err != nil {
		log.Error(err, "Could not validate settings and determine if reboot is needed")
		var invalidSettingsErr *bmc.InvalidBIOSSettingsError
		if errors.As(err, &invalidSettingsErr) {
			invalidSettings, errCond := GetCondition(r.Conditions, flowStatus.Conditions, ConditionSettingsInvalid)
			if errCond != nil {
				return ctrl.Result{}, fmt.Errorf("failed to get condition for invalid settings: %w", errCond)
			}
			if errCond := r.Conditions.Update(
				invalidSettings,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(ReasonInvalidSettings),
				conditionutils.UpdateMessage(fmt.Sprintf("Settings provided are invalid: %v", err)),
			); errCond != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update invalid settings condition: %w", errCond)
			}
			err = r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateFailed, flowStatus, invalidSettings)
			return ctrl.Result{}, errors.Join(err, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, nil))
		}
		return ctrl.Result{}, err
	}

	rebootCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionRebootRequired)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get condition for reboot requirement: %w", err)
	}

	// ConditionTrue = reboot IS required, ConditionFalse = reboot NOT required
	if resetReq {
		log.V(1).Info("BIOSSettings update requires server reboot")
		if err := r.Conditions.Update(
			rebootCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonRebootRequired),
			conditionutils.UpdateMessage("Settings provided require server reboot"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update reboot required condition: %w", err)
		}
	} else {
		log.V(1).Info("BIOSSettings update does not require reboot")
		if err := r.Conditions.Update(
			rebootCondition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
			conditionutils.UpdateReason(ReasonRebootNotRequired),
			conditionutils.UpdateMessage("Settings provided do not require server reboot"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update reboot not required condition: %w", err)
		}
	}
	log.V(1).Info("Determined reboot requirement for BIOSSettings")
	return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, rebootCondition)
}

func (r *BIOSSettingsReconciler) applySettingUpdate(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if result, err := r.setTimeoutForAppliedSettings(ctx, settings, flowStatus); result.RequeueAfter > 0 || err != nil {
		return result, err
	}

	// Ensure server is powered on
	turnOnServer, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionServerPowerOn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get condition for initial power on of server: %w", err)
	}

	if turnOnServer.Status != metav1.ConditionTrue {
		if r.isServerInPowerState(server, metalv1alpha1.ServerOnPowerState) {
			if err := r.Conditions.Update(
				turnOnServer,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(ReasonPoweredOn),
				conditionutils.UpdateMessage("Server is powered on to start the BIOS update process"),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update power on server condition: %w", err)
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, turnOnServer)
		}
		// Request maintenance to get the server powered on
		if settings.Spec.ServerMaintenanceRef == nil {
			log.V(1).Info("Server powered off, requesting maintenance to turn the server on")
			if result, err := r.requestMaintenanceForServer(ctx, settings, server); result.RequeueAfter > 0 || err != nil {
				return result, err
			}
		}

		if err := r.patchPowerState(ctx, settings, metalv1alpha1.PowerOn); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to power on server: %w", err)
		}
		log.V(1).Info("Reconciled BIOSSettings at TurnOnServer step")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	// Check if we have already determined whether reboot is needed
	condFound, err := r.Conditions.FindSlice(flowStatus.Conditions, ConditionRebootRequired, &metav1.Condition{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to find condition %v: %w", ConditionRebootRequired, err)
	}

	if !condFound {
		return r.determineRebootRequirement(ctx, bmcClient, settings, flowItem, flowStatus, server)
	}

	// Issue the BIOS settings update
	issueBiosUpdate, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionSettingsUpdateIssued)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get condition for issuing BIOSSettings update: %w", err)
	}

	if issueBiosUpdate.Status != metav1.ConditionTrue {
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.applyBIOSSettings(ctx, bmcClient, settings, flowItem, flowStatus, server, issueBiosUpdate)
	}

	// Handle reboot if required (ConditionTrue = reboot needed)
	rebootCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionRebootRequired)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get condition for reboot requirement: %w", err)
	}

	if rebootCondition.Status == metav1.ConditionTrue {
		rebootPowerOnCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionRebootPowerOn)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get condition for reboot power on: %w", err)
		}
		// Reboot is not yet completed
		if rebootPowerOnCondition.Status != metav1.ConditionTrue {
			return ctrl.Result{RequeueAfter: 1 * time.Second}, r.rebootServer(ctx, settings, flowStatus, server)
		}
	}
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) setTimeoutForAppliedSettings(ctx context.Context, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	timeoutCheck, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionUpdateStarted)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get condition for update timeout: %w", err)
	}
	if timeoutCheck.Status != metav1.ConditionTrue {
		if err := r.Conditions.Update(
			timeoutCheck,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonUpdateStarted),
			conditionutils.UpdateMessage("Settings are being updated on Server. Timeout will occur beyond this point if settings are not applied"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update update started condition: %w", err)
		}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, timeoutCheck)
	}

	startTime := timeoutCheck.LastTransitionTime.Time
	if time.Now().After(startTime.Add(r.TimeoutExpiry)) {
		log.V(1).Info("Timed out while updating the BIOSSettings")
		timedOut, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionUpdateTimedOut)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get condition for update timeout: %w", err)
		}
		if err := r.Conditions.Update(
			timedOut,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonTimedOut),
			conditionutils.UpdateMessage(fmt.Sprintf("Timeout after: %v. startTime: %v. timedOut on: %v", r.TimeoutExpiry, startTime, time.Now().String())),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update timeout condition: %w", err)
		}
		err = r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateFailed, flowStatus, timedOut)
		return ctrl.Result{}, errors.Join(err, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, nil))
	}
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) verifySettingsUpdateComplete(ctx context.Context, bmcClient bmc.BMC, biosSettings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	verifyCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionSettingsVerified)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get condition for verification: %w", err)
	}

	if verifyCondition.Status != metav1.ConditionTrue {
		settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings diff: %w", err)
		}

		if len(settingsDiff) == 0 {
			if err := r.Conditions.Update(
				verifyCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(ReasonVerified),
				conditionutils.UpdateMessage("Required BIOS settings have been applied and verified on the server"),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update verified BIOSSettings condition: %w", err)
			}
			log.V(1).Info("Verified BIOS setting sequence", "Name", flowStatus.Name)
			return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateFlowStatus(ctx, biosSettings, metalv1alpha1.BIOSSettingsFlowStateApplied, flowStatus, verifyCondition)
		}

		log.V(1).Info("Waited for the BIOS setting to take effect")
		if verifyCondition.Reason != ReasonNotYetVerified {
			if err := r.Conditions.Update(
				verifyCondition,
				conditionutils.UpdateStatus(corev1.ConditionFalse),
				conditionutils.UpdateReason(ReasonNotYetVerified),
				conditionutils.UpdateMessage("Required BIOS settings have not yet been verified on the server"),
			); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update not yet verified BIOSSettings condition: %w", err)
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, r.updateFlowStatus(ctx, biosSettings, flowStatus.State, flowStatus, verifyCondition)
		}
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
	}

	log.V(1).Info("BIOS settings have been applied and verified on the server", "SettingsFlow", flowItem.Name)
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) rebootServer(ctx context.Context, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) error {
	log := ctrl.LoggerFrom(ctx)
	rebootPowerOffCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionRebootPowerOff)
	if err != nil {
		return fmt.Errorf("failed to get PowerOff condition: %w", err)
	}

	if rebootPowerOffCondition.Status != metav1.ConditionTrue {
		if r.isServerInPowerState(server, metalv1alpha1.ServerOnPowerState) {
			if err := r.patchPowerState(ctx, settings, metalv1alpha1.PowerOff); err != nil {
				return fmt.Errorf("failed to power off server for reboot: %w", err)
			}
		}
		if r.isServerInPowerState(server, metalv1alpha1.ServerOffPowerState) {
			if err := r.Conditions.Update(
				rebootPowerOffCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(ReasonPoweredOff),
				conditionutils.UpdateMessage("Server has entered power off state"),
			); err != nil {
				return fmt.Errorf("failed to update powerOff condition: %w", err)
			}
			return r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, rebootPowerOffCondition)
		}
		log.V(1).Info("Waited for server to power off", "Server", server.Name)
		return nil
	}

	rebootPowerOnCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionRebootPowerOn)
	if err != nil {
		return fmt.Errorf("failed to get PowerOn condition: %w", err)
	}

	if rebootPowerOnCondition.Status != metav1.ConditionTrue {
		if r.isServerInPowerState(server, metalv1alpha1.ServerOffPowerState) {
			if err := r.patchPowerState(ctx, settings, metalv1alpha1.PowerOn); err != nil {
				return fmt.Errorf("failed to power on server for reboot: %w", err)
			}
		}
		if r.isServerInPowerState(server, metalv1alpha1.ServerOnPowerState) {
			if err := r.Conditions.Update(
				rebootPowerOnCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(ReasonPoweredOn),
				conditionutils.UpdateMessage("Server has entered power on state"),
			); err != nil {
				return fmt.Errorf("failed to update reboot powerOn condition: %w", err)
			}
			return r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, rebootPowerOnCondition)
		}
		log.V(1).Info("Waited for server to power on", "Server", server.Name)
		return nil
	}

	return nil
}

func (r *BIOSSettingsReconciler) applyBIOSSettings(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server, issueBiosUpdate *metav1.Condition) error {
	log := ctrl.LoggerFrom(ctx)
	settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
	if err != nil {
		return fmt.Errorf("failed to get BIOS settings difference: %w", err)
	}

	if len(settingsDiff) == 0 {
		log.V(1).Info("No BIOS settings difference found to apply on server", "settingsName", flowItem.Name)
		if err := r.Conditions.Update(
			issueBiosUpdate,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonUpdateSkipped),
			conditionutils.UpdateMessage("BIOS settings update skipped on the server as no difference found"),
		); err != nil {
			return fmt.Errorf("failed to update settings update skipped condition: %w", err)
		}
		err = r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, issueBiosUpdate)
		log.V(1).Info("Skipped BIOSSettings update as no difference found", "SettingsFlow", flowItem.Name)
		return err
	}

	// Verify no pending settings exist before issuing new ones
	pendingSettings, err := r.getPendingBIOSSettings(ctx, bmcClient, server)
	if err != nil {
		return fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}
	if len(pendingSettings) > 0 {
		return fmt.Errorf("pending settings found on BIOS, cannot issue new settings update. pending settings: %v", pendingSettings)
	}

	log.V(1).Info("Issued settings update on BMC", "settingsDiff", settingsDiff, "SettingsName", flowItem.Name)
	if err := bmcClient.SetBiosAttributesOnReset(ctx, server.Spec.SystemURI, settingsDiff); err != nil {
		return fmt.Errorf("failed to set BMC settings: %w", err)
	}

	// Get the latest pending settings and verify they match our desired settings
	pendingSettings, err = r.getPendingBIOSSettings(ctx, bmcClient, server)
	if err != nil {
		return fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}

	rebootCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionRebootRequired)
	if err != nil {
		return fmt.Errorf("failed to get condition for reboot requirement: %w", err)
	}

	// If reboot is required but no pending settings, the BMC did not accept the update
	if len(pendingSettings) == 0 && rebootCondition.Status == metav1.ConditionTrue {
		log.V(1).Info("BIOSSettings update issued to BMC was not accepted, will retry")
		return fmt.Errorf("BIOS setting issued to BMC not accepted")
	}

	// Re-fetch the settings diff: attributes that were applied immediately (no reboot)
	// will no longer appear, so only attributes still outstanding remain.
	settingsDiff, err = r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
	if err != nil {
		return fmt.Errorf("failed to re-fetch BIOS settings difference: %w", err)
	}

	// Verify pending settings match what we requested
	pendingSettingsDiff := make(schemas.SettingsAttributes, len(settingsDiff))
	for name, value := range settingsDiff {
		pendingValue, ok := pendingSettings[name]
		switch {
		case !ok:
			pendingSettingsDiff[name] = "<missing>"
		case value != pendingValue:
			pendingSettingsDiff[name] = pendingValue
		}
	}

	if len(pendingSettingsDiff) > 0 {
		log.V(1).Info("Difference between pending settings and required settings", "SettingsDiff", pendingSettingsDiff)
		unexpectedCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, ConditionUnexpectedPendingSettings)
		if err != nil {
			return fmt.Errorf("failed to get condition for unexpected pending settings: %w", err)
		}
		if err := r.Conditions.Update(
			unexpectedCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ReasonUnexpectedPending),
			conditionutils.UpdateMessage(fmt.Sprintf("Found unexpected settings after issuing update: %v", pendingSettingsDiff)),
		); err != nil {
			return fmt.Errorf("failed to update unexpected pending settings condition: %w", err)
		}
		err = r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateFailed, flowStatus, unexpectedCondition)
		return errors.Join(err, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, nil))
	}

	if err := r.Conditions.Update(
		issueBiosUpdate,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(ReasonUpdateIssued),
		conditionutils.UpdateMessage("BIOS settings update has been triggered on the server"),
	); err != nil {
		return fmt.Errorf("failed to update issued BIOSSettings update condition: %w", err)
	}

	return r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, issueBiosUpdate)
}

func (r *BIOSSettingsReconciler) ensureNoStrandedStatus(ctx context.Context, settings *metalv1alpha1.BIOSSettings) (bool, error) {
	// In case the settings Spec got changed during InProgress and left behind stale states, clean them up.
	settingsNamePriorityMap := map[string]int32{}
	settingsBase := settings.DeepCopy()
	for _, flowItem := range settings.Spec.SettingsFlow {
		settingsNamePriorityMap[flowItem.Name] = flowItem.Priority
	}

	nextFlowStatuses := make([]metalv1alpha1.BIOSSettingsFlowStatus, 0)
	for _, flowStatus := range settings.Status.FlowState {
		if value, ok := settingsNamePriorityMap[flowStatus.Name]; ok && value == flowStatus.Priority {
			nextFlowStatuses = append(nextFlowStatuses, flowStatus)
		}
	}

	if len(nextFlowStatuses) != len(settings.Status.FlowState) {
		settings.Status.FlowState = nextFlowStatuses
		if err := r.Status().Patch(ctx, settings, client.MergeFrom(settingsBase)); err != nil {
			return false, fmt.Errorf("failed to patch BIOSSettings FlowState status: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func (r *BIOSSettingsReconciler) handleAppliedState(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if err := r.removeServerMaintenance(ctx, settings); err != nil {
		return ctrl.Result{}, err
	}

	if requeue, err := r.ensureNoStrandedStatus(ctx, settings); requeue || err != nil {
		return ctrl.Result{}, err
	}

	_, settingsDiff, err := r.getBIOSVersionAndSettingsDiff(ctx, bmcClient, settings, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOS version and settings diff: %w", err)
	}
	if len(settingsDiff) > 0 {
		log.V(1).Info("Found BIOS setting difference after applied state", "SettingsDiff", settingsDiff)
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStatePending, nil)
	}

	log.V(1).Info("Finished BIOSSettings update", "Server", server.Name)
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) handleFailedState(ctx context.Context, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if shouldRetryReconciliation(settings) {
		log.V(1).Info("Retried reconciliation")
		biosSettingsBase := settings.DeepCopy()
		settings.Status.State = metalv1alpha1.BIOSSettingsStatePending
		settings.Status.FlowState = []metalv1alpha1.BIOSSettingsFlowStatus{}
		settings.Status.Conditions = []metav1.Condition{}
		annotations := settings.GetAnnotations()
		delete(annotations, metalv1alpha1.OperationAnnotation)
		settings.SetAnnotations(annotations)

		if err := r.Status().Patch(ctx, settings, client.MergeFrom(biosSettingsBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch BIOSSettings status for retrying: %w", err)
		}
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Failed to update BIOS settings", "Server", server.Name)

	// Create maintenance object only if failure is due to pending settings and maintenance not already present
	if settings.Spec.ServerMaintenanceRef == nil {
		for _, condition := range settings.Status.Conditions {
			if condition.Type == ConditionPendingSettingsCheck && condition.Status == metav1.ConditionTrue {
				if _, err := r.requestMaintenanceForServer(ctx, settings, server); err != nil {
					return ctrl.Result{}, err
				}
				break
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) getPendingBIOSSettings(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (schemas.SettingsAttributes, error) {
	if server == nil {
		return schemas.SettingsAttributes{}, fmt.Errorf("server is nil")
	}
	return bmcClient.GetBiosPendingAttributeValues(ctx, server.Spec.SystemURI)
}

func (r *BIOSSettingsReconciler) getSettingsDiff(ctx context.Context, bmcClient bmc.BMC, settings map[string]string, server *metalv1alpha1.Server) (schemas.SettingsAttributes, error) {
	keys := slices.Collect(maps.Keys(settings))

	actualSettings, err := bmcClient.GetBiosAttributeValues(ctx, server.Spec.SystemURI, keys)
	if err != nil {
		return schemas.SettingsAttributes{}, fmt.Errorf("failed to get BIOSSettings: %w", err)
	}

	return computeSettingsDiff(settings, actualSettings)
}

func (r *BIOSSettingsReconciler) getBIOSVersionAndSettingsDiff(ctx context.Context, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (string, schemas.SettingsAttributes, error) {
	log := ctrl.LoggerFrom(ctx)
	completeSettings := make(map[string]string)
	for _, flowItem := range settings.Spec.SettingsFlow {
		maps.Copy(completeSettings, flowItem.Settings)
	}

	diff, err := r.getSettingsDiff(ctx, bmcClient, completeSettings, server)
	if err != nil {
		return "", diff, fmt.Errorf("failed to find BIOS settings difference: %w", err)
	}
	if len(diff) > 0 {
		log.V(1).Info("Found BIOSSettings difference on Server", "Server", server.Name, "SettingsDifference", diff)
	}

	version, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemURI)
	if err != nil {
		return "", diff, fmt.Errorf("failed to get BIOS version: %w", err)
	}

	return version, diff, nil
}

func (r *BIOSSettingsReconciler) isServerInPowerState(server *metalv1alpha1.Server, state metalv1alpha1.ServerPowerState) bool {
	return server.Status.PowerState == state
}

func (r *BIOSSettingsReconciler) isServerInMaintenance(ctx context.Context, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) bool {
	log := ctrl.LoggerFrom(ctx)
	if settings.Spec.ServerMaintenanceRef == nil {
		return false
	}

	if server.Status.State == metalv1alpha1.ServerStateMaintenance {
		if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.Name != settings.Spec.ServerMaintenanceRef.Name || server.Spec.ServerMaintenanceRef.Namespace != settings.Spec.ServerMaintenanceRef.Namespace {
			log.V(1).Info("Server is already in maintenance", "Server", server.Name)
			return false
		}
	} else {
		log.V(1).Info("Server not yet in maintenance", "Server", server.Name, "State", server.Status.State)
		return false
	}

	return true
}

func (r *BIOSSettingsReconciler) requestMaintenanceForServer(ctx context.Context, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if settings.Spec.ServerMaintenanceRef != nil {
		// Verify the referenced ServerMaintenance still exists
		if _, err := GetServerMaintenanceForObjectReference(ctx, r.Client, settings.Spec.ServerMaintenanceRef); apierrors.IsNotFound(err) {
			log.V(1).Info("Referenced ServerMaintenance no longer existed, cleared ref to allow re-creation")
			if err := r.patchMaintenanceRef(ctx, settings, nil); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to clear stale ServerMaintenance ref: %w", err)
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		} else if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to verify ServerMaintenance existence: %w", err)
		}
		condition, err := GetCondition(r.Conditions, settings.Status.Conditions, ServerMaintenanceConditionCreated)
		if err != nil {
			return ctrl.Result{}, err
		}
		if condition.Status == metav1.ConditionTrue {
			log.V(1).Info("ServerMaintenance already present for BIOSSettings", "ServerMaintenance", settings.Spec.ServerMaintenanceRef.Name)
			return ctrl.Result{}, nil
		}
		log.V(1).Info("ServerMaintenance present for BIOSSettings, updated condition")
		if err := r.Conditions.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(ServerMaintenanceReasonCreated),
			conditionutils.UpdateMessage(fmt.Sprintf("Created/present %v at %v", settings.Spec.ServerMaintenanceRef.Name, time.Now())),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update ServerMaintenance created condition: %w", err)
		}
		if err := r.updateStatus(ctx, settings, settings.Status.State, condition); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch BIOSSettings conditions: %w", err)
		}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	serverMaintenance := &metalv1alpha1.ServerMaintenance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.ManagerNamespace,
			Name:      settings.Name,
		}}

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
		serverMaintenance.Spec.Policy = settings.Spec.ServerMaintenancePolicy
		serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
		serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
		if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
			serverMaintenance.Status.State = ""
		}
		return controllerutil.SetControllerReference(settings, serverMaintenance, r.Client.Scheme())
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create or patch ServerMaintenance: %w", err)
	}
	log.V(1).Info("Created/Patched ServerMaintenance", "ServerMaintenance", serverMaintenance.Name, "Operation", opResult)

	if err := r.patchMaintenanceRef(ctx, settings, serverMaintenance); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch ServerMaintenance ref in BIOSSettings: %w", err)
	}

	log.V(1).Info("Patched ServerMaintenance reference on BIOSSettings")
	return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
}

func (r *BIOSSettingsReconciler) getBIOSSettingsByName(ctx context.Context, name string) (*metalv1alpha1.BIOSSettings, error) {
	biosSettings := &metalv1alpha1.BIOSSettings{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, biosSettings); err != nil {
		return nil, fmt.Errorf("failed to get referred BIOSSettings: %w", err)
	}
	return biosSettings, nil
}

func (r *BIOSSettingsReconciler) patchBIOSSettingsRefForServer(ctx context.Context, server *metalv1alpha1.Server, settings *metalv1alpha1.BIOSSettings) error {
	if server == nil {
		return nil
	}

	current := server.Spec.BIOSSettingsRef
	if settings != nil && current != nil && current.Name == settings.Name {
		return nil
	}

	serverBase := server.DeepCopy()
	if settings == nil {
		server.Spec.BIOSSettingsRef = nil
	} else {
		server.Spec.BIOSSettingsRef = &corev1.LocalObjectReference{Name: settings.Name}
	}

	return r.Patch(ctx, server, client.MergeFrom(serverBase))
}

func (r *BIOSSettingsReconciler) patchMaintenanceRef(ctx context.Context, settings *metalv1alpha1.BIOSSettings, maintenance *metalv1alpha1.ServerMaintenance) error {
	biosSettingsBase := settings.DeepCopy()

	if maintenance == nil {
		settings.Spec.ServerMaintenanceRef = nil
	} else {
		settings.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{
			Namespace: maintenance.Namespace,
			Name:      maintenance.Name,
		}
	}
	if err := r.Patch(ctx, settings, client.MergeFrom(biosSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch ServerMaintenance ref in BIOSSettings: %w", err)
	}

	return nil
}

func (r *BIOSSettingsReconciler) patchPowerState(ctx context.Context, settings *metalv1alpha1.BIOSSettings, powerState metalv1alpha1.Power) error {
	log := ctrl.LoggerFrom(ctx)
	if settings == nil {
		return fmt.Errorf("BIOSSettings is nil")
	}

	serverMaintenance, err := GetServerMaintenanceForObjectReference(ctx, r.Client, settings.Spec.ServerMaintenanceRef)
	if err != nil {
		return err
	}
	if serverMaintenance.Spec.ServerPower == powerState {
		return nil
	}

	serverMaintenanceBase := serverMaintenance.DeepCopy()
	serverMaintenance.Spec.ServerPower = powerState
	if err := r.Patch(ctx, serverMaintenance, client.MergeFrom(serverMaintenanceBase)); err != nil {
		return fmt.Errorf("failed to patch power for ServerMaintenance: %w", err)
	}

	log.V(1).Info("Patched desired Power of the ServerMaintenance", "Server", serverMaintenance.Spec.ServerRef.Name, "PowerState", powerState)
	return nil
}

func (r *BIOSSettingsReconciler) updateFlowStatus(ctx context.Context, settings *metalv1alpha1.BIOSSettings, state metalv1alpha1.BIOSSettingsFlowState, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, condition *metav1.Condition) error {
	if flowStatus == nil || (flowStatus.State == state && condition == nil) {
		return nil
	}

	biosSettingsBase := settings.DeepCopy()

	currentIdx := -1
	for idx, status := range settings.Status.FlowState {
		if status.Priority == flowStatus.Priority && status.Name == flowStatus.Name {
			settings.Status.FlowState[idx].State = state
			if state == metalv1alpha1.BIOSSettingsFlowStateApplied {
				now := metav1.Now()
				settings.Status.FlowState[idx].LastAppliedTime = &now
			}
			if condition != nil {
				if err := r.Conditions.UpdateSlice(
					&settings.Status.FlowState[idx].Conditions,
					condition.Type,
					conditionutils.UpdateStatus(condition.Status),
					conditionutils.UpdateReason(condition.Reason),
					conditionutils.UpdateMessage(condition.Message),
				); err != nil {
					return fmt.Errorf("failed to patch BIOSSettings condition: %w", err)
				}
			} else {
				settings.Status.FlowState[idx].Conditions = []metav1.Condition{}
			}
			currentIdx = idx
			continue
		} else if state == metalv1alpha1.BIOSSettingsFlowStateInProgress &&
			status.State == metalv1alpha1.BIOSSettingsFlowStateInProgress {
			// If current is InProgress, move all other InProgress settings to Pending.
			// This can happen when we detect unexpected settings change on the BMC and need to restart.
			settings.Status.FlowState[idx].State = metalv1alpha1.BIOSSettingsFlowStatePending
		}
	}

	if currentIdx == -1 {
		flowStatus.State = state
		settings.Status.FlowState = append(settings.Status.FlowState, *flowStatus)
	}

	if err := r.Status().Patch(ctx, settings, client.MergeFrom(biosSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSSettings FlowState status: %w", err)
	}

	return nil
}

func (r *BIOSSettingsReconciler) updateStatus(ctx context.Context, settings *metalv1alpha1.BIOSSettings, state metalv1alpha1.BIOSSettingsState, condition *metav1.Condition) error {
	if settings.Status.State == state && condition == nil {
		return nil
	}

	biosSettingsBase := settings.DeepCopy()
	settings.Status.State = state

	if state == metalv1alpha1.BIOSSettingsStateApplied {
		now := metav1.Now()
		settings.Status.LastAppliedTime = &now
	} else if !settings.Status.LastAppliedTime.IsZero() {
		settings.Status.LastAppliedTime = nil
	}

	if condition != nil {
		if err := r.Conditions.UpdateSlice(
			&settings.Status.Conditions,
			condition.Type,
			conditionutils.UpdateStatus(condition.Status),
			conditionutils.UpdateReason(condition.Reason),
			conditionutils.UpdateMessage(condition.Message),
		); err != nil {
			return fmt.Errorf("failed to patch BIOSSettings condition: %w", err)
		}
	} else if state == metalv1alpha1.BIOSSettingsStatePending {
		// Reset when restarting the setting update
		settings.Status.Conditions = []metav1.Condition{}
		settings.Status.FlowState = []metalv1alpha1.BIOSSettingsFlowStatus{}
	}

	if err := r.Status().Patch(ctx, settings, client.MergeFrom(biosSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSSettings status: %w", err)
	}
	return nil
}

func (r *BIOSSettingsReconciler) getFlowItemFromSettingsStatus(settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem) *metalv1alpha1.BIOSSettingsFlowStatus {
	if len(settings.Status.FlowState) == 0 {
		return nil
	}

	for i := range settings.Status.FlowState {
		if settings.Status.FlowState[i].Priority == flowItem.Priority &&
			flowItem.Name == settings.Status.FlowState[i].Name {
			return &settings.Status.FlowState[i]
		}
	}
	return nil
}

func (r *BIOSSettingsReconciler) enqueueBiosSettingsByServerRefs(ctx context.Context, obj client.Object) []ctrl.Request {
	server := obj.(*metalv1alpha1.Server)
	log := ctrl.LoggerFrom(ctx).WithValues("Server", server.Name)

	if server.Status.State == metalv1alpha1.ServerStateDiscovery ||
		server.Status.State == metalv1alpha1.ServerStateError ||
		server.Status.State == metalv1alpha1.ServerStateInitial {
		return nil
	}

	settingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := r.List(ctx, settingsList, client.MatchingFields{serverRefField: server.Name}); err != nil {
		log.Error(err, "Failed to list BIOSSettings by server ref")
		return nil
	}

	reqs := make([]ctrl.Request, 0, len(settingsList.Items))
	for _, settings := range settingsList.Items {
		if settings.Spec.ServerMaintenanceRef == nil ||
			(server.Spec.ServerMaintenanceRef != nil &&
				(server.Spec.ServerMaintenanceRef.Name != settings.Spec.ServerMaintenanceRef.Name ||
					server.Spec.ServerMaintenanceRef.Namespace != settings.Spec.ServerMaintenanceRef.Namespace)) ||
			settings.Status.State == metalv1alpha1.BIOSSettingsStateApplied ||
			settings.Status.State == metalv1alpha1.BIOSSettingsStateFailed {
			continue
		}
		reqs = append(reqs, ctrl.Request{NamespacedName: types.NamespacedName{Name: settings.Name}})
	}
	return reqs
}

func (r *BIOSSettingsReconciler) enqueueBiosSettingsByBMC(ctx context.Context, obj client.Object) []ctrl.Request {
	bmcObj := obj.(*metalv1alpha1.BMC)
	log := ctrl.LoggerFrom(ctx).WithValues("BMC", bmcObj.Name)

	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList, client.MatchingFields{bmcRefField: bmcObj.Name}); err != nil {
		log.Error(err, "Failed to list Servers by BMC ref")
		return nil
	}

	var reqs []ctrl.Request
	for _, server := range serverList.Items {
		if server.Spec.BIOSSettingsRef == nil {
			continue
		}

		settings := &metalv1alpha1.BIOSSettings{}
		if err := r.Get(ctx, types.NamespacedName{Name: server.Spec.BIOSSettingsRef.Name}, settings); err != nil {
			log.Error(err, "Failed to get BIOSSettings, skipping", "name", server.Spec.BIOSSettingsRef.Name)
			continue
		}

		// Only enqueue if BMC reset was issued but not yet completed
		if settings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
			resetCond, err := GetCondition(r.Conditions, settings.Status.Conditions, BMCConditionReset)
			if err == nil && resetCond.Status != metav1.ConditionTrue && resetCond.Reason == BMCReasonReset {
				reqs = append(reqs, ctrl.Request{NamespacedName: types.NamespacedName{Name: settings.Name}})
			}
		}
	}
	return reqs
}

func (r *BIOSSettingsReconciler) enqueueBiosSettingsByBiosVersionResource(ctx context.Context, obj client.Object) []ctrl.Request {
	version := obj.(*metalv1alpha1.BIOSVersion)
	log := ctrl.LoggerFrom(ctx).WithValues("BIOSVersion", version.Name)

	if version.Status.State != metalv1alpha1.BIOSVersionStateCompleted || version.Spec.ServerRef == nil {
		return nil
	}

	settingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := r.List(ctx, settingsList, client.MatchingFields{serverRefField: version.Spec.ServerRef.Name}); err != nil {
		log.Error(err, "Failed to list BIOSSettings by server ref")
		return nil
	}

	reqs := make([]ctrl.Request, 0, 1)
	for _, settings := range settingsList.Items {
		if settings.Status.State == metalv1alpha1.BIOSSettingsStateApplied ||
			settings.Status.State == metalv1alpha1.BIOSSettingsStateFailed {
			continue
		}
		reqs = append(reqs, ctrl.Request{NamespacedName: types.NamespacedName{Name: settings.Name}})
	}
	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *BIOSSettingsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSSettings{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosSettingsByServerRefs)).
		Watches(&metalv1alpha1.BIOSVersion{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosSettingsByBiosVersionResource)).
		Watches(&metalv1alpha1.BMC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosSettingsByBMC)).
		Complete(r)
}
