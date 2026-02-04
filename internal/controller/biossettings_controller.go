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
	"strconv"
	"time"

	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/stmcginnis/gofish/redfish"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

const (
	BIOSSettingsFinalizer = "metal.ironcore.dev/biossettings"
)

// Server Maintenance conditions
const (
	BIOSServerMaintenanceConditionCreated = "ServerMaintenanceCreated"
	BIOSServerMaintenanceReasonCreated    = "ServerMaintenanceHasBeenCreated"
	BIOSServerMaintenanceConditionDeleted = "ServerMaintenanceDeleted"
	BIOSServerMaintenanceReasonDeleted    = "ServerMaintenanceHasBeenDeleted"
)

// BIOS Version conditions
const (
	BIOSVersionUpdateConditionPending = "BIOSVersionUpdatePending"
	BIOSVersionUpgradeReasonPending   = "BIOSVersionNeedsToBeUpgraded"
)

// Pending settings and validation conditions
const (
	BIOSPendingSettingConditionCheck     = "BIOSSettingsCheckPendingSettings"
	BIOSPendingSettingsReasonFound       = "BIOSPendingSettingsFound"
	BIOSSettingsConditionDuplicateKey    = "BIOSSettingsDuplicateKeys"
	BIOSSettingsReasonFoundDuplicateKeys = "BIOSSettingsDuplicateKeysFound"
)

// Update timing conditions
const (
	BIOSSettingConditionUpdateStartTime = "BIOSSettingUpdateStartTime"
	BIOSSettingsReasonUpdateStartTime   = "BIOSSettingsUpdateHasStarted"
	BIOSSettingConditionUpdateTimedOut  = "BIOSSettingsTimedOut"
	BIOSSettingsReasonUpdateTimedOut    = "BIOSSettingsTimedOutDuringUpdate"
)

// Server power and BMC conditions
const (
	BIOSSettingsConditionServerPowerOn = "ServerPowerOnCondition"
	BIOSSettingsReasonServerPoweredOn  = "ServerPoweredHasBeenPoweredOn"
	BMCConditionReset                  = "BMCResetIssued"
	BMCReasonReset                     = "BMCResetIssued"
)

// Settings update conditions
const (
	BIOSSettingsConditionIssuedUpdate           = "SettingsUpdateIssued"
	BIOSSettingReasonIssuedUpdate               = "BIOSSettingUpdateIssued"
	BIOSSettingsConditionUnknownPendingSettings = "UnknownPendingSettingState"
	BIOSSettingsReasonUnexpectedPendingSettings = "UnexpectedPendingSettingsPostUpdateHasBeenIssued"
	BIOSSettingsConditionWrongSettings          = "SettingsProvidedNotValid"
	BIOSSettingsReasonWrongSettings             = "SettingsProvidedAreNotValid"
)

// Reboot conditions
const (
	BIOSSettingsConditionRebootPostUpdate  = "ServerRebootPostUpdateHasBeenIssued"
	BIOSSettingsReasonSkipReboot           = "SkipServerRebootPostUpdateHasBeenIssued"
	BIOSSettingsReasonRebootNeeded         = "RebootPostSettingUpdate"
	BIOSSettingsConditionRebootPowerOff    = "RebootPowerOff"
	BIOSSettingsReasonRebootServerPowerOff = "PowerOffCompletedDuringReboot"
	BIOSSettingsConditionRebootPowerOn     = "RebootPowerOn"
	BIOSSettingsReasonRebootServerPowerOn  = "PowerOnCompletedDuringReboot"
)

// Verification conditions
const (
	BIOSSettingsConditionVerifySettings     = "VerifySettingsPostUpdate"
	BIOSSettingsReasonVerificationCompleted = "VerificationCompleted"
)

// BIOSSettingsReconciler reconciles a BIOSSettings object
type BIOSSettingsReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.Options
	ResyncInterval   time.Duration
	TimeoutExpiry    time.Duration
	Conditions       *conditionutils.Accessor
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
	log := ctrl.LoggerFrom(ctx)
	biosSettings := &metalv1alpha1.BIOSSettings{}
	if err := r.Get(ctx, req.NamespacedName, biosSettings); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, log, biosSettings)
}

func (r *BIOSSettingsReconciler) reconcileExists(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings) (ctrl.Result, error) {
	if r.shouldDelete(log, settings) {
		return r.delete(ctx, log, settings)
	}
	return r.reconcile(ctx, log, settings)
}

func (r *BIOSSettingsReconciler) shouldDelete(log logr.Logger, settings *metalv1alpha1.BIOSSettings) bool {
	if settings.DeletionTimestamp.IsZero() {
		return false
	}

	if controllerutil.ContainsFinalizer(settings, BIOSSettingsFinalizer) &&
		settings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
		log.V(1).Info("Postponing delete as BIOSSettings update is in progress")
		return false
	}
	return true
}

