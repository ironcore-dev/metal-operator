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

	BIOSServerMaintenanceConditionCreated       = "ServerMaintenanceCreated"
	BIOSServerMaintenanceReasonCreated          = "ServerMaintenanceHasBeenCreated"
	BIOSServerMaintenanceConditionDeleted       = "ServerMaintenanceDeleted"
	BIOSServerMaintenanceReasonDeleted          = "ServerMaintenanceHasBeenDeleted"
	BIOSVersionUpdateConditionPending           = "BIOSVersionUpdatePending"
	BIOSVersionUpgradeReasonPending             = "BIOSVersionNeedsTObeUpgraded"
	BIOSPendingSettingConditionCheck            = "BIOSSettingsCheckPendingSettings"
	BIOSPendingSettingsReasonFound              = "BIOSPendingSettingsFound"
	BIOSSettingsConditionDuplicateKey           = "BIOSSettingsDuplicateKeys"
	BIOSSettingsReasonFoundDuplicateKeys        = "BIOSSettingsDuplicateKeysFound"
	BIOSSettingConditionUpdateStartTime         = "BIOSSettingUpdateStartTime"
	BIOSSettingsReasonUpdateStartTime           = "BIOSSettingsUpdateHasStarted"
	BIOSSettingConditionUpdateTimedOut          = "BIOSSettingsTimedOut"
	BIOSSettingsReasonUpdateTimedOut            = "BIOSSettingsTimedOutDuringUpdate"
	BIOSSettingsConditionServerPowerOn          = "ServerPowerOnCondition"
	BIOSSettingsReasonServerPoweredOn           = "ServerPoweredHasBeenPoweredOn"
	BMCConditionReset                           = "BMCResetIssued"
	BMCReasonReset                              = "BMCResetIssued"
	BIOSSettingsConditionIssuedUpdate           = "SettingsUpdateIssued"
	BIOSSettingReasonIssuedUpdate               = "BIOSSettingUpdateIssued"
	BIOSSettingsConditionUnknownPendingSettings = "UnknownPendingSettingState"
	BIOSSettingsReasonUnexpectedPendingSettings = "UnexpectedPendingSettingsPostUpdateHasBeenIssued"
	BIOSSettingsConditionRebootPostUpdate       = "ServerRebootPostUpdateHasBeenIssued"
	BIOSSettingsReasonSkipReboot                = "SkipServerRebootPostUpdateHasBeenIssued"
	BIOSSettingsReasonRebootNeeded              = "RebootPostSettingUpdate"
	BIOSSettingsConditionRebootPowerOff         = "RebootPowerOff"
	BIOSSettingsReasonRebootServerPowerOff      = "PowerOffCompletedDuringReboot"
	BIOSSettingsConditionRebootPowerOn          = "RebootPowerOn"
	BIOSSettingsReasonRebootServerPowerOn       = "PowerOnCompletedDuringReboot"
	BIOSSettingsConditionVerifySettings         = "VerifySettingsPostUpdate"
	BIOSSettingsReasonVerificationCompleted     = "VerificationCompleted"
	BIOSSettingsConditionWrongSettings          = "SettingsProvidedNotValid"
	BIOSSettingsReasonWrongSettings             = "SettingsProvidedAreNotValid"
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
	if err == nil && maintenance.DeletionTimestamp.IsZero() {
		if metav1.IsControlledBy(maintenance, settings) {
			log.V(1).Info("Deleting ServerMaintenance", "ServerMaintenance", client.ObjectKeyFromObject(maintenance), "State", maintenance.Status.State)
			condition, err = GetCondition(r.Conditions, settings.Status.Conditions, BIOSServerMaintenanceConditionDeleted)
			if err != nil {
				return fmt.Errorf("failed to get the delete condition for ServerMaintenance: %w", err)
			}
			if err := r.Conditions.Update(
				condition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(BIOSServerMaintenanceReasonDeleted),
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

func (r *BIOSSettingsReconciler) handleSettingPendingState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if len(settings.Spec.SettingsFlow) == 0 {
		log.V(1).Info("Skipping BIOSSettings because no settings flow found")
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateApplied, nil)
	}

	pendingSettings, err := r.getPendingBIOSSettings(ctx, bmcClient, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}

	if len(pendingSettings) > 0 {
		log.V(1).Info("Pending BIOS setting tasks found", "TaskCount", len(pendingSettings))

		pendingSettingStateCheckCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, BIOSPendingSettingConditionCheck)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get Condition for pending BIOSSettings state %w", err)
		}

		if err := r.Conditions.Update(
			pendingSettingStateCheckCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BIOSPendingSettingsReasonFound),
			conditionutils.UpdateMessage(fmt.Sprintf("Found pending BIOS settings (%d)", len(pendingSettings))),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update pending BIOSSettings update condition: %w", err)
		}

		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, pendingSettingStateCheckCondition)
	}

	// Verify that no duplicate name and duplicate settings are found
	allNames := map[string]struct{}{}
	allSettingsNames := map[string]struct{}{}
	duplicateName := make([]string, 0, len(settings.Spec.SettingsFlow))
	var duplicateSettingsNames []string
	for _, flowItem := range settings.Spec.SettingsFlow {
		if _, ok := allNames[flowItem.Name]; ok {
			duplicateName = append(duplicateName, flowItem.Name)
		}
		allNames[flowItem.Name] = struct{}{}
		for key := range flowItem.Settings {
			if _, ok := allSettingsNames[key]; ok {
				duplicateSettingsNames = append(duplicateSettingsNames, key)
			}
			allSettingsNames[key] = struct{}{}
		}
	}

	if len(duplicateName) > 0 || len(duplicateSettingsNames) > 0 {
		log.V(1).Info("Found duplicate keys", "DuplicatesCount", len(duplicateName), "DuplicatesSettingsCound", len(duplicateSettingsNames))
		duplicateCheckCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, BIOSSettingsConditionDuplicateKey)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get Condition for pending BIOSSettings state %w", err)
		}
		if err := r.Conditions.Update(
			duplicateCheckCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BIOSSettingsReasonFoundDuplicateKeys),
			conditionutils.UpdateMessage(fmt.Sprintf("Found duplicate keys (%d) and settings (%d)", len(duplicateName), len(duplicateSettingsNames))),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update pending BIOSSettings update condition: %w", err)
		}
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, duplicateCheckCondition)
	}

	// Check if all settings have been applied
	biosVersion, settingsDiff, err := r.getBIOSVersionAndSettingsDiff(ctx, log, bmcClient, settings, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOSSettings: %w", err)
	}
	// if setting is not different, complete the BIOS tasks, does not matter if the bios version do not match
	// if conditions are present, skip this shortcut to be able to capture all conditions states (ex: verifySetting, reboot etc)
	if len(settingsDiff) == 0 && len(settings.Status.Conditions) == 0 {
		// move status to completed
		verifySettingUpdate, err := GetCondition(r.Conditions, settings.Status.Conditions, BIOSSettingsConditionVerifySettings)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get Condition for verified BIOSSettings condition %w", err)
		}
		// move  biosSettings state to completed
		if err := r.Conditions.Update(
			verifySettingUpdate,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BIOSSettingsReasonVerificationCompleted),
			conditionutils.UpdateMessage("Required BIOS settings has been verified on the server"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update verify biossetting condition: %w", err)
		}
		return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateApplied, verifySettingUpdate)
	}

	var state = metalv1alpha1.BIOSSettingsStateInProgress
	var condition *metav1.Condition
	if biosVersion != settings.Spec.Version {
		versionCheckCondition, err := GetCondition(r.Conditions, settings.Status.Conditions, BIOSVersionUpdateConditionPending)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get Condition for pending BIOSVersion update state: %w", err)
		}
		if versionCheckCondition.Status == metav1.ConditionTrue {
			log.V(1).Info("Pending BIOS version upgrade.", "current bios Version", biosVersion, "required version", settings.Spec.Version)
			return ctrl.Result{}, nil
		}
		if err := r.Conditions.Update(
			versionCheckCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BIOSVersionUpgradeReasonPending),
			conditionutils.UpdateMessage(fmt.Sprintf("Waiting to update biosVersion: %s, current biosVersion: %s", settings.Spec.Version, biosVersion)),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update Pending BIOSVersion update condition: %w", err)
		}
		state = metalv1alpha1.BIOSSettingsStatePending
		condition = versionCheckCondition
	}
	return ctrl.Result{}, r.updateStatus(ctx, settings, state, condition)
}

func (r *BIOSSettingsReconciler) handleSettingInProgressState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if req, err := r.requestMaintenanceForServer(ctx, log, settings, server); err != nil || req {
		return ctrl.Result{}, err
	}

	if ok := r.isServerInMaintenance(log, settings, server); !ok {
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

	// loop through all the sequence in priority order and verify/Apply the settings
	for _, settingsFlowItem := range settingsFlow {
		// check each setting in the order of priority apply and verify it
		currentSettingsFlowStatus := r.getFlowItemFromSettingsStatus(settings, &settingsFlowItem)

		// if the setting state is not found, create it
		if currentSettingsFlowStatus == nil {
			currentSettingsFlowStatus = &metalv1alpha1.BIOSSettingsFlowStatus{
				Priority: settingsFlowItem.Priority,
				Name:     settingsFlowItem.Name,
			}
			return ctrl.Result{}, r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateInProgress, currentSettingsFlowStatus, nil)
		}

		// if the state is InProgress, go ahead and apply/Verify the settings
		if currentSettingsFlowStatus.State != metalv1alpha1.BIOSSettingsFlowStateInProgress {
			// else, check if the settings is still as expected, and proceed.
			settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, settingsFlowItem.Settings, server)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed get current BIOS settings difference: %w", err)
			}
			if len(settingsDiff) > 0 {
				log.V(1).Info("Found BIOSSettings difference on Server", "Server", server.Name, "SettingsDifference", settingsDiff)
			}

			// Handle if no setting update is needed
			if len(settingsDiff) == 0 {
				// if the state reflects it move on
				if currentSettingsFlowStatus.State == metalv1alpha1.BIOSSettingsFlowStateApplied {
					continue
				}
				// mark completed, and move on
				verifySettingUpdate, err := GetCondition(r.Conditions, currentSettingsFlowStatus.Conditions, BIOSSettingsConditionVerifySettings)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to get Condition for verified BIOSSettings condition: %w", err)
				}
				// move  biosSettings state to completed
				if err := r.Conditions.Update(
					verifySettingUpdate,
					conditionutils.UpdateStatus(corev1.ConditionTrue),
					conditionutils.UpdateReason(BIOSSettingsReasonVerificationCompleted),
					conditionutils.UpdateMessage("Required BIOS settings has been RE verified on the server. Hence, moving out of Pending state"),
				); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to update verified BIOSSettings condition: %w", err)
				}
				return ctrl.Result{}, r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateApplied, currentSettingsFlowStatus, verifySettingUpdate)
			}

			// If the BIOS settings are different and the status was previously applied,
			// make sure to reapply settings, reset any other InProgress state for higher Priority Settings.
			if currentSettingsFlowStatus.State == metalv1alpha1.BIOSSettingsFlowStateApplied {
				// update the state to reflect the current settings we are about to apply
				// may be added condition to indicate the reapply
				return ctrl.Result{}, r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateInProgress, currentSettingsFlowStatus, nil)
			}
		}

		if ok, err := r.applySettingUpdate(ctx, log, bmcClient, settings, &settingsFlowItem, currentSettingsFlowStatus, server); ok && err == nil {
			if requeue, err := r.verifySettingsUpdateComplete(ctx, log, bmcClient, settings, &settingsFlowItem, currentSettingsFlowStatus, server); requeue && err == nil {
				return ctrl.Result{RequeueAfter: r.ResyncInterval}, err
			}
			return ctrl.Result{}, err
		} else {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateApplied, nil)
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
	turnOnServer, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionServerPowerOn)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for Initial powerOn of server %v", err)
	}

	if turnOnServer.Status != metav1.ConditionTrue {
		if r.isServerInPowerState(server, metalv1alpha1.ServerOnPowerState) {
			if err := r.Conditions.Update(
				turnOnServer,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(BIOSSettingsReasonServerPoweredOn),
				conditionutils.UpdateMessage("Server is powered On to start the biosUpdate process"),
			); err != nil {
				return false, fmt.Errorf("failed to update power on server condition: %w", err)
			}
			return false, r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, turnOnServer)
		}
		// we need to request maintenance to get the server to power-On to apply the BIOS settings
		if settings.Spec.ServerMaintenanceRef == nil {
			log.V(1).Info("Server powered off, request maintenance to turn the server On")
			if requeue, err := r.requestMaintenanceForServer(ctx, log, settings, server); err != nil || requeue {
				return false, err
			}
		}

		if err := r.patchPowerState(ctx, log, settings, metalv1alpha1.PowerOn); err != nil {
			return false, fmt.Errorf("failed to power on Server %w", err)
		}
		log.V(1).Info("Reconciled BIOSSettings at TurnOnServer Condition")
		return false, err
	}

	// check if we have already determined if we need reboot of not.
	// if the condition is present, we have checked the skip reboot condition.
	condFound, err := r.Conditions.FindSlice(flowStatus.Conditions, BIOSSettingsConditionRebootPostUpdate, &metav1.Condition{})
	if err != nil {
		return false, fmt.Errorf("failed to find Condition %v. error: %v", BIOSSettingsConditionRebootPostUpdate, err)
	}

	if !condFound {
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
			log.V(1).Error(err, "could not validate settings and determine if reboot needed")
			var invalidSettingsErr *bmc.InvalidBIOSSettingsError
			if errors.As(err, &invalidSettingsErr) {
				inValidSettings, errCond := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionWrongSettings)
				if errCond != nil {
					return false, fmt.Errorf("failed to get Condition for skip reboot post setting update %v", err)
				}
				if errCond := r.Conditions.Update(
					inValidSettings,
					conditionutils.UpdateStatus(corev1.ConditionTrue),
					conditionutils.UpdateReason(BIOSSettingsReasonWrongSettings),
					conditionutils.UpdateMessage(fmt.Sprintf("Settings provided is invalid. error: %v", err)),
				); errCond != nil {
					return false, fmt.Errorf("failed to update Invalid Settings condition: %w", errCond)
				}
				err = r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateFailed, flowStatus, inValidSettings)
				return true, errors.Join(err, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, nil))
			}
			return false, err
		}

		skipReboot, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPostUpdate)
		if err != nil {
			return false, fmt.Errorf("failed to get Condition for skip reboot post setting update %v", err)
		}

		// if we dont need reboot. skip reboot steps.
		if !resetReq {
			log.V(1).Info("BIOSSettings update does not need reboot")
			if err := r.Conditions.Update(
				skipReboot,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(BIOSSettingsReasonSkipReboot),
				conditionutils.UpdateMessage("Settings provided does not need server reboot"),
			); err != nil {
				return false, fmt.Errorf("failed to update skip reboot condition: %w", err)
			}
		} else {
			if err := r.Conditions.Update(
				skipReboot,
				conditionutils.UpdateStatus(corev1.ConditionFalse),
				conditionutils.UpdateReason(BIOSSettingsReasonRebootNeeded),
				conditionutils.UpdateMessage("Settings provided needs server reboot"),
			); err != nil {
				return false, fmt.Errorf("failed to update skip reboot condition: %w", err)
			}
		}
		err = r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, skipReboot)
		log.V(1).Info("Reconciled biosSettings at check if reboot is needed")
		return false, err
	}

	issueBiosUpdate, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionIssuedUpdate)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for issuing BIOSSetting update to server %v", err)
	}

	if issueBiosUpdate.Status != metav1.ConditionTrue {
		return false, r.applyBIOSSettings(ctx, log, bmcClient, settings, flowItem, flowStatus, server, issueBiosUpdate)
	}

	skipReboot, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPostUpdate)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for reboot needed condition %v", err)
	}

	if skipReboot.Status != metav1.ConditionTrue {
		rebootPowerOnCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPowerOn)
		if err != nil {
			return false, fmt.Errorf("failed to get Condition for reboot PowerOn condition %v", err)
		}
		// reboot is not yet completed
		if rebootPowerOnCondition.Status != metav1.ConditionTrue {
			return false, r.rebootServer(ctx, log, settings, flowStatus, server)
		}
	}
	return true, nil
}