func (r *BIOSSettingsReconciler) delete(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings) (ctrl.Result, error) {
	log.V(1).Info("Deleting BIOSSettings")
	if err := r.cleanupReferences(ctx, log, settings); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to cleanup references: %w", err)
	}
	log.V(1).Info("Ensured references were removed")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, settings, BIOSSettingsFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured that the finalizer is removed")

	log.V(1).Info("BIOSSettings is deleted")
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) removeServerMaintenance(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings) error {
	if settings.Spec.ServerMaintenanceRef == nil {
		return nil
	}

	maintenance, err := GetServerMaintenanceForObjectReference(ctx, r.Client, settings.Spec.ServerMaintenanceRef)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get ServerMaintenance for BIOSSettings: %w", err)
	}

	var condition *metav1.Condition

	// Delete the maintenance if it exists and is controlled by this settings object
	if err == nil && maintenance.DeletionTimestamp.IsZero() && metav1.IsControlledBy(maintenance, settings) {
		log.V(1).Info("Deleting ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(maintenance), "State", maintenance.Status.State)
		condition, err = SetCondition(r.Conditions, settings.Status.Conditions,
			BIOSServerMaintenanceConditionDeleted, metav1.ConditionTrue, BIOSServerMaintenanceReasonDeleted,
			fmt.Sprintf("Deleting %s", maintenance.Name))
		if err != nil {
			return err
		}
		if err := r.Delete(ctx, maintenance); err != nil {
			return err
		}
	} else if err == nil && !metav1.IsControlledBy(maintenance, settings) {
		log.V(1).Info("ServerMaintenance is owned by someone else", "ServerMaintenance", client.ObjectKeyFromObject(maintenance), "State", maintenance.Status.State)
	}

	// Remove the reference from settings
	if apierrors.IsNotFound(err) || err == nil {
		if err := r.patchMaintenanceRef(ctx, settings, nil, condition); err != nil {
			return fmt.Errorf("failed to remove the ServerMaintenance reference in BIOSSettings status: %w", err)
		}
	}
	return nil
}

func (r *BIOSSettingsReconciler) cleanupReferences(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings) (err error) {
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

func (r *BIOSSettingsReconciler) reconcile(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(settings) {
		log.V(1).Info("Skipping BIOSSettings reconciliation")
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
			log.V(1).Info("Server contains a reference to a different BIOSSettings object", "BIOSSettings", server.Spec.BIOSSettingsRef.Name)
			return ctrl.Result{}, err
		}
		// Check if the current BIOSSettings version is newer and update reference if it is newer
		// todo : handle version checks correctly
		if referredBIOSSetting.Spec.Version < settings.Spec.Version {
			log.V(1).Info("Updating BIOSSettings reference to the latest BIOS version")
			if err := r.patchBIOSSettingsRefForServer(ctx, server, settings); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, settings, BIOSSettingsFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		if errors.As(err, &bmcutils.BMCUnAvailableError{}) {
			log.V(1).Info("BMC is not available", "BMC", server.Spec.BMCRef.Name, "Server", server.Name, "Message", err.Error())
			return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client for server: %w", err)
	}
	defer bmcClient.Logout()

	return r.ensureBIOSSettingsStateTransition(ctx, log, bmcClient, settings, server)
}

func (r *BIOSSettingsReconciler) ensureBIOSSettingsStateTransition(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	switch settings.Status.State {
	case "", metalv1alpha1.BIOSSettingsStatePending:
		return r.handleSettingPendingState(ctx, log, bmcClient, settings, server)
	case metalv1alpha1.BIOSSettingsStateInProgress:
		return r.handleSettingInProgressState(ctx, log, bmcClient, settings, server)
	case metalv1alpha1.BIOSSettingsStateApplied:
		return r.handleAppliedState(ctx, log, bmcClient, settings, server)
	case metalv1alpha1.BIOSSettingsStateFailed:
		return r.handleFailedState(ctx, log, settings, server)
	default:
		return ctrl.Result{}, fmt.Errorf("invalid BIOSSettings state: %s", settings.Status.State)
	}
}

// validateSettingsFlow checks for duplicate names and settings keys in the settings flow.
func (r *BIOSSettingsReconciler) validateSettingsFlow(settings *metalv1alpha1.BIOSSettings) (duplicateNames, duplicateSettings []string) {
	allNames := make(map[string]struct{})
	allSettingsNames := make(map[string]struct{})

	for _, flowItem := range settings.Spec.SettingsFlow {
		if _, ok := allNames[flowItem.Name]; ok {
			duplicateNames = append(duplicateNames, flowItem.Name)
		}
		allNames[flowItem.Name] = struct{}{}

		for key := range flowItem.Settings {
			if _, ok := allSettingsNames[key]; ok {
				duplicateSettings = append(duplicateSettings, key)
			}
			allSettingsNames[key] = struct{}{}
		}
	}
	return duplicateNames, duplicateSettings
}

// checkPendingSettings checks if there are pending BIOS settings and fails if found.
func (r *BIOSSettingsReconciler) checkPendingSettings(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (hasPending bool, err error) {
	pendingSettings, err := r.getPendingBIOSSettings(ctx, bmcClient, server)
	if err != nil {
		return false, fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}

	if len(pendingSettings) == 0 {
		return false, nil
	}

	log.V(1).Info("Pending BIOS setting tasks found", "TaskCount", len(pendingSettings))
	condition, err := SetCondition(r.Conditions, settings.Status.Conditions,
		BIOSPendingSettingConditionCheck, metav1.ConditionTrue, BIOSPendingSettingsReasonFound,
		fmt.Sprintf("Found pending BIOS settings (%d)", len(pendingSettings)))
	if err != nil {
		return true, err
	}
	return true, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, condition)
}

func (r *BIOSSettingsReconciler) handleSettingPendingState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if len(settings.Spec.SettingsFlow) == 0 {
		log.V(1).Info("Skipping BIOSSettings because no settings flow found")
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateApplied, nil)
	}

	// Check for pending BIOS settings
	if hasPending, err := r.checkPendingSettings(ctx, log, bmcClient, settings, server); hasPending || err != nil {
		return ctrl.Result{}, err
	}

	// Validate settings flow for duplicates
	duplicateNames, duplicateSettings := r.validateSettingsFlow(settings)
	if len(duplicateNames) > 0 || len(duplicateSettings) > 0 {
		log.V(1).Info("Found duplicate keys", "DuplicatesCount", len(duplicateNames), "DuplicatesSettingsCount", len(duplicateSettings))
		condition, err := SetCondition(r.Conditions, settings.Status.Conditions,
			BIOSSettingsConditionDuplicateKey, metav1.ConditionTrue, BIOSSettingsReasonFoundDuplicateKeys,
			fmt.Sprintf("Found duplicate keys (%d) and settings (%d)", len(duplicateNames), len(duplicateSettings)))
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, condition)
	}

	// Check if all settings have been applied
	biosVersion, settingsDiff, err := r.getBIOSVersionAndSettingsDiff(ctx, log, bmcClient, settings, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOSSettings: %w", err)
	}

	// If settings match and no conditions present, mark as applied (shortcut path)
	if len(settingsDiff) == 0 && len(settings.Status.Conditions) == 0 {
		condition, err := SetCondition(r.Conditions, settings.Status.Conditions,
			BIOSSettingsConditionVerifySettings, metav1.ConditionTrue, BIOSSettingsReasonVerificationCompleted,
			"Required BIOS settings has been verified on the server")
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateApplied, condition)
	}

	// Check if BIOS version upgrade is needed
	if biosVersion != settings.Spec.Version {
		return r.handleBIOSVersionMismatch(ctx, log, settings, biosVersion)
	}

	return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateInProgress, nil)
}