func (r *BIOSSettingsReconciler) setTimeoutForAppliedSettings(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus) (bool, error) {
	timeoutCheck, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingConditionUpdateStartTime)
	if err != nil {
		return false, fmt.Errorf("failed to get condition for TimeOut during setting update %v", err)
	}
	if timeoutCheck.Status != metav1.ConditionTrue {
		if err := r.Conditions.Update(
			timeoutCheck,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BIOSSettingsReasonUpdateStartTime),
			conditionutils.UpdateMessage("Settings are being updated on Server. Timeout will occur beyond this point if settings are not applied"),
		); err != nil {
			return false, fmt.Errorf("failed to update starting setting update condition: %w", err)
		}
		return true, r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, timeoutCheck)
	} else {
		startTime := timeoutCheck.LastTransitionTime.Time
		if time.Now().After(startTime.Add(r.TimeoutExpiry)) {
			log.V(1).Info("Timeout while updating the biosSettings")
			timedOut, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingConditionUpdateTimedOut)
			if err != nil {
				return false, fmt.Errorf("failed to get Condition for Timeout of BIOSSettings update %w", err)
			}
			if err := r.Conditions.Update(
				timedOut,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(BIOSSettingsReasonUpdateTimedOut),
				conditionutils.UpdateMessage(fmt.Sprintf("Timeout after: %v. startTime: %v. timedOut on: %v", r.TimeoutExpiry, startTime, time.Now().String())),
			); err != nil {
				return false, fmt.Errorf("failed to update timeout during settings update condition: %w", err)
			}
			err = r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateFailed, flowStatus, timedOut)
			return true, errors.Join(err, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, nil))
		}
	}
	return false, nil
}