// handleBIOSVersionMismatch handles the case where the current BIOS version doesn't match the required version.
func (r *BIOSSettingsReconciler) handleBIOSVersionMismatch(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, currentVersion string) (ctrl.Result, error) {
	versionCheckCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, BIOSVersionUpdateConditionPending)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get Condition for pending BIOSVersion update state: %w", err)
	}

	if versionCheckCondition.Status == metav1.ConditionTrue {
		log.V(1).Info("Pending BIOS version upgrade.", "current bios Version", currentVersion, "required version", settings.Spec.Version)
		return ctrl.Result{}, nil
	}

	condition, err := SetCondition(r.Conditions, settings.Status.Conditions,
		BIOSVersionUpdateConditionPending, metav1.ConditionTrue, BIOSVersionUpgradeReasonPending,
		fmt.Sprintf("Waiting to update biosVersion: %s, current biosVersion: %s", settings.Spec.Version, currentVersion))
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStatePending, condition)
}

func (r *BIOSSettingsReconciler) handleSettingInProgressState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if req, err := r.requestMaintenanceForServer(ctx, log, settings, server); err != nil || req {
		return ctrl.Result{}, err
	}

	if !r.isServerInMaintenance(log, settings, server) {
		log.V(1).Info("Server is not yet in Maintenance status, skipping")
		return ctrl.Result{}, nil
	}

	if ok, err := r.handleBMCReset(ctx, log, bmcClient, settings, server); !ok || err != nil {
		return ctrl.Result{}, err
	}

	settingsFlow := append([]metalv1alpha1.SettingsFlowItem{}, settings.Spec.SettingsFlow...)
	sort.Slice(settingsFlow, func(i, j int) bool {
		return settingsFlow[i].Priority <= settingsFlow[j].Priority
	})

	// Process each setting in priority order
	for _, settingsFlowItem := range settingsFlow {
		result, done, err := r.processSettingsFlowItem(ctx, log, bmcClient, settings, &settingsFlowItem, server)
		if err != nil || done {
			return result, err
		}
	}

	return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateApplied, nil)
}

// processSettingsFlowItem processes a single settings flow item.
// Returns (result, done, error) where done=true means exit the loop, done=false means continue to next item.
func (r *BIOSSettingsReconciler) processSettingsFlowItem(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, server *metalv1alpha1.Server) (ctrl.Result, bool, error) {
	flowStatus := r.getFlowItemFromSettingsStatus(settings, flowItem)

	// Create new flow status if not found
	if flowStatus == nil {
		flowStatus = &metalv1alpha1.BIOSSettingsFlowStatus{
			Priority: flowItem.Priority,
			Name:     flowItem.Name,
		}
		err := r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateInProgress, flowStatus, nil)
		return ctrl.Result{}, true, err
	}

	// Check if settings need to be reapplied (for non-InProgress states)
	if flowStatus.State != metalv1alpha1.BIOSSettingsFlowStateInProgress {
		shouldContinue, done, err := r.checkAndReapplySettings(ctx, log, bmcClient, settings, flowItem, flowStatus, server)
		if err != nil || done {
			return ctrl.Result{}, true, err
		}
		if shouldContinue {
			// This flow item is already applied, move to next
			return ctrl.Result{}, false, nil
		}
	}

	// Apply and verify settings
	ok, err := r.applySettingUpdate(ctx, log, bmcClient, settings, flowItem, flowStatus, server)
	if err != nil {
		return ctrl.Result{}, true, err
	}
	if !ok {
		// applySettingUpdate returned false with no error - means status was updated and we need to requeue
		return ctrl.Result{}, true, nil
	}

	requeue, err := r.verifySettingsUpdateComplete(ctx, log, bmcClient, settings, flowItem, flowStatus, server)
	if requeue && err == nil {
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, true, nil
	}
	return ctrl.Result{}, true, err
}

// checkAndReapplySettings checks if settings need to be reapplied and handles the state accordingly.
// Returns (shouldContinue, done, error):
//   - shouldContinue=true means skip to next flow item (settings already applied and match)
//   - done=true means exit the processing loop (status was updated)
func (r *BIOSSettingsReconciler) checkAndReapplySettings(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (bool, bool, error) {
	settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
	if err != nil {
		return false, true, fmt.Errorf("failed to get current BIOS settings difference: %w", err)
	}
	if len(settingsDiff) > 0 {
		log.V(1).Info("Found BIOSSettings difference on Server", "Server", server.Name, "SettingsDifference", settingsDiff)
	}

	// No difference - check if already applied or mark as verified
	if len(settingsDiff) == 0 {
		if flowStatus.State == metalv1alpha1.BIOSSettingsFlowStateApplied {
			// Already applied, continue to next flow item
			return true, false, nil
		}
		// Mark as verified/applied
		condition, err := SetCondition(r.Conditions, flowStatus.Conditions,
			BIOSSettingsConditionVerifySettings, metav1.ConditionTrue, BIOSSettingsReasonVerificationCompleted,
			"Required BIOS settings has been RE verified on the server. Hence, moving out of Pending state")
		if err != nil {
			return false, true, err
		}
		err = r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateApplied, flowStatus, condition)
		return false, true, err
	}

	// Settings are different and were previously applied - need to reapply
	if flowStatus.State == metalv1alpha1.BIOSSettingsFlowStateApplied {
		err = r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateInProgress, flowStatus, nil)
		return false, true, err
	}

	// Continue with apply/verify for this flow item
	return false, false, nil
}

func (r *BIOSSettingsReconciler) handleBMCReset(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (bool, error) {
	resetBMC, err := GetCondition(r.Conditions, settings.Status.Conditions, BMCConditionReset)
	if err != nil {
		return false, fmt.Errorf("failed to get condition for reset of BMC of server %v", err)
	}

	if resetBMC.Status != metav1.ConditionTrue {
		// once the server is powered on, reset the BMC to make sure its in stable state
		// this avoids problems with some BMCs that hang up in subsequent operations
		if resetBMC.Reason != BMCReasonReset {
			if err := resetBMCOfServer(ctx, log, r.Client, server, bmcClient); err == nil {
				// mark reset to be issued, wait for next reconcile
				if err := r.Conditions.Update(
					resetBMC,
					conditionutils.UpdateStatus(corev1.ConditionFalse),
					conditionutils.UpdateReason(BMCReasonReset),
					conditionutils.UpdateMessage("Issued BMC reset to stabilize BMC of the server"),
				); err != nil {
					return false, fmt.Errorf("failed to update reset BMC condition: %w", err)
				}
				return false, r.updateStatus(ctx, settings, settings.Status.State, resetBMC)
			} else {
				return false, fmt.Errorf("failed to reset BMC: %w", err)
			}
		} else if server.Spec.BMCRef != nil {
			// we need to wait until the BMC resource annotation is removed
			bmcObj := &metalv1alpha1.BMC{}
			if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.BMCRef.Name}, bmcObj); err != nil {
				return false, err
			}
			annotations := bmcObj.GetAnnotations()
			if op, ok := annotations[metalv1alpha1.OperationAnnotation]; ok {
				if op == metalv1alpha1.GracefulRestartBMC {
					log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
					return false, nil
				}
			}
		}
		if err := r.Conditions.Update(
			resetBMC,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BMCReasonReset),
			conditionutils.UpdateMessage("BMC reset to stabilize BMC of the server is completed"),
		); err != nil {
			return false, fmt.Errorf("failed to update power on server condition: %w", err)
		}
		return false, r.updateStatus(ctx, settings, settings.Status.State, resetBMC)
	}
	return true, nil
}

func (r *BIOSSettingsReconciler) applySettingUpdate(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (bool, error) {
	if modified, err := r.setTimeoutForAppliedSettings(ctx, log, settings, flowStatus); modified || err != nil {
		return false, err
	}

	// Ensure server is powered on
	if ready, err := r.ensureServerPoweredOn(ctx, log, settings, flowStatus, server); !ready || err != nil {
		return false, err
	}

	// Determine if reboot is required
	if determined, err := r.determineRebootRequirement(ctx, log, bmcClient, settings, flowItem, flowStatus, server); !determined || err != nil {
		return determined, err
	}

	// Issue BIOS update if not already done
	issueBiosUpdate, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionIssuedUpdate)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for issuing BIOSSetting update to server: %w", err)
	}
	if issueBiosUpdate.Status != metav1.ConditionTrue {
		return false, r.applyBIOSSettings(ctx, log, bmcClient, settings, flowItem, flowStatus, server, issueBiosUpdate)
	}

	// Handle reboot if needed
	skipReboot, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPostUpdate)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for reboot needed condition: %w", err)
	}
	if skipReboot.Status == metav1.ConditionTrue {
		return true, nil
	}

	rebootPowerOnCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPowerOn)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for reboot PowerOn condition: %w", err)
	}
	if rebootPowerOnCondition.Status != metav1.ConditionTrue {
		return false, r.rebootServer(ctx, log, settings, flowStatus, server)
	}
	return true, nil
}

// ensureServerPoweredOn ensures the server is powered on for BIOS settings update.
func (r *BIOSSettingsReconciler) ensureServerPoweredOn(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (bool, error) {
	turnOnServer, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionServerPowerOn)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for Initial powerOn of server: %w", err)
	}

	if turnOnServer.Status == metav1.ConditionTrue {
		return true, nil
	}

	// Server already powered on - update condition and return
	if r.isServerInPowerState(server, metalv1alpha1.ServerOnPowerState) {
		if err := r.Conditions.Update(
			turnOnServer,
			conditionutils.UpdateStatus(metav1.ConditionTrue),
			conditionutils.UpdateReason(BIOSSettingsReasonServerPoweredOn),
			conditionutils.UpdateMessage("Server is powered On to start the biosUpdate process"),
		); err != nil {
			return false, fmt.Errorf("failed to update power on server condition: %w", err)
		}
		return false, r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, turnOnServer)
	}

	// Request maintenance if needed to power on the server
	if settings.Spec.ServerMaintenanceRef == nil {
		log.V(1).Info("Server powered off, request maintenance to turn the server On")
		if requeue, err := r.requestMaintenanceForServer(ctx, log, settings, server); err != nil || requeue {
			return false, err
		}
	}

	if err := r.patchPowerState(ctx, log, settings, metalv1alpha1.PowerOn); err != nil {
		return false, fmt.Errorf("failed to power on Server: %w", err)
	}
	log.V(1).Info("Reconciled BIOSSettings at TurnOnServer Condition")
	return false, nil
}