func (r *BIOSSettingsReconciler) verifySettingsUpdateComplete(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, biosSettings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) (bool, error) {
	verifySettingUpdate, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionVerifySettings)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for Verification condition %w", err)
	}

	if verifySettingUpdate.Status != metav1.ConditionTrue {
		// make sure the setting has actually applied.
		settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
		if err != nil {
			return false, fmt.Errorf("failed to get BIOS settings diff: %w", err)
		}

		// if setting is not different, complete the BIOS tasks
		if len(settingsDiff) == 0 {
			if err := r.Conditions.Update(
				verifySettingUpdate,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(BIOSSettingsReasonVerificationCompleted),
				conditionutils.UpdateMessage("Required BIOS settings has been applied and verified on the server"),
			); err != nil {
				return false, fmt.Errorf("failed to update verify BIOSSetting condition: %w", err)
			}
			log.V(1).Info("Verified BIOS setting sequence", "Name", flowStatus.Name)
			return false, r.updateFlowStatus(ctx, biosSettings, metalv1alpha1.BIOSSettingsFlowStateApplied, flowStatus, verifySettingUpdate)
		}

		log.V(1).Info("Waiting on the BIOS setting to take place")
		return true, nil
	}

	log.V(1).Info("BIOS settings have been applied and verified on the server", "SettingsFlow", flowItem.Name)
	return false, nil
}