// determineRebootRequirement checks if the settings require a server reboot.
func (r *BIOSSettingsReconciler) determineRebootRequirement(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (bool, error) {
	// Check if we have already determined reboot requirement
	condFound, err := r.Conditions.FindSlice(flowStatus.Conditions, BIOSSettingsConditionRebootPostUpdate, &metav1.Condition{})
	if err != nil {
		return false, fmt.Errorf("failed to find Condition %v: %w", BIOSSettingsConditionRebootPostUpdate, err)
	}
	if condFound {
		return true, nil
	}

	log.V(1).Info("Verify if the current Settings needs reboot of server")
	settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
	if err != nil {
		return false, fmt.Errorf("failed to get BIOS settings difference: %w", err)
	}
	if len(settingsDiff) > 0 {
		log.V(1).Info("Found BIOSSettings difference on Server", "Server", server.Name, "SettingsDifference", settingsDiff)
	}

	resetReq, err := bmcClient.CheckBiosAttributes(settingsDiff)
	if err != nil {
		return r.handleInvalidSettings(ctx, log, settings, flowStatus, err)
	}

	// Set reboot condition based on whether reboot is required
	var status metav1.ConditionStatus
	var reason, message string
	if resetReq {
		status, reason, message = metav1.ConditionFalse, BIOSSettingsReasonRebootNeeded, "Settings provided needs server reboot"
	} else {
		status, reason, message = metav1.ConditionTrue, BIOSSettingsReasonSkipReboot, "Settings provided does not need server reboot"
		log.V(1).Info("BIOSSettings update does not need reboot")
	}

	skipReboot, err := SetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPostUpdate, status, reason, message)
	if err != nil {
		return false, err
	}

	log.V(1).Info("Reconciled biosSettings at check if reboot is needed")
	return false, r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, skipReboot)
}

// handleInvalidSettings handles the case where settings validation fails.
// Returns (determined=false, error) to signal the caller to stop processing.
func (r *BIOSSettingsReconciler) handleInvalidSettings(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, validationErr error) (bool, error) {
	log.V(1).Error(validationErr, "could not validate settings and determine if reboot needed")

	var invalidSettingsErr *bmc.InvalidBIOSSettingsError
	if !errors.As(validationErr, &invalidSettingsErr) {
		return false, validationErr
	}

	condition, err := SetCondition(r.Conditions, flowStatus.Conditions,
		BIOSSettingsConditionWrongSettings, metav1.ConditionTrue, BIOSSettingsReasonWrongSettings,
		fmt.Sprintf("Settings provided is invalid. error: %v", validationErr))
	if err != nil {
		return false, err
	}

	flowErr := r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateFailed, flowStatus, condition)
	statusErr := r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, nil)
	// Return false to signal that determination didn't complete normally (due to invalid settings)
	// This ensures the caller stops processing. Include any errors from the status updates.
	return false, errors.Join(flowErr, statusErr)
}

func (r *BIOSSettingsReconciler) setTimeoutForAppliedSettings(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus) (bool, error) {
	timeoutCheck, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingConditionUpdateStartTime)
	if err != nil {
		return false, fmt.Errorf("failed to get condition for TimeOut during setting update: %w", err)
	}

	// Start the timeout timer if not already started
	if timeoutCheck.Status != metav1.ConditionTrue {
		condition, err := SetCondition(r.Conditions, flowStatus.Conditions,
			BIOSSettingConditionUpdateStartTime, metav1.ConditionTrue, BIOSSettingsReasonUpdateStartTime,
			"Settings are being updated on Server. Timeout will occur beyond this point if settings are not applied")
		if err != nil {
			return false, err
		}
		return true, r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, condition)
	}

	// Check if timeout has been exceeded
	startTime := timeoutCheck.LastTransitionTime.Time
	if !time.Now().After(startTime.Add(r.TimeoutExpiry)) {
		return false, nil
	}

	log.V(1).Info("Timeout while updating the biosSettings")
	condition, err := SetCondition(r.Conditions, flowStatus.Conditions,
		BIOSSettingConditionUpdateTimedOut, metav1.ConditionTrue, BIOSSettingsReasonUpdateTimedOut,
		fmt.Sprintf("Timeout after: %v. startTime: %v. timedOut on: %v", r.TimeoutExpiry, startTime, time.Now().String()))
	if err != nil {
		return false, err
	}

	flowErr := r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateFailed, flowStatus, condition)
	statusErr := r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, nil)
	return true, errors.Join(flowErr, statusErr)
}

func (r *BIOSSettingsReconciler) verifySettingsUpdateComplete(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, biosSettings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (bool, error) {
	verifySettingUpdate, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionVerifySettings)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for Verification condition: %w", err)
	}

	if verifySettingUpdate.Status == metav1.ConditionTrue {
		log.V(1).Info("BIOS settings have been applied and verified on the server", "SettingsFlow", flowItem.Name)
		return false, nil
	}

	// Verify the settings have been applied
	settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
	if err != nil {
		return false, fmt.Errorf("failed to get BIOS settings diff: %w", err)
	}

	// Settings still different - wait for them to apply
	if len(settingsDiff) > 0 {
		log.V(1).Info("Waiting on the BIOS setting to take place")
		return true, nil
	}

	// Settings applied - mark as verified
	condition, err := SetCondition(r.Conditions, flowStatus.Conditions,
		BIOSSettingsConditionVerifySettings, metav1.ConditionTrue, BIOSSettingsReasonVerificationCompleted,
		"Required BIOS settings has been applied and verified on the server")
	if err != nil {
		return false, err
	}

	log.V(1).Info("Verified BIOS setting sequence", "Name", flowStatus.Name)
	return false, r.updateFlowStatus(ctx, biosSettings, metalv1alpha1.BIOSSettingsFlowStateApplied, flowStatus, condition)
}

func (r *BIOSSettingsReconciler) rebootServer(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) error {
	// Phase 1: Power off the server
	powerOffDone, err := r.handleRebootPowerOff(ctx, log, settings, flowStatus, server)
	if err != nil || !powerOffDone {
		return err
	}

	// Phase 2: Power on the server
	return r.handleRebootPowerOn(ctx, log, settings, flowStatus, server)
}

// handleRebootPowerOff handles the power off phase of a server reboot.
func (r *BIOSSettingsReconciler) handleRebootPowerOff(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (bool, error) {
	rebootPowerOffCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPowerOff)
	if err != nil {
		return false, fmt.Errorf("failed to get PowerOff condition: %w", err)
	}

	if rebootPowerOffCondition.Status == metav1.ConditionTrue {
		return true, nil
	}

	// Request power off if server is on
	if r.isServerInPowerState(server, metalv1alpha1.ServerOnPowerState) {
		if err := r.patchPowerState(ctx, log, settings, metalv1alpha1.PowerOff); err != nil {
			return false, fmt.Errorf("failed to power off server: %w", err)
		}
	}

	// Server is now off - update condition
	if r.isServerInPowerState(server, metalv1alpha1.ServerOffPowerState) {
		if err := r.Conditions.Update(
			rebootPowerOffCondition,
			conditionutils.UpdateStatus(metav1.ConditionTrue),
			conditionutils.UpdateReason(BIOSSettingsReasonRebootServerPowerOff),
			conditionutils.UpdateMessage("Server has entered power off state"),
		); err != nil {
			return false, fmt.Errorf("failed to update powerOff condition: %w", err)
		}
		return false, r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, rebootPowerOffCondition)
	}

	log.V(1).Info("Reconciled BIOSSettings. Waiting for powering off the Server", "Server", server.Name)
	return false, nil
}

// handleRebootPowerOn handles the power on phase of a server reboot.
func (r *BIOSSettingsReconciler) handleRebootPowerOn(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) error {
	rebootPowerOnCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPowerOn)
	if err != nil {
		return fmt.Errorf("failed to get PowerOn condition: %w", err)
	}

	if rebootPowerOnCondition.Status == metav1.ConditionTrue {
		return nil
	}

	// Request power on if server is off
	if r.isServerInPowerState(server, metalv1alpha1.ServerOffPowerState) {
		if err := r.patchPowerState(ctx, log, settings, metalv1alpha1.PowerOn); err != nil {
			return fmt.Errorf("failed to power on server: %w", err)
		}
	}

	// Server is now on - update condition
	if r.isServerInPowerState(server, metalv1alpha1.ServerOnPowerState) {
		if err := r.Conditions.Update(
			rebootPowerOnCondition,
			conditionutils.UpdateStatus(metav1.ConditionTrue),
			conditionutils.UpdateReason(BIOSSettingsReasonRebootServerPowerOn),
			conditionutils.UpdateMessage("Server has entered power on state"),
		); err != nil {
			return fmt.Errorf("failed to update reboot server powerOn condition: %w", err)
		}
		return r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, rebootPowerOnCondition)
	}

	log.V(1).Info("Reconciled BIOSSettings. Waiting for powering on the Server", "Server", server.Name)
	return nil
}

func (r *BIOSSettingsReconciler) applyBIOSSettings(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server, issueBiosUpdate *metav1.Condition) error {
	settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
	if err != nil {
		return fmt.Errorf("failed to get BIOS settings difference: %w", err)
	}

	// No settings difference - mark as issued and return
	if len(settingsDiff) == 0 {
		log.V(1).Info("No BIOS settings difference found to apply on server", "currentSettings Name", flowItem.Name)
		if err := r.Conditions.Update(
			issueBiosUpdate,
			conditionutils.UpdateStatus(metav1.ConditionTrue),
			conditionutils.UpdateReason(BIOSSettingReasonIssuedUpdate),
			conditionutils.UpdateMessage("BIOS Settings issue has been Skipped on the server as no difference found"),
		); err != nil {
			return fmt.Errorf("failed to update issued settings update condition: %w", err)
		}
		log.V(1).Info("Reconciled BIOSSettings at issue Settings to server state", "SettingsFlow", flowItem.Name)
		return r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, issueBiosUpdate)
	}

	// Issue the settings update
	if err := r.issueSettingsUpdate(ctx, log, bmcClient, settings, flowItem, flowStatus, server, settingsDiff); err != nil {
		return err
	}

	// Mark update as issued
	if err := r.Conditions.Update(
		issueBiosUpdate,
		conditionutils.UpdateStatus(metav1.ConditionTrue),
		conditionutils.UpdateReason(BIOSSettingReasonIssuedUpdate),
		conditionutils.UpdateMessage("BIOS settings update has been triggered on the server"),
	); err != nil {
		return fmt.Errorf("failed to update issued BIOSSettings update condition: %w", err)
	}
	return r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, issueBiosUpdate)
}

// issueSettingsUpdate applies the BIOS settings to the BMC and validates they were accepted.
func (r *BIOSSettingsReconciler) issueSettingsUpdate(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server, settingsDiff redfish.SettingsAttributes) error {
	// Check for existing pending settings before issuing new ones
	pendingSettings, err := r.getPendingBIOSSettings(ctx, bmcClient, server)
	if err != nil {
		return fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}
	if len(pendingSettings) > 0 {
		return fmt.Errorf("pending settings found on BIOS, cannot issue new settings update. pending settings: %v", pendingSettings)
	}

	// Apply the settings
	log.V(1).Info("Applying settings", "settingsDiff", settingsDiff, "SettingsName", flowItem.Name)
	if err := bmcClient.SetBiosAttributesOnReset(ctx, server.Spec.SystemURI, settingsDiff); err != nil {
		return fmt.Errorf("failed to set BMC settings: %w", err)
	}

	// Verify settings were accepted
	pendingSettings, err = r.getPendingBIOSSettings(ctx, bmcClient, server)
	if err != nil {
		return fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}

	skipReboot, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPostUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Condition for reboot needed condition: %w", err)
	}

	// If reboot is required but no pending settings, BMC didn't accept the update
	if len(pendingSettings) == 0 && skipReboot.Status == metav1.ConditionFalse {
		log.V(1).Info("BIOSSettings update issued to BMC was not accepted. retrying....")
		return fmt.Errorf("bios setting issued to bmc not accepted")
	}

	// Verify all required settings are in pending settings
	return r.validatePendingSettings(ctx, log, settings, flowStatus, settingsDiff, pendingSettings)
}