func (r *BIOSSettingsReconciler) rebootServer(ctx context.Context, log logr.Logger, settings *metalv1alpha1.BIOSSettings, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server) error {
	rebootPowerOffCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPowerOff)
	if err != nil {
		return fmt.Errorf("failed to get PowerOff condition: %w", err)
	}

	if rebootPowerOffCondition.Status != metav1.ConditionTrue {
		// expected state it to be off and initial state is to be on.
		if r.isServerInPowerState(server, metalv1alpha1.ServerOnPowerState) {
			if err := r.patchPowerState(ctx, log, settings, metalv1alpha1.PowerOff); err != nil {
				return fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.isServerInPowerState(server, metalv1alpha1.ServerOffPowerState) {
			if err := r.Conditions.Update(
				rebootPowerOffCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(BIOSSettingsReasonRebootServerPowerOff),
				conditionutils.UpdateMessage("Server has entered power off state"),
			); err != nil {
				return fmt.Errorf("failed to update powerOff condition: %w", err)
			}
			return r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, rebootPowerOffCondition)
		}
		log.V(1).Info("Reconciled BIOSSettings. Waiting for powering off the Server", "Server", server.Name)
		return nil
	}

	rebootPowerOnCondition, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPowerOn)
	if err != nil {
		return fmt.Errorf("failed to get PowerOn condition %v", err)
	}

	if rebootPowerOnCondition.Status != metav1.ConditionTrue {
		// expected power state it to be on and initial state is to be off.
		if r.isServerInPowerState(server, metalv1alpha1.ServerOffPowerState) {
			if err := r.patchPowerState(ctx, log, settings, metalv1alpha1.PowerOn); err != nil {
				return fmt.Errorf("failed to reboot server: %w", err)
			}
		}
		if r.isServerInPowerState(server, metalv1alpha1.ServerOnPowerState) {
			if err := r.Conditions.Update(
				rebootPowerOnCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
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

	return nil
}

func (r *BIOSSettingsReconciler) applyBIOSSettings(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, settings *metalv1alpha1.BIOSSettings, flowItem *metalv1alpha1.SettingsFlowItem, flowStatus *metalv1alpha1.BIOSSettingsFlowStatus, server *metalv1alpha1.Server, issueBiosUpdate *metav1.Condition) error {
	settingsDiff, err := r.getSettingsDiff(ctx, bmcClient, flowItem.Settings, server)
	if err != nil {
		return fmt.Errorf("failed to get BIOS settings difference: %w", err)
	}

	if len(settingsDiff) == 0 {
		log.V(1).Info("No BIOS settings difference found to apply on server", "currentSettings Name", flowItem.Name)
		if err := r.Conditions.Update(
			issueBiosUpdate,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BIOSSettingReasonIssuedUpdate),
			conditionutils.UpdateMessage("BIOS Settings issue has been Skipped on the server as no difference found"),
		); err != nil {
			return fmt.Errorf("failed to update issued settings update condition: %w", err)
		}
		err = r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, issueBiosUpdate)
		log.V(1).Info("Reconciled BIOSSettings at issue Settings to server state", "SettingsFlow", flowItem.Name)
		return err
	}
	// check if the pending tasks not present on the bios settings
	pendingSettings, err := r.getPendingBIOSSettings(ctx, bmcClient, server)
	if err != nil {
		return fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}
	var pendingSettingsDiff redfish.SettingsAttributes
	if len(pendingSettings) == 0 {
		log.V(1).Info("Applying settings", "settingsDiff", settingsDiff, "SettingsName", flowItem.Name)
		err = bmcClient.SetBiosAttributesOnReset(ctx, server.Spec.SystemURI, settingsDiff)
		if err != nil {
			return fmt.Errorf("failed to set BMC settings: %w", err)
		}
	} else {
		// this can only happen if we have issued the settings update
		// or unexpected pending settings found because of spec update during Inprogress of settings
		return fmt.Errorf("pending settings found on BIOS, cannot issue new settings update. pending settings: %v", pendingSettings)
	}

	// Get the latest pending settings and expect it to be zero different from the required settings.
	pendingSettings, err = r.getPendingBIOSSettings(ctx, bmcClient, server)
	if err != nil {
		return fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}

	skipReboot, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionRebootPostUpdate)
	if err != nil {
		return fmt.Errorf("failed to get Condition for reboot needed condition %w", err)
	}

	// At this point the BIOS setting update needs to be already issued.
	// if no reboot is required, most likely the settings is already applied,
	// hence no pending task will be present.
	if len(pendingSettings) == 0 && skipReboot.Status == metav1.ConditionFalse {
		// todo: fail after X amount of time
		log.V(1).Info("BIOSSettings update issued to BMC was not accepted. retrying....")
		return errors.Join(err, fmt.Errorf("bios setting issued to bmc not accepted"))
	}

	pendingSettingsDiff = make(redfish.SettingsAttributes, len(settingsDiff))
	for name, value := range settingsDiff {
		if pendingValue, ok := pendingSettings[name]; ok && value != pendingValue {
			pendingSettingsDiff[name] = pendingValue
		}
	}

	// all required settings should in pending settings.
	if len(pendingSettingsDiff) > 0 {
		log.V(1).Info("Difference between the pending settings and that of required", "SettingsDiff", pendingSettingsDiff)
		unexpectedPendingSettings, err := GetCondition(r.Conditions, flowStatus.Conditions, BIOSSettingsConditionUnknownPendingSettings)
		if err != nil {
			return fmt.Errorf("failed to get Condition for unexpected pending BIOSSetting state %v", err)
		}
		if err := r.Conditions.Update(
			unexpectedPendingSettings,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BIOSSettingsReasonUnexpectedPendingSettings),
			conditionutils.UpdateMessage(fmt.Sprintf("Found unexpected settings after issuing settings update for BIOS. unexpected settings %v", pendingSettingsDiff)),
		); err != nil {
			return fmt.Errorf("failed to update unexpected pending BIOSSettings found condition: %w", err)
		}
		err = r.updateFlowStatus(ctx, settings, metalv1alpha1.BIOSSettingsFlowStateFailed, flowStatus, unexpectedPendingSettings)
		return errors.Join(err, r.updateStatus(ctx, settings, metalv1alpha1.BIOSSettingsStateFailed, nil))
	}

	if err := r.Conditions.Update(
		issueBiosUpdate,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(BIOSSettingReasonIssuedUpdate),
		conditionutils.UpdateMessage("BIOS settings update has been triggered on the server"),
	); err != nil {
		return fmt.Errorf("failed to update issued BIOSSettings update condition: %w", err)
	}

	return r.updateFlowStatus(ctx, settings, flowStatus.State, flowStatus, issueBiosUpdate)
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

	condition, err := GetCondition(r.Conditions, settings.Status.Conditions, BIOSServerMaintenanceConditionCreated)
	if err != nil {
		return false, err
	}
	if err := r.Conditions.Update(
		condition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(BIOSServerMaintenanceReasonCreated),
		conditionutils.UpdateMessage(fmt.Sprintf("Created %v at %v", serverMaintenance.Name, time.Now())),
	); err != nil {
		return false, fmt.Errorf("failed to update creating ServerMaintenance condition: %w", err)
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

	for _, flowState := range settings.Status.FlowState {
		if flowState.Priority == flowItem.Priority &&
			flowItem.Name == flowState.Name {
			return &flowState
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