// validatePendingSettings checks that all required settings are correctly pending.
func (r *BIOSSettingsReconciler) validatePendingSettings(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, settingsDiff, pendingSettings redfish.SettingsAttributes) error {
	pendingSettingsDiff := make(redfish.SettingsAttributes, len(settingsDiff))
	for name, value := range settingsDiff {
		if pendingValue, ok := pendingSettings[name]; ok && value != pendingValue {
			pendingSettingsDiff[name] = pendingValue
		}
	}

	if len(pendingSettingsDiff) == 0 {
		return nil
	}

	log.V(1).Info("Difference between the pending settings and that of required", "SettingsDiff", pendingSettingsDiff)
	condition, err := SetCondition(r.Conditions, flowStatus.Conditions,
		BIOSSettingsConditionUnknownPendingSettings, metav1.ConditionTrue, BIOSSettingsReasonUnexpectedPendingSettings,
		fmt.Sprintf("Found unexpected settings after issuing settings update for BIOS. unexpected settings %v", pendingSettingsDiff))
	if err != nil {
		return err
	}

	flowErr := r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateFailed, flowStatus, condition)
	statusErr := r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, nil)
	return errors.Join(flowErr, statusErr)
}

func (r *BIOSSettingsReconciler) ensureNoStrandedStatus(ctx context.Context, settings *metalv1alpha1.BIOSSettings) (bool, error) {
	// In case the settings Spec got changed during in progress and left behind Stale states clean it up.
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

func (r *BIOSSettingsReconciler) handleAppliedState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if err := r.removeServerMaintenance(ctx, log, settings); err != nil {
		return ctrl.Result{}, err
	}

	if requeue, err := r.ensureNoStrandedStatus(ctx, settings); requeue || err != nil {
		return ctrl.Result{}, err
	}

	_, settingsDiff, err := r.getBIOSVersionAndSettingsDiff(ctx, log, bmcClient, settings, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOS version and settings diff: %w", err)
	}
	if len(settingsDiff) > 0 {
		log.V(1).Info("Found BIOS setting difference after applied state", "SettingsDiff", settingsDiff)
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStatePending, nil)
	}

	log.V(1).Info("Finished BIOSSettings update", "server", server)
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) handleFailedState(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if shouldRetryReconciliation(settings) {
		log.V(1).Info("Retrying reconciliation")
		biosSettingsBase := settings.DeepCopy()
		settings.Status.State = metalv1alpha1.BIOSSettingsStatePending
		// todo: add FlowState reset after the #403 is merged
		settings.Status.Conditions = nil
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
		// Check if the failure is due to pending settings
		for _, condition := range settings.Status.Conditions {
			if condition.Type == BIOSPendingSettingConditionCheck && condition.Status == metav1.ConditionTrue {
				if _, err := r.requestMaintenanceForServer(ctx, log, settings, server); err != nil {
					return ctrl.Result{}, err
				}
				break
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *BIOSSettingsReconciler) getPendingBIOSSettings(ctx context.Context, bmcClient bmc.BMC, server *metalv1alpha1.Server) (redfish.SettingsAttributes, error) {
	if server == nil {
		return redfish.SettingsAttributes{}, fmt.Errorf("server is nil")
	}
	return bmcClient.GetBiosPendingAttributeValues(ctx, server.Spec.SystemURI)
}

func (r *BIOSSettingsReconciler) getSettingsDiff(ctx context.Context, bmcClient bmc.BMC, settings map[string]string, server *metalv1alpha1.Server) (redfish.SettingsAttributes, error) {
	keys := slices.Collect(maps.Keys(settings))

	// get the accepted type/values from the server BIOS for given keys
	actualSettings, err := bmcClient.GetBiosAttributeValues(ctx, server.Spec.SystemURI, keys)
	if err != nil {
		return redfish.SettingsAttributes{}, fmt.Errorf("failed to get BIOSSettings: %w", err)
	}

	// check if the given settings match the accepted setting's type/values from server BIOS
	diff := redfish.SettingsAttributes{}
	var errs []error
	for key, value := range settings {
		res, ok := actualSettings[key]
		if ok {
			switch data := res.(type) {
			case int:
				intValue, err := strconv.Atoi(value)
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to check type for name %s; value %s; error: %w", key, value, err))
					continue
				}
				if data != intValue {
					diff[key] = intValue
				}
			case string:
				if data != value {
					diff[key] = value
				}
			case float64:
				floatValue, err := strconv.ParseFloat(value, 64)
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to check type for name %s; value %s; error: %w", key, value, err))
				}
				if data != floatValue {
					diff[key] = floatValue
				}
			}
		} else {
			diff[key] = value
		}
	}

	return diff, errors.Join(errs...)
}

func (r *BIOSSettingsReconciler) getBIOSVersionAndSettingsDiff(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (string, redfish.SettingsAttributes, error) {
	completeSettings := make(map[string]string)
	for _, flowItem := range settings.Spec.SettingsFlow {
		for key, value := range flowItem.Settings {
			completeSettings[key] = value
		}
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
		return "", diff, fmt.Errorf("failed get BIOS version: %w", err)
	}

	return version, diff, nil
}

func (r *BIOSSettingsReconciler) isServerInPowerState(server *metalv1alpha1.Server, state metalv1alpha1.ServerPowerState) bool {
	return server.Status.PowerState == state
}

func (r *BIOSSettingsReconciler) isServerInMaintenance(log logr.Logger, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) bool {
	if settings.Spec.ServerMaintenanceRef == nil {
		return false
	}

	if server.Status.State == metalv1alpha1.ServerStateMaintenance {
		if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.UID != settings.Spec.ServerMaintenanceRef.UID {
			log.V(1).Info("Server is already in maintenance", "Server", server.Name)
			return false
		}
	} else {
		log.V(1).Info("Server not yet in maintenance", "Server", server.Name, "State", server.Status.State)
		return false
	}

	return true
}

func (r *BIOSSettingsReconciler) requestMaintenanceForServer(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (bool, error) {
	if settings.Spec.ServerMaintenanceRef != nil {
		return false, nil
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
		return false, fmt.Errorf("failed to create or patch serverMaintenance: %w", err)
	}
	log.V(1).Info("Created/Patched ServerMaintenance", "ServerMaintenance", serverMaintenance.Name, "Operation", opResult)

	condition, err := SetCondition(r.Conditions, settings.Status.Conditions,
		BIOSServerMaintenanceConditionCreated, metav1.ConditionTrue, BIOSServerMaintenanceReasonCreated,
		fmt.Sprintf("Created %v at %v", serverMaintenance.Name, time.Now()))
	if err != nil {
		return false, err
	}

	if err := r.patchMaintenanceRef(ctx, settings, serverMaintenance, condition); err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in biosSettings status: %w", err)
	}

	log.V(1).Info("Patched ServerMaintenance reference on BIOSSettings")
	return true, nil
}

func (r *BIOSSettingsReconciler) getBIOSSettingsByName(ctx context.Context, name string) (*metalv1alpha1.BIOSSettings, error) {
	biosSettings := &metalv1alpha1.BIOSSettings{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, biosSettings); err != nil {
		return nil, fmt.Errorf("failed to get referred BIOSSetting: %w", err)
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

func (r *BIOSSettingsReconciler) patchMaintenanceRef(ctx context.Context, settings *metalv1alpha1.BIOSSettings, maintenance *metalv1alpha1.ServerMaintenance, condition *metav1.Condition) error {
	biosSettingsBase := settings.DeepCopy()

	if maintenance == nil {
		settings.Spec.ServerMaintenanceRef = nil
	} else {
		settings.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{
			APIVersion: metalv1alpha1.GroupVersion.String(),
			Kind:       "ServerMaintenance",
			Namespace:  maintenance.Namespace,
			Name:       maintenance.Name,
			UID:        maintenance.UID,
		}
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
	}

	if err := r.Patch(ctx, settings, client.MergeFrom(biosSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch ServerMaintenance ref in BIOSSettings: %w", err)
	}

	if err := r.updateStatus(ctx, settings, settings.Status.State, condition); err != nil {
		return fmt.Errorf("failed to patch BIOSSettings conditions: %w", err)
	}

	return nil
}

func (r *BIOSSettingsReconciler) patchPowerState(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, powerState metalv1alpha1.Power) error {
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
		return fmt.Errorf("failed to patch power for serverMaintenance: %w", err)
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
					return fmt.Errorf("failed to patch BIOSettings condition: %w", err)
				}
			} else {
				settings.Status.FlowState[idx].Conditions = nil
			}
			currentIdx = idx
			continue
		} else if state == metalv1alpha1.BIOSSettingsFlowStateInProgress &&
			status.State == metalv1alpha1.BIOSSettingsFlowStateInProgress {
			// if current is InProgress, move all other settings state to Pending state.
			// This can happen when we suddenly detect settings change in actual BMC and have to start over the settings
			settings.Status.FlowState[idx].State = metalv1alpha1.BIOSSettingsFlowStatePending
		}
	}

	if currentIdx == -1 {
		// if the currentFlowStatus is missing, add it.
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
		settings.Status.LastAppliedTime = &metav1.Time{}
	}

	if condition != nil {
		if err := r.Conditions.UpdateSlice(
			&settings.Status.Conditions,
			condition.Type,
			conditionutils.UpdateStatus(condition.Status),
			conditionutils.UpdateReason(condition.Reason),
			conditionutils.UpdateMessage(condition.Message),
		); err != nil {
			return fmt.Errorf("failed to patch BIOSettings condition: %w", err)
		}
	} else if state == metalv1alpha1.BIOSSettingsStatePending {
		// reset, when we restart the setting update
		settings.Status.Conditions = nil
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
		log.Error(err, "failed to list BIOSSettings by server ref")
		return nil
	}

	reqs := make([]ctrl.Request, 0, len(settingsList.Items))
	for _, settings := range settingsList.Items {
		if settings.Spec.ServerMaintenanceRef == nil ||
			(server.Spec.ServerMaintenanceRef != nil &&
				server.Spec.ServerMaintenanceRef.UID != settings.Spec.ServerMaintenanceRef.UID) ||
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
		log.Error(err, "failed to list Servers by BMC ref")
		return nil
	}

	var reqs []ctrl.Request
	for _, server := range serverList.Items {
		if server.Spec.BIOSSettingsRef == nil {
			continue
		}

		settings := &metalv1alpha1.BIOSSettings{}
		if err := r.Get(ctx, types.NamespacedName{Name: server.Spec.BIOSSettingsRef.Name}, settings); err != nil {
			log.Error(err, "failed to get BIOSSettings, skipping", "name", server.Spec.BIOSSettingsRef.Name)
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
		log.Error(err, "failed to list BIOSSettings by server ref")
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
