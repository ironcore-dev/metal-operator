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

// BiosSettingsReconciler reconciles a BIOSSettings object
type BiosSettingsReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.Options
	ResyncInterval   time.Duration
	TimeoutExpiry    time.Duration
}

const (
	BIOSSettingsFinalizer = "metal.ironcore.dev/biossettings"

	serverMaintenanceCreatedCondition = "ServerMaintenanceCreated"
	createdServerMaintenanceReason    = "CreatedServerMaintenance"
	serverMaintenanceDeletedCondition = "ServerMaintenanceDeleted"
	deleteServerMaintenanceReason     = "DeleteServerMaintenance"
	pendingVersionUpdateCondition     = "PendingBIOSVersionUpdate"
	pendingBIOSVersionUpgradeReason   = "PendingBIOSVersionUpgrade"
	pendingSettingCheckCondition      = "PendingSettingStateCheck"
	pendingBIOSSettingsFound          = "PendingBIOSSettingsFound"
	duplicateKeyCheckCondition        = "DuplicateKeysCheck"
	duplicateKeysFoundReason          = "DuplicateKeysFound"
	timeoutStartCondition             = "SettingUpdateStartTime"
	settingsUpdateStartedReason       = "SettingsUpdateStarted"
	timedOutCondition                 = "TimedOutDuringSettingUpdate"
	timeoutOutDuringUpdateReason      = "TimeoutOutDuringUpdate"
	turnServerOnCondition             = "TurnServerOnCondition"
	serverPoweredOnReason             = "ServerPoweredOn"
	BMCConditionReset                 = "BMCResetIssued"
	BMCReasonReset                    = "BMCResetIssued"
	issueSettingsUpdateCondition      = "IssueSettingsUpdate"
	issuedBIOSSettingUpdateReason     = "IssuedBIOSSettingUpdate"
	unknownPendingSettingCondition    = "UnknownPendingSettingStateCheck"
	unexpectedPendingSettingsReason   = "UnexpectedPendingSettingsPostSettingUpdate"
	skipRebootCondition               = "SkipServerRebootPostSettingUpdate"
	skipRebootPostSettingUpdateReason = "SkipRebootPostSettingUpdate"
	rebootPostSettingUpdateReason     = "RebootPostSettingUpdate"
	rebootPowerOffCondition           = "RebootPowerOff"
	rebootPowerOffCompletedReason     = "RebootPowerOffCompleted"
	rebootPowerOnCondition            = "RebootPowerOn"
	rebootPowerOnCompletedReason      = "RebootPowerOnCompleted"
	verifySettingCondition            = "VerifySettingsPostUpdate"
	verificationCompleteReason        = "VerificationComplete"
)

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
func (r *BiosSettingsReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	biosSettings := &metalv1alpha1.BIOSSettings{}
	if err := r.Get(ctx, req.NamespacedName, biosSettings); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling biosSettings")

	return r.reconcileExists(ctx, log, biosSettings)
}

// Determine whether reconciliation is required. It's not required if:
// - object is being deleted;
// - object does not contain reference to server;
// - object contains reference to server, but server references to another object with lower version;
func (r *BiosSettingsReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if r.shouldDelete(log, biosSettings) {
		log.V(1).Info("Object is being deleted")
		return r.delete(ctx, log, biosSettings)
	}

	return r.reconcile(ctx, log, biosSettings)
}

func (r *BiosSettingsReconciler) shouldDelete(
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
) bool {
	if biosSettings.DeletionTimestamp.IsZero() {
		return false
	}

	if controllerutil.ContainsFinalizer(biosSettings, BIOSSettingsFinalizer) &&
		biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
		log.V(1).Info("Postponing delete as Settings update is in progress")
		return false
	}
	return true
}

func (r *BiosSettingsReconciler) delete(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
) (ctrl.Result, error) {
	if err := r.cleanupReferences(ctx, log, biosSettings); err != nil {
		log.Error(err, "failed to cleanup references")
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured references were cleaned up")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosSettings, BIOSSettingsFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("BIOSSettings is deleted")
	return ctrl.Result{}, nil
}

func (r *BiosSettingsReconciler) cleanupServerMaintenanceReferences(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
) error {
	if biosSettings.Spec.ServerMaintenanceRef == nil {
		return nil
	}
	// try to get the serverMaintaince created
	serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, biosSettings.Spec.ServerMaintenanceRef)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get referred serverMaintenance obj from biosSettings: %w", err)
	}

	var condition *metav1.Condition
	// if we got the server ref. by and its not being deleted
	if err == nil && serverMaintenance.DeletionTimestamp.IsZero() {
		// created by the controller
		if metav1.IsControlledBy(serverMaintenance, biosSettings) {
			// if the biosSettings is not being deleted, update the
			log.V(1).Info("Deleting server maintenance", "serverMaintenance Name", serverMaintenance.Name, "state", serverMaintenance.Status.State)
			acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
			condition, err = r.getCondition(acc, biosSettings.Status.Conditions, serverMaintenanceDeletedCondition)
			if err != nil {
				return fmt.Errorf("failed to get the delete condition while clean up maintenance %v", err)
			}
			if err := acc.Update(
				condition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(deleteServerMaintenanceReason),
				conditionutils.UpdateMessage(fmt.Sprintf("Deleting %v", serverMaintenance.Name)),
			); err != nil {
				return fmt.Errorf("failed to update deleting serverMaintenance condition: %w", err)
			}
			if err := r.Delete(ctx, serverMaintenance); err != nil {
				return err
			}
		} else { // not created by controller
			log.V(1).Info("Server maintenance status not updated as its provided by user", "serverMaintenance Name", serverMaintenance.Name, "state", serverMaintenance.Status.State)
		}
	}
	// if already deleted or we deleted it or its created by user, remove reference
	if apierrors.IsNotFound(err) || err == nil {
		err = r.patchMaintenanceRequestRefOnBiosSettings(ctx, log, biosSettings, nil, condition)
		if err != nil {
			return fmt.Errorf("failed to clean up serverMaintenance ref in biosSettings status: %w", err)
		}
	}
	return nil
}

func (r *BiosSettingsReconciler) cleanupReferences(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
) (err error) {
	if biosSettings.Spec.ServerRef != nil {
		server, err := r.getReferredServer(ctx, log, biosSettings.Spec.ServerRef)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		// if we can not find the server, nothing else to clean up
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Referred Server is gone")
			return nil
		}
		// if we have found the server, check if ref is this serevrBIOS and remove it
		if err == nil {
			if server.Spec.BIOSSettingsRef != nil {
				if server.Spec.BIOSSettingsRef.Name != biosSettings.Name {
					return nil
				}
				return r.patchBiosSettingsRefOnServer(ctx, log, server, nil)
			} else {
				// nothing else to clean up
				return nil
			}
		}
	}

	return err
}

func (r *BiosSettingsReconciler) reconcile(ctx context.Context, log logr.Logger, biosSettings *metalv1alpha1.BIOSSettings) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(biosSettings) {
		log.V(1).Info("Skipped BIOS Setting reconciliation")
		return ctrl.Result{}, nil
	}

	// if object does not refer to server object - stop reconciliation
	if biosSettings.Spec.ServerRef == nil {
		log.V(1).Info("Object does not refer to server object")
		return ctrl.Result{}, nil
	}

	// if referred server contains reference to different BIOSSettings object - stop reconciliation
	server, err := r.getReferredServer(ctx, log, biosSettings.Spec.ServerRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Referred server object is not found")
			// here we will move it to pending state,
			// to allow deletion of the resource and allow re-apply of setting if server comes back.
			err := r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStatePending, nil)
			return ctrl.Result{}, err
		}
		log.V(1).Info("Referred server object could not be fetched")
		return ctrl.Result{}, err
	}
	// patch server with biossettings reference
	if server.Spec.BIOSSettingsRef == nil {
		if err := r.patchBiosSettingsRefOnServer(ctx, log, server, &corev1.LocalObjectReference{Name: biosSettings.Name}); err != nil {
			return ctrl.Result{}, err
		}
	} else if server.Spec.BIOSSettingsRef.Name != biosSettings.Name {
		referredBIOSSetting, err := r.getReferredBIOSSettings(ctx, log, server.Spec.BIOSSettingsRef)
		if err != nil {
			log.V(1).Info("Referred server contains reference to different BIOSSettings object, unable to fetch the referenced bios setting")
			return ctrl.Result{}, err
		}
		// check if the current BIOS setting version is newer and update reference if it is newer
		// todo : handle version checks correctly
		if referredBIOSSetting.Spec.Version < biosSettings.Spec.Version {
			log.V(1).Info("Updating BIOSSetting reference to the latest BIOS version")
			if err := r.patchBiosSettingsRefOnServer(ctx, log, server, &corev1.LocalObjectReference{Name: biosSettings.Name}); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, biosSettings, BIOSSettingsFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		if errors.As(err, &bmcutils.BMCUnAvailableError{}) {
			log.V(1).Info("BMC is not available, skipping", "BMC", server.Spec.BMCRef.Name, "Server", server.Name, "error", err)
			return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client for server: %w", err)
	}
	defer bmcClient.Logout()

	return r.ensureBIOSSettingsStateTransition(ctx, log, bmcClient, biosSettings, server)
}

func (r *BiosSettingsReconciler) ensureBIOSSettingsStateTransition(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	switch biosSettings.Status.State {
	case "", metalv1alpha1.BIOSSettingsStatePending:
		return r.handleSettingPendingState(ctx, log, bmcClient, biosSettings, server)
	case metalv1alpha1.BIOSSettingsStateInProgress:
		return r.handleSettingInProgressState(ctx, log, bmcClient, biosSettings, server)
	case metalv1alpha1.BIOSSettingsStateApplied:
		return r.handleSettingAppliedState(ctx, log, bmcClient, biosSettings, server)
	case metalv1alpha1.BIOSSettingsStateFailed:
		return r.handleFailedState(ctx, log, biosSettings, server)
	}
	log.V(1).Info("Unknown State found", "BIOSSettings state", biosSettings.Status.State)
	return ctrl.Result{}, nil
}

func (r *BiosSettingsReconciler) handleSettingPendingState(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	if len(biosSettings.Spec.SettingsFlow) == 0 {
		log.V(1).Info("Skipped BIOSSetting as no settings found")
		err := r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStateApplied, nil)
		return ctrl.Result{}, err
	}
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	pendingSettings, err := r.getPendingSettingsOnBIOS(ctx, log, bmcClient, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get pending settings on bios: %w", err)
	}
	if len(pendingSettings) > 0 {
		log.V(1).Info("Pending bios setting tasks found", "biosSettings pending tasks", pendingSettings)
		pendingSettingStateCheckCondition, err := r.getCondition(acc, biosSettings.Status.Conditions, pendingSettingCheckCondition)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get Condition for pending Settings state %v", err)
		}
		if err := acc.Update(
			pendingSettingStateCheckCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(pendingBIOSSettingsFound),
			conditionutils.UpdateMessage(fmt.Sprintf("Pending Setting found, Hence can not start with bios setting update, current pending settings: %v", pendingSettings)),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update Pending BIOSVersion update condition: %w", err)
		}
		err = r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStateFailed, pendingSettingStateCheckCondition)
		return ctrl.Result{}, err
	}

	// verify no duplicate name and duplicate settings are found
	// as this will cause infinite loops
	allNames := map[string]struct{}{}
	allSettingsNames := map[string]struct{}{}
	duplicateName := make([]string, 0, len(biosSettings.Spec.SettingsFlow))
	duplicateSettingsNames := make([]string, 0, len(biosSettings.Spec.SettingsFlow))
	for _, settings := range biosSettings.Spec.SettingsFlow {

		if _, ok := allNames[settings.Name]; ok {
			duplicateName = append(duplicateName, settings.Name)
		}
		allNames[settings.Name] = struct{}{}
		for key := range settings.Settings {
			if _, ok := allSettingsNames[key]; ok {
				duplicateSettingsNames = append(duplicateSettingsNames, key)
			}
			allSettingsNames[key] = struct{}{}
		}
	}

	if len(duplicateName) > 0 || len(duplicateSettingsNames) > 0 {
		log.V(1).Info("Duplicate keys found", "duplicate names", duplicateName, "duplicate keys", duplicateSettingsNames)
		duplicateCheckCondition, err := r.getCondition(acc, biosSettings.Status.Conditions, duplicateKeyCheckCondition)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get Condition for pending Settings state %v", err)
		}
		if err := acc.Update(
			duplicateCheckCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(duplicateKeysFoundReason),
			conditionutils.UpdateMessage(fmt.Sprintf("Found duplicate Keys in Name: %v or settings Keys %v", duplicateName, duplicateSettingsNames)),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update Pending BIOSVersion update condition: %w", err)
		}
		err = r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStateFailed, duplicateCheckCondition)
		return ctrl.Result{}, err
	}

	// find if all settings have already been applied.
	currentBiosVersion, settingsDiff, err := r.getBIOSVersionAndSettingDifference(ctx, log, bmcClient, biosSettings, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings: %w", err)
	}
	// if setting is not different, complete the BIOS tasks, does not matter if the bios version do not match
	// if conditions are present, skip this shortcut to be able capture all conditions states (ex: verifySetting, reboot etc)
	if len(settingsDiff) == 0 && len(biosSettings.Status.Conditions) == 0 {
		// move status to completed
		verifySettingUpdate, err := r.getCondition(acc, biosSettings.Status.Conditions, verifySettingCondition)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get Condition for Verifyed Settings condition %v", err)
		}
		// move  biosSettings state to completed
		if err := acc.Update(
			verifySettingUpdate,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(verificationCompleteReason),
			conditionutils.UpdateMessage("Required BIOS settings has been verified on the server"),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update verify biossetting condition: %w", err)
		}
		err = r.updateBiosSettingsStatus(
			ctx,
			log,
			biosSettings,
			metalv1alpha1.BIOSSettingsStateApplied,
			verifySettingUpdate)
		return ctrl.Result{}, err
	}

	var state = metalv1alpha1.BIOSSettingsStateInProgress
	var condition *metav1.Condition
	if currentBiosVersion != biosSettings.Spec.Version {
		versionCheckCondition, err := r.getCondition(acc, biosSettings.Status.Conditions, pendingVersionUpdateCondition)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get Condition for pending BIOSVersion update state %v", err)
		}
		if versionCheckCondition.Status == metav1.ConditionTrue {
			log.V(1).Info("Pending BIOS version upgrade.", "current bios Version", currentBiosVersion, "required version", biosSettings.Spec.Version)
			return ctrl.Result{}, nil
		}
		if err := acc.Update(
			versionCheckCondition,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(pendingBIOSVersionUpgradeReason),
			conditionutils.UpdateMessage(fmt.Sprintf("Waiting to update biosVersion: %v, current biosVersion: %v", biosSettings.Spec.Version, currentBiosVersion)),
		); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update Pending BIOSVersion update condition: %w", err)
		}
		state = metalv1alpha1.BIOSSettingsStatePending
		condition = versionCheckCondition
	}
	return ctrl.Result{}, r.updateBiosSettingsStatus(ctx, log, biosSettings, state, condition)
}

func (r *BiosSettingsReconciler) handleSettingInProgressState(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	if req, err := r.requestMaintenanceOnServer(ctx, log, biosSettings, server); err != nil || req {
		return ctrl.Result{}, err
	}

	// check if the maintenance is granted
	if ok := r.checkIfMaintenanceGranted(log, biosSettings, server); !ok {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with updating settings")
		return ctrl.Result{}, nil
	}

	if ok, err := r.handleBMCReset(ctx, log, bmcClient, biosSettings, server); !ok || err != nil {
		return ctrl.Result{}, err
	}

	settingsFlow := append([]metalv1alpha1.SettingsFlowItem{}, biosSettings.Spec.SettingsFlow...)

	sort.Slice(settingsFlow, func(i, j int) bool {
		return settingsFlow[i].Priority <= settingsFlow[j].Priority
	})

	// loop through all the sequence in priority order and verify/Apply the settings
	for _, settings := range settingsFlow {
		// check each settings in the order of priority apply and verify it
		currentSettingsFlowStatus := r.getCurrentSettingsFlowStatus(biosSettings, &settings)

		// if the setting state is not found, create it
		if currentSettingsFlowStatus == nil {
			// create settingsFlow state
			currentSettingsFlowStatus = &metalv1alpha1.BIOSSettingsFlowStatus{
				Priority: settings.Priority,
				Name:     settings.Name,
			}
			err := r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsFlowStateInProgress, currentSettingsFlowStatus, nil)
			return ctrl.Result{}, err
		}

		// if the state is InProgress, go ahead and apply/Verify the settings
		if currentSettingsFlowStatus.State != metalv1alpha1.BIOSSettingsFlowStateInProgress {
			// else, check if the settings is still as expected, and proceed.
			settingDiff, err := r.getCurrentSettingDifference(ctx, log, bmcClient, settings.Settings, server)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed get current Settings difference. current Settings Name: %v, error: %v", settings.Name, err)
			}

			// Handle if no setting update is needed
			if len(settingDiff) == 0 {
				// if the state reflects it move on
				if currentSettingsFlowStatus.State == metalv1alpha1.BIOSSettingsFlowStateApplied {
					continue
				}
				// mark completed, and move on
				acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
				verifySettingUpdate, err := r.getCondition(acc, currentSettingsFlowStatus.Conditions, verifySettingCondition)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to get Condition for Verifyed Settings condition %v", err)
				}
				// move  biosSettings state to completed
				if err := acc.Update(
					verifySettingUpdate,
					conditionutils.UpdateStatus(corev1.ConditionTrue),
					conditionutils.UpdateReason(verificationCompleteReason),
					conditionutils.UpdateMessage("Required BIOS settings has been RE verified on the server. Hence, moving out of Pending state"),
				); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to update verify biossetting condition: %w", err)
				}
				err = r.updateBiosSettingsFlowStatus(
					ctx, log, biosSettings, metalv1alpha1.BIOSSettingsFlowStateApplied,
					currentSettingsFlowStatus, verifySettingUpdate,
				)
				return ctrl.Result{}, err
			}

			// If the settings is different and the status was previously applied.
			// make sure to reapply settings, reset any other InProgress state for higher Priority Settings.
			if currentSettingsFlowStatus.State == metalv1alpha1.BIOSSettingsFlowStateApplied {
				// update the state to reflect the current settings we are about to apply
				// may be add condition to indicate the reapply
				err := r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsFlowStateInProgress, currentSettingsFlowStatus, nil)
				return ctrl.Result{}, err
			}
		}

		// apply the current settings
		if ok, err := r.applySettingUpdate(ctx, log, bmcClient, biosSettings, &settings, currentSettingsFlowStatus, server); ok && err == nil {
			if requeue, err := r.VerifySettingsUpdateComplete(ctx, log, bmcClient, biosSettings, &settings, currentSettingsFlowStatus, server); requeue && err == nil {
				return ctrl.Result{RequeueAfter: r.ResyncInterval}, err
			}
			return ctrl.Result{}, err
		} else {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStateApplied, nil)
}

func (r *BiosSettingsReconciler) handleBMCReset(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (bool, error) {

	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	// reset BMC if not already done
	resetBMC, err := r.getCondition(acc, biosSettings.Status.Conditions, BMCConditionReset)
	if err != nil {
		return false, fmt.Errorf("failed to get condition for reset of BMC of server %v", err)
	}

	if resetBMC.Status != metav1.ConditionTrue {
		// once the server is powered on, reset the BMC to make sure its in stable state
		// this avoids problems with some BMCs that hang up in subsequent operations
		if resetBMC.Reason != BMCReasonReset {
			if err := resetBMCOfServer(ctx, log, r.Client, server, bmcClient); err == nil {
				// mark reset to be issued, wait for next reconcile
				if err := acc.Update(
					resetBMC,
					conditionutils.UpdateStatus(corev1.ConditionFalse),
					conditionutils.UpdateReason(BMCReasonReset),
					conditionutils.UpdateMessage("Issued BMC reset to stabilize BMC of the server"),
				); err != nil {
					return false, fmt.Errorf("failed to update reset BMC condition: %w", err)
				}
				return false, r.updateBiosSettingsStatus(ctx, log, biosSettings, biosSettings.Status.State, resetBMC)
			} else {
				log.V(1).Error(err, "failed to reset BMC of the server")
				return false, err
			}
		} else if server.Spec.BMCRef != nil {
			// we need to wait until the BMC resource annotation is removed
			key := types.NamespacedName{Name: server.Spec.BMCRef.Name}
			BMC := &metalv1alpha1.BMC{}
			if err := r.Get(ctx, key, BMC); err != nil {
				log.V(1).Error(err, "failed to get referred server's Manager")
				return false, err
			}
			annotations := BMC.GetAnnotations()
			if annotations != nil {
				if op, ok := annotations[metalv1alpha1.OperationAnnotation]; ok {
					if op == metalv1alpha1.GracefulRestartBMC {
						log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
						return false, nil
					}
				}
			}
		}
		if err := acc.Update(
			resetBMC,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(BMCReasonReset),
			conditionutils.UpdateMessage("BMC reset to stabilize BMC of the server is completed"),
		); err != nil {
			return false, fmt.Errorf("failed to update power on server condition: %w", err)
		}
		return false, r.updateBiosSettingsStatus(ctx, log, biosSettings, biosSettings.Status.State, resetBMC)
	}
	return true, nil
}

func (r *BiosSettingsReconciler) applySettingUpdate(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosSettings *metalv1alpha1.BIOSSettings,
	currentSettings *metalv1alpha1.SettingsFlowItem,
	currentFlowStatus *metalv1alpha1.BIOSSettingsFlowStatus,
	server *metalv1alpha1.Server,
) (bool, error) {
	if modified, err := r.SetTimeOutForApplyingSettings(ctx, log, biosSettings, currentFlowStatus); modified || err != nil {
		return false, err
	}
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	turnOnServer, err := r.getCondition(acc, currentFlowStatus.Conditions, turnServerOnCondition)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for Initial powerOn of server %v", err)
	}

	if turnOnServer.Status != metav1.ConditionTrue {
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			if err := acc.Update(
				turnOnServer,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(serverPoweredOnReason),
				conditionutils.UpdateMessage("Server is powered On to start the biosUpdate process"),
			); err != nil {
				return false, fmt.Errorf("failed to update power on server condition: %w", err)
			}
			return false, r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, currentFlowStatus.State, currentFlowStatus, turnOnServer)
		}
		// we need to request maintenance to get the server to power-On to apply the BIOS settings
		if biosSettings.Spec.ServerMaintenanceRef == nil {
			log.V(1).Info("Server powered off, request maintenance to turn the server On")
			if requeue, err := r.requestMaintenanceOnServer(ctx, log, biosSettings, server); err != nil || requeue {
				return false, err
			}
		}

		err := r.patchServerMaintenancePowerState(ctx, log, biosSettings, metalv1alpha1.PowerOn)
		if err != nil {
			return false, fmt.Errorf("failed to turn on server %w", err)
		}
		log.V(1).Info("Reconciled biosSettings at TurnOnServer Condition")
		return false, err
	}

	// check if we have already determined if we need reboot of not.
	// if the condition is present, we have checked the skip reboot condition.
	condFound, err := acc.FindSlice(
		currentFlowStatus.Conditions,
		skipRebootCondition,
		&metav1.Condition{})
	if err != nil {
		return false, fmt.Errorf("failed to find Condition %v. error: %v", skipRebootCondition, err)
	}
	if !condFound {
		log.V(1).Info("Verify if the current Settings needs reboot of server")
		settingsDiff, err := r.getCurrentSettingDifference(ctx, log, bmcClient, currentSettings.Settings, server)
		if err != nil {
			return false, fmt.Errorf("failed to get BIOS settings difference: %w", err)
		}
		resetReq, err := bmcClient.CheckBiosAttributes(settingsDiff)
		if err != nil {
			log.V(1).Error(err, "could not determine if reboot needed")
			return false, err
		}

		skipReboot, err := r.getCondition(acc, currentFlowStatus.Conditions, skipRebootCondition)
		if err != nil {
			return false, fmt.Errorf("failed to get Condition for skip reboot post setting update %v", err)
		}

		// if we dont need reboot. skip reboot steps.
		if !resetReq {
			log.V(1).Info("BIOSSettings update does not need reboot")
			if err := acc.Update(
				skipReboot,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(skipRebootPostSettingUpdateReason),
				conditionutils.UpdateMessage("Settings provided does not need server reboot"),
			); err != nil {
				return false, fmt.Errorf("failed to update skip reboot condition: %w", err)
			}
		} else {
			if err := acc.Update(
				skipReboot,
				conditionutils.UpdateStatus(corev1.ConditionFalse),
				conditionutils.UpdateReason(rebootPostSettingUpdateReason),
				conditionutils.UpdateMessage("Settings provided needs server reboot"),
			); err != nil {
				return false, fmt.Errorf("failed to update skip reboot condition: %w", err)
			}
		}
		err = r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, currentFlowStatus.State, currentFlowStatus, skipReboot)
		log.V(1).Info("Reconciled biosSettings at check if reboot is needed")
		return false, err
	}

	issueBiosUpdate, err := r.getCondition(acc, currentFlowStatus.Conditions, issueSettingsUpdateCondition)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for issuing BIOSSetting update to server %v", err)
	}

	if issueBiosUpdate.Status != metav1.ConditionTrue {
		return false, r.applyBiosSettingOnServer(ctx, log, bmcClient, biosSettings, currentSettings, currentFlowStatus, server, issueBiosUpdate)
	}

	skipReboot, err := r.getCondition(acc, currentFlowStatus.Conditions, skipRebootCondition)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for reboot needed condition %v", err)
	}

	if skipReboot.Status != metav1.ConditionTrue {
		rebootPowerOnCondition, err := r.getCondition(acc, currentFlowStatus.Conditions, rebootPowerOnCondition)
		if err != nil {
			return false, fmt.Errorf("failed to get Condition for reboot PowerOn condition %v", err)
		}
		// reboot is not yet completed
		if rebootPowerOnCondition.Status != metav1.ConditionTrue {
			return false, r.rebootServer(ctx, log, biosSettings, currentFlowStatus, server)
		}
	}
	return true, nil
}

func (r *BiosSettingsReconciler) SetTimeOutForApplyingSettings(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	currentFlowStatus *metalv1alpha1.BIOSSettingsFlowStatus,
) (bool, error) {
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	timeoutCheck, err := r.getCondition(acc, currentFlowStatus.Conditions, timeoutStartCondition)
	if err != nil {
		return false, fmt.Errorf("failed to get condition for TimeOut during setting update %v", err)
	}
	if timeoutCheck.Status != metav1.ConditionTrue {
		if err := acc.Update(
			timeoutCheck,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(settingsUpdateStartedReason),
			conditionutils.UpdateMessage("Settings are being updated on Server. Timeout will occur beyond this point if settings are not applied"),
		); err != nil {
			return false, fmt.Errorf("failed to update starting setting update condition: %w", err)
		}
		err = r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, currentFlowStatus.State, currentFlowStatus, timeoutCheck)
		return true, err
	} else {
		startTime := timeoutCheck.LastTransitionTime.Time
		if time.Now().After(startTime.Add(r.TimeoutExpiry)) {
			log.V(1).Info("Timedout while updating the biosSettings")
			timedOut, err := r.getCondition(acc, currentFlowStatus.Conditions, timedOutCondition)
			if err != nil {
				return false, fmt.Errorf("failed to get Condition for Timeout of BIOSSettings update %v", err)
			}
			if err := acc.Update(
				timedOut,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(timeoutOutDuringUpdateReason),
				conditionutils.UpdateMessage(fmt.Sprintf("Timeout after: %v. startTime: %v. timedOut on: %v", r.TimeoutExpiry, startTime, time.Now().String())),
			); err != nil {
				return false, fmt.Errorf("failed to update timeout during settings update condition: %w", err)
			}
			err = r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsFlowStateFailed, currentFlowStatus, timedOut)
			return true, err
		}
	}
	return false, nil
}

func (r *BiosSettingsReconciler) VerifySettingsUpdateComplete(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosSettings *metalv1alpha1.BIOSSettings,
	currentSettings *metalv1alpha1.SettingsFlowItem,
	currentFlowStatus *metalv1alpha1.BIOSSettingsFlowStatus,
	server *metalv1alpha1.Server,
) (bool, error) {
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	verifySettingUpdate, err := r.getCondition(acc, currentFlowStatus.Conditions, verifySettingCondition)
	if err != nil {
		return false, fmt.Errorf("failed to get Condition for Verification condition %v", err)
	}
	if verifySettingUpdate.Status != metav1.ConditionTrue {
		// make sure the setting has actually applied.
		settingsDiff, err := r.getCurrentSettingDifference(ctx, log, bmcClient, currentSettings.Settings, server)

		if err != nil {
			return false, fmt.Errorf("failed to get BIOS settings: %w", err)
		}
		// if setting is not different, complete the BIOS tasks
		if len(settingsDiff) == 0 {
			if err := acc.Update(
				verifySettingUpdate,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(verificationCompleteReason),
				conditionutils.UpdateMessage("Required BIOS settings has been applied and verified on the server"),
			); err != nil {
				return false, fmt.Errorf("failed to update verify biossetting condition: %w", err)
			}
			log.V(1).Info("Verified BIOS setting sequence", "Name", currentFlowStatus.Name)
			err := r.updateBiosSettingsFlowStatus(
				ctx,
				log,
				biosSettings,
				metalv1alpha1.BIOSSettingsFlowStateApplied,
				currentFlowStatus,
				verifySettingUpdate)
			return false, err
		}
		log.V(1).Info("Waiting on the BIOS setting to take place")
		return true, nil
	}
	log.V(1).Info("BIOS settings have been applied and verified on the server", "currentSettings Name", currentSettings.Name)
	return false, nil
}

func (r *BiosSettingsReconciler) rebootServer(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	currentFlowStatus *metalv1alpha1.BIOSSettingsFlowStatus,
	server *metalv1alpha1.Server,
) error {
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	rebootPowerOffCondition, err := r.getCondition(acc, currentFlowStatus.Conditions, rebootPowerOffCondition)
	if err != nil {
		return fmt.Errorf("failed to get Condition for reboot PowerOff condition %v", err)
	}

	if rebootPowerOffCondition.Status != metav1.ConditionTrue {
		// expected state it to be off and initial state is to be on.
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.patchServerMaintenancePowerState(ctx, log, biosSettings, metalv1alpha1.PowerOff)
			if err != nil {
				return fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOffPowerState) {
			if err := acc.Update(
				rebootPowerOffCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(rebootPowerOffCompletedReason),
				conditionutils.UpdateMessage("Server has entered power off state"),
			); err != nil {
				return fmt.Errorf("failed to update reboot server powerOff condition: %w", err)
			}
			err = r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, currentFlowStatus.State, currentFlowStatus, rebootPowerOffCondition)
			return err
		}
		log.V(1).Info("Reconciled biosSettings at reboot wait for power off")
		return nil
	}

	rebootPowerOnCondition, err := r.getCondition(acc, currentFlowStatus.Conditions, rebootPowerOnCondition)
	if err != nil {
		return fmt.Errorf("failed to get Condition for reboot PowerOn condition %v", err)
	}

	if rebootPowerOnCondition.Status != metav1.ConditionTrue {
		// expected power state it to be on and initial state is to be off.
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOffPowerState) {
			err := r.patchServerMaintenancePowerState(ctx, log, biosSettings, metalv1alpha1.PowerOn)
			if err != nil {
				return fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			if err := acc.Update(
				rebootPowerOnCondition,
				conditionutils.UpdateStatus(corev1.ConditionTrue),
				conditionutils.UpdateReason(rebootPowerOnCompletedReason),
				conditionutils.UpdateMessage("Server has entered power on state"),
			); err != nil {
				return fmt.Errorf("failed to update reboot server powerOn condition: %w", err)
			}
			err = r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, currentFlowStatus.State, currentFlowStatus, rebootPowerOnCondition)
			return err
		}
		log.V(1).Info("Reconciled biosSettings at reboot wait for power on")
		return nil
	}
	return nil
}

func (r *BiosSettingsReconciler) applyBiosSettingOnServer(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosSettings *metalv1alpha1.BIOSSettings,
	currentSettings *metalv1alpha1.SettingsFlowItem,
	currentFlowStatus *metalv1alpha1.BIOSSettingsFlowStatus,
	server *metalv1alpha1.Server,
	issueBiosUpdate *metav1.Condition,
) error {
	settingsDiff, err := r.getCurrentSettingDifference(ctx, log, bmcClient, currentSettings.Settings, server)
	if err != nil {
		return fmt.Errorf("failed to get BIOS settings difference: %w", err)
	}
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	// check if the pending tasks not present on the bios settings
	pendingSettings, err := r.getPendingSettingsOnBIOS(ctx, log, bmcClient, server)
	if err != nil {
		return fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}
	var pendingSettingsDiff redfish.SettingsAttributes
	if len(pendingSettings) == 0 {
		log.V(1).Info("Applying settings", "settingsDiff", settingsDiff, "SettingsName", currentSettings.Name)
		err = bmcClient.SetBiosAttributesOnReset(ctx, server.Spec.SystemURI, settingsDiff)
		if err != nil {
			return fmt.Errorf("failed to set BMC settings: %w", err)
		}
	}

	// Get the latest pending settings and expect it to be zero different from the required settings.
	pendingSettings, err = r.getPendingSettingsOnBIOS(ctx, log, bmcClient, server)
	if err != nil {
		return fmt.Errorf("failed to get pending BIOS settings: %w", err)
	}

	skipReboot, err := r.getCondition(acc, currentFlowStatus.Conditions, skipRebootCondition)
	if err != nil {
		return fmt.Errorf("failed to get Condition for reboot needed condition %v", err)
	}

	// At this point the BIOS setting update needs to be already issued.
	// if no reboot is required, postlikely the settings is already applied,
	// hence no pending task will be present.
	if len(pendingSettings) == 0 && skipReboot.Status == metav1.ConditionFalse {
		// todo: fail after X amount of time
		log.V(1).Info("BIOS Setting update issued to BMC was not accepted. retrying....")
		return errors.Join(err, fmt.Errorf("bios setting issued to bmc not accepted"))
	}

	pendingSettingsDiff = r.checkPendingSettingsDiff(log, pendingSettings, settingsDiff)

	// all required settings should in pending settings.
	if len(pendingSettingsDiff) > 0 {
		log.V(1).Info("Unknown pending BIOS settings found", "Unknown pending settings", pendingSettingsDiff)
		unexpectedPendingSettings, err := r.getCondition(acc, currentFlowStatus.Conditions, unknownPendingSettingCondition)
		if err != nil {
			return fmt.Errorf("failed to get Condition for unexpected pending BIOSSetting state %v", err)
		}
		if err := acc.Update(
			unexpectedPendingSettings,
			conditionutils.UpdateStatus(corev1.ConditionTrue),
			conditionutils.UpdateReason(unexpectedPendingSettingsReason),
			conditionutils.UpdateMessage(fmt.Sprintf("Found unexpected settings after issuing settings update for BIOS. unexpected settings %v", pendingSettingsDiff)),
		); err != nil {
			return fmt.Errorf("failed to update unexpected pending settings found condition: %w", err)
		}
		err = r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsFlowStateFailed, currentFlowStatus, unexpectedPendingSettings)
		return err
	}

	if err := acc.Update(
		issueBiosUpdate,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(issuedBIOSSettingUpdateReason),
		conditionutils.UpdateMessage("BIOS Settings Update has been triggered on the server"),
	); err != nil {
		return fmt.Errorf("failed to update issued settings update condition: %w", err)
	}
	err = r.updateBiosSettingsFlowStatus(ctx, log, biosSettings, currentFlowStatus.State, currentFlowStatus, issueBiosUpdate)
	log.V(1).Info("Reconciled biosSettings at issue Settings to server state", "currentSettings Name", currentSettings.Name)
	return err
}

func (r *BiosSettingsReconciler) ensureNoStrandedStatus(
	ctx context.Context,
	biosSettings *metalv1alpha1.BIOSSettings,
) (bool, error) {
	// Incase the settings Spec got changed during Inprogress and left behind Stale states clean it up.
	settingsNamePriorityMap := map[string]int32{}
	biosSettingsBase := biosSettings.DeepCopy()
	for _, settings := range biosSettings.Spec.SettingsFlow {
		settingsNamePriorityMap[settings.Name] = settings.Priority
	}
	nextFlowStatuses := make([]metalv1alpha1.BIOSSettingsFlowStatus, 0)
	for _, flowStatus := range biosSettings.Status.FlowState {
		if value, ok := settingsNamePriorityMap[flowStatus.Name]; ok && value == flowStatus.Priority {
			nextFlowStatuses = append(nextFlowStatuses, flowStatus)
		}
	}

	if len(nextFlowStatuses) != len(biosSettings.Status.FlowState) {
		biosSettings.Status.FlowState = nextFlowStatuses
		if err := r.Status().Patch(ctx, biosSettings, client.MergeFrom(biosSettingsBase)); err != nil {
			return false, fmt.Errorf("failed to patch BIOSSettings FlowState status: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func (r *BiosSettingsReconciler) handleSettingAppliedState(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	// clean up maintenance crd and references.
	if err := r.cleanupServerMaintenanceReferences(ctx, log, biosSettings); err != nil {
		return ctrl.Result{}, err
	}

	if requeue, err := r.ensureNoStrandedStatus(ctx, biosSettings); requeue || err != nil {
		return ctrl.Result{}, err
	}

	_, settingsDiff, err := r.getBIOSVersionAndSettingDifference(ctx, log, bmcClient, biosSettings, server)

	if err != nil {
		log.V(1).Error(err, "unable to fetch and check BIOSSettings")
		return ctrl.Result{}, err
	}
	if len(settingsDiff) > 0 {
		log.V(1).Info("Found bios setting difference after applied state", "settingsDiff", settingsDiff)
		err := r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStatePending, nil)
		return ctrl.Result{}, err
	}

	log.V(1).Info("Done with bios setting update", "ctx", ctx, "biosSettings", biosSettings, "server", server)
	return ctrl.Result{}, nil
}

func (r *BiosSettingsReconciler) handleFailedState(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	if shouldRetryReconciliation(biosSettings) {
		log.V(1).Info("Retrying BIOSSettings reconciliation")
		biosSettingsBase := biosSettings.DeepCopy()
		biosSettings.Status.State = metalv1alpha1.BIOSSettingsStatePending
		// todo: add FlowState reset after the #403 is merged
		biosSettings.Status.Conditions = nil
		annotations := biosSettings.GetAnnotations()
		delete(annotations, metalv1alpha1.OperationAnnotation)
		biosSettings.SetAnnotations(annotations)
		if err := r.Status().Patch(ctx, biosSettings, client.MergeFrom(biosSettingsBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch BIOSSettings status for retrying: %w", err)
		}
		return ctrl.Result{}, nil
	}
	// todo: revisit this logic to either create maintenance if not present, put server in Error state on failed bios settings maintenance
	log.V(1).Info("Failed to update bios setting", "ctx", ctx, "biosSettings", biosSettings, "server", server)
	return ctrl.Result{}, nil
}

func (r *BiosSettingsReconciler) checkPendingSettingsDiff(
	log logr.Logger,
	pendingSettings redfish.SettingsAttributes,
	settingsDiff redfish.SettingsAttributes,
) redfish.SettingsAttributes {
	// if settingsDiff is provided find the difference between settingsDiff and pending
	log.V(1).Info("Checking for the difference in the pending settings than that of required")
	unknownpendingSettings := make(redfish.SettingsAttributes, len(settingsDiff))
	for name, value := range settingsDiff {
		if pendingValue, ok := pendingSettings[name]; ok && value != pendingValue {
			unknownpendingSettings[name] = pendingValue
		}
	}
	return unknownpendingSettings
}

func (r *BiosSettingsReconciler) getPendingSettingsOnBIOS(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	server *metalv1alpha1.Server,
) (pendingSettings redfish.SettingsAttributes, err error) {
	log.V(1).Info("Fetching the pending settings on bios")

	pendingSettings, err = bmcClient.GetBiosPendingAttributeValues(ctx, server.Spec.SystemURI)
	if err != nil {
		return pendingSettings, err
	}

	return pendingSettings, nil
}

func (r *BiosSettingsReconciler) getCurrentSettingDifference(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	currentPrioritySettings map[string]string,
	server *metalv1alpha1.Server,
) (diff redfish.SettingsAttributes, err error) {

	keys := slices.Collect(maps.Keys(currentPrioritySettings))

	// get the accepted type/values from the server BIOS for given keys
	currentSettings, err := bmcClient.GetBiosAttributeValues(ctx, server.Spec.SystemURI, keys)
	if err != nil {
		log.V(1).Info("Failed to get with bios setting", "error", err)
		return diff, fmt.Errorf("failed to get BIOS settings: %w", err)
	}

	// check if the given settings match the accepted setting's type/values from server BIOS
	diff = redfish.SettingsAttributes{}
	var errs []error
	for key, value := range currentPrioritySettings {
		res, ok := currentSettings[key]
		if ok {
			switch data := res.(type) {
			case int:
				intvalue, err := strconv.Atoi(value)
				if err != nil {
					log.V(1).Info("Failed to check type for", "Setting name", key, "setting value", value, "error", err)
					errs = append(errs, fmt.Errorf("failed to check type for name %v; value %v; error: %v", key, value, err))
					continue
				}
				if data != intvalue {
					diff[key] = intvalue
				}
			case string:
				if data != value {
					diff[key] = value
				}
			case float64:
				floatvalue, err := strconv.ParseFloat(value, 64)
				if err != nil {
					log.V(1).Info("Failed to check type for", "Setting name", key, "setting value", value, "error", err)
					errs = append(errs, fmt.Errorf("failed to check type for name %v; value %v; error: %v", key, value, err))
				}
				if data != floatvalue {
					diff[key] = floatvalue
				}
			}
		} else {
			diff[key] = value
		}
	}

	if len(diff) > 0 {
		log.V(1).Info("current BIOS settings on the server", "currentSettings", currentSettings)
	}

	return diff, errors.Join(errs...)
}

func (r *BiosSettingsReconciler) getBIOSVersionAndSettingDifference(
	ctx context.Context,
	log logr.Logger,
	bmcClient bmc.BMC,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (currentbiosVersion string, diff redfish.SettingsAttributes, err error) {

	completeSettings := make(map[string]string)
	for _, settings := range biosSettings.Spec.SettingsFlow {
		for key, value := range settings.Settings {
			completeSettings[key] = value
		}
	}

	diff, err = r.getCurrentSettingDifference(ctx, log, bmcClient, completeSettings, server)

	if err != nil {
		log.V(1).Info("Failed to get bios setting differences for some settings", "error", err)
		return "", diff, fmt.Errorf("failed to find diff for some bios settings: %v", err)
	}

	// fetch the current bios version from the server bmc
	currentBiosVersion, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemURI)
	if err != nil {
		return "", diff, fmt.Errorf("failed to load bios version: %w for server %v", err, server.Name)
	}

	return currentBiosVersion, diff, nil
}

func (r *BiosSettingsReconciler) checkForRequiredPowerStatus(
	server *metalv1alpha1.Server,
	powerState metalv1alpha1.ServerPowerState,
) bool {
	return server.Status.PowerState == powerState
}

func (r *BiosSettingsReconciler) checkIfMaintenanceGranted(
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) bool {
	if biosSettings.Spec.ServerMaintenanceRef == nil {
		return true
	}

	if server.Status.State == metalv1alpha1.ServerStateMaintenance {
		if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.UID != biosSettings.Spec.ServerMaintenanceRef.UID {
			// server in maintenance for other tasks. or
			// server maintenance ref is wrong in either server or biosSettings
			// wait for update on the server obj
			log.V(1).Info("Server is already in maintenance for other tasks", "Server", server.Name, "serverMaintenanceRef", server.Spec.ServerMaintenanceRef)
			return false
		}
	} else {
		// we still need to wait for server to enter maintenance
		// wait for update on the server obj
		log.V(1).Info("Server not yet in maintenance", "Server", server.Name, "State", server.Status.State, "MaintenanceRef", server.Spec.ServerMaintenanceRef)
		return false
	}

	return true
}

func (r *BiosSettingsReconciler) requestMaintenanceOnServer(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (bool, error) {
	// if Server maintenance ref is already given. no further action required.
	if biosSettings.Spec.ServerMaintenanceRef != nil {
		return false, nil
	}

	serverMaintenance := &metalv1alpha1.ServerMaintenance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.ManagerNamespace,
			Name:      biosSettings.Name,
		}}

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
		serverMaintenance.Spec.Policy = biosSettings.Spec.ServerMaintenancePolicy
		serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
		serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
		if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
			serverMaintenance.Status.State = ""
		}
		return controllerutil.SetControllerReference(biosSettings, serverMaintenance, r.Client.Scheme())
	})
	if err != nil {
		return false, fmt.Errorf("failed to create or patch serverMaintenance: %w", err)
	}
	log.V(1).Info("Created serverMaintenance", "serverMaintenance", serverMaintenance.Name, "serverMaintenance label", serverMaintenance.Labels, "Operation", opResult)

	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	createdCondition, err := r.getCondition(acc, biosSettings.Status.Conditions, serverMaintenanceCreatedCondition)
	if err != nil {
		return false, err
	}
	if err := acc.Update(
		createdCondition,
		conditionutils.UpdateStatus(corev1.ConditionTrue),
		conditionutils.UpdateReason(createdServerMaintenanceReason),
		conditionutils.UpdateMessage(fmt.Sprintf("Created %v at %v", serverMaintenance.Name, time.Now())),
	); err != nil {
		return false, fmt.Errorf("failed to update creating serverMaintenance condition: %w", err)
	}

	err = r.patchMaintenanceRequestRefOnBiosSettings(ctx, log, biosSettings, serverMaintenance, createdCondition)
	if err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in biosSettings status: %w", err)
	}

	log.V(1).Info("Patched serverMaintenance on biosSettings")

	return true, nil
}

func (r *BiosSettingsReconciler) getCondition(
	acc *conditionutils.Accessor,
	conditions []metav1.Condition,
	conditionType string,
) (*metav1.Condition, error) {
	condition := &metav1.Condition{}
	condFound, err := acc.FindSlice(conditions, conditionType, condition)

	if err != nil {
		return nil, fmt.Errorf("failed to find Condition %v. error: %v", conditionType, err)
	}
	if !condFound {
		condition.Type = conditionType
		if err := acc.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
		); err != nil {
			return condition, fmt.Errorf("failed to create/update new Condition %v. error: %v", conditionType, err)
		}
	}

	return condition, nil
}

func (r *BiosSettingsReconciler) getReferredServer(
	ctx context.Context,
	log logr.Logger,
	serverRef *corev1.LocalObjectReference,
) (*metalv1alpha1.Server, error) {
	key := client.ObjectKey{Name: serverRef.Name}
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, key, server); err != nil {
		log.V(1).Error(err, "failed to get referred server")
		return server, err
	}
	return server, nil
}

func (r *BiosSettingsReconciler) getReferredServerMaintenance(
	ctx context.Context,
	log logr.Logger,
	serverMaintenanceRef *corev1.ObjectReference,
) (*metalv1alpha1.ServerMaintenance, error) {
	if serverMaintenanceRef == nil {
		return nil, fmt.Errorf("nil ServerMaintenance reference")
	}
	key := client.ObjectKey{Name: serverMaintenanceRef.Name, Namespace: r.ManagerNamespace}
	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	if err := r.Get(ctx, key, serverMaintenance); err != nil {
		log.V(1).Error(err, "failed to get referred serverMaintenance obj")
		return serverMaintenance, err
	}

	return serverMaintenance, nil
}

func (r *BiosSettingsReconciler) getReferredBIOSSettings(
	ctx context.Context,
	log logr.Logger,
	referredBIOSSetteingRef *corev1.LocalObjectReference,
) (*metalv1alpha1.BIOSSettings, error) {
	key := client.ObjectKey{Name: referredBIOSSetteingRef.Name, Namespace: metav1.NamespaceNone}
	biosSettings := &metalv1alpha1.BIOSSettings{}
	if err := r.Get(ctx, key, biosSettings); err != nil {
		log.V(1).Error(err, "failed to get referred BIOSSetting")
		return biosSettings, err
	}
	return biosSettings, nil
}

func (r *BiosSettingsReconciler) patchBiosSettingsRefOnServer(
	ctx context.Context,
	log logr.Logger,
	server *metalv1alpha1.Server,
	biosSettingsReference *corev1.LocalObjectReference,
) (err error) {
	if biosSettingsReference != nil && server.Spec.BIOSSettingsRef != nil && server.Spec.BIOSSettingsRef.Name == biosSettingsReference.Name {
		return nil
	}

	serverBase := server.DeepCopy()
	server.Spec.BIOSSettingsRef = biosSettingsReference
	if err = r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		log.V(1).Error(err, "failed to patch bios settings ref")
		return err
	}
	return err
}

func (r *BiosSettingsReconciler) patchMaintenanceRequestRefOnBiosSettings(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	serverMaintenance *metalv1alpha1.ServerMaintenance,
	condition *metav1.Condition,
) error {
	biosSettingsBase := biosSettings.DeepCopy()

	if serverMaintenance == nil {
		biosSettings.Spec.ServerMaintenanceRef = nil
	} else {
		biosSettings.Spec.ServerMaintenanceRef = &corev1.ObjectReference{
			APIVersion: metalv1alpha1.GroupVersion.String(),
			Kind:       "ServerMaintenance",
			Namespace:  serverMaintenance.Namespace,
			Name:       serverMaintenance.Name,
			UID:        serverMaintenance.UID,
		}
	}
	if condition != nil {
		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
		if err := acc.UpdateSlice(
			&biosSettings.Status.Conditions,
			condition.Type,
			conditionutils.UpdateStatus(condition.Status),
			conditionutils.UpdateReason(condition.Reason),
			conditionutils.UpdateMessage(condition.Message),
		); err != nil {
			return fmt.Errorf("failed to patch BIOSSettings condition: %w", err)
		}
	}

	if err := r.Patch(ctx, biosSettings, client.MergeFrom(biosSettingsBase)); err != nil {
		log.V(1).Error(err, "failed to patch serverMaintenanceRef onto BIOSsettings")
		return err
	}

	err := r.updateBiosSettingsStatus(ctx, log, biosSettings, biosSettings.Status.State, condition)

	return err
}

func (r *BiosSettingsReconciler) patchServerMaintenancePowerState(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	powerState metalv1alpha1.Power,
) error {
	serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, biosSettings.Spec.ServerMaintenanceRef)
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

	log.V(1).Info("Patched desired Power of the ServerMaintenance", "Server", serverMaintenance.Spec.ServerRef.Name, "state", powerState)
	return nil
}

func (r *BiosSettingsReconciler) updateBiosSettingsFlowStatus(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	state metalv1alpha1.BIOSSettingsFlowState,
	currentSettingsFlowStatus *metalv1alpha1.BIOSSettingsFlowStatus,
	condition *metav1.Condition,
) error {
	if currentSettingsFlowStatus == nil || (currentSettingsFlowStatus.State == state && condition == nil) {
		return nil
	}

	biosSettingsBase := biosSettings.DeepCopy()

	currentIdx := -1
	for idx, flowStatus := range biosSettings.Status.FlowState {
		if flowStatus.Priority == currentSettingsFlowStatus.Priority && flowStatus.Name == currentSettingsFlowStatus.Name {
			biosSettings.Status.FlowState[idx].State = state
			if state == metalv1alpha1.BIOSSettingsFlowStateApplied {
				time := metav1.Now()
				biosSettings.Status.FlowState[idx].LastAppliedTime = &time
			}
			if condition != nil {
				acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
				if err := acc.UpdateSlice(
					&biosSettings.Status.FlowState[idx].Conditions,
					condition.Type,
					conditionutils.UpdateStatus(condition.Status),
					conditionutils.UpdateReason(condition.Reason),
					conditionutils.UpdateMessage(condition.Message),
				); err != nil {
					return fmt.Errorf("failed to patch BIOSettings condition: %w", err)
				}
			} else {
				biosSettings.Status.FlowState[idx].Conditions = nil
			}
			currentIdx = idx
			continue
		} else if state == metalv1alpha1.BIOSSettingsFlowStateInProgress &&
			flowStatus.State == metalv1alpha1.BIOSSettingsFlowStateInProgress {
			// if current is InProgress, move all other settings state to Pending state.
			// This can happen when we suddenly detect settings change in actual BMC and have to start over the settings
			biosSettings.Status.FlowState[idx].State = metalv1alpha1.BIOSSettingsFlowStatePending
		}
	}

	if currentIdx == -1 {
		// if the currentFlowStatus is missing, add it.
		currentSettingsFlowStatus.State = state
		biosSettings.Status.FlowState = append(biosSettings.Status.FlowState, *currentSettingsFlowStatus)
		currentIdx = len(biosSettings.Status.FlowState) - 1
	}

	if err := r.Status().Patch(ctx, biosSettings, client.MergeFrom(biosSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSSettings FlowState status: %w", err)
	}

	log.V(1).Info("Updated biosSettings Flow state",
		"currentSettings Name", biosSettings.Status.FlowState[currentIdx].Name,
		"Settings Status", biosSettings.Status.FlowState[currentIdx])

	return nil
}

func (r *BiosSettingsReconciler) updateBiosSettingsStatus(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	state metalv1alpha1.BIOSSettingsState,
	condition *metav1.Condition,
) error {
	if biosSettings.Status.State == state && condition == nil {
		return nil
	}

	biosSettingsBase := biosSettings.DeepCopy()
	biosSettings.Status.State = state

	if state == metalv1alpha1.BIOSSettingsStateApplied {
		time := metav1.Now()
		biosSettings.Status.LastAppliedTime = &time
	} else if !biosSettings.Status.LastAppliedTime.IsZero() {
		biosSettings.Status.LastAppliedTime = &metav1.Time{}
	}

	if condition != nil {
		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
		if err := acc.UpdateSlice(
			&biosSettings.Status.Conditions,
			condition.Type,
			conditionutils.UpdateStatus(condition.Status),
			conditionutils.UpdateReason(condition.Reason),
			conditionutils.UpdateMessage(condition.Message),
		); err != nil {
			return fmt.Errorf("failed to patch BIOSettings condition: %w", err)
		}
	} else if state == metalv1alpha1.BIOSSettingsStatePending {
		// reset, when we restart the setting update
		biosSettings.Status.Conditions = nil
		biosSettings.Status.FlowState = []metalv1alpha1.BIOSSettingsFlowStatus{}
	}

	if err := r.Status().Patch(ctx, biosSettings, client.MergeFrom(biosSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSSettings status: %w", err)
	}

	log.V(1).Info("Updated biosSettings state ", "State", state, "Conditions", biosSettings.Status.Conditions)

	return nil
}

func (r *BiosSettingsReconciler) getCurrentSettingsFlowStatus(
	biosSettings *metalv1alpha1.BIOSSettings,
	currentSettings *metalv1alpha1.SettingsFlowItem,
) *metalv1alpha1.BIOSSettingsFlowStatus {

	if len(biosSettings.Status.FlowState) == 0 {
		return nil
	}

	for _, flowState := range biosSettings.Status.FlowState {
		if flowState.Priority == currentSettings.Priority &&
			currentSettings.Name == flowState.Name {
			return &flowState
		}
	}
	return nil
}

func (r *BiosSettingsReconciler) enqueueBiosSettingsByServerRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)

	// return early if hosts are not required states
	if host.Status.State == metalv1alpha1.ServerStateDiscovery ||
		host.Status.State == metalv1alpha1.ServerStateError ||
		host.Status.State == metalv1alpha1.ServerStateInitial {
		return nil
	}

	// no need to queue if the server is not yet in maintenance
	// hence return early
	if host.Spec.ServerMaintenanceRef == nil {
		return nil
	}

	BIOSSettingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := r.List(ctx, BIOSSettingsList); err != nil {
		log.V(1).Error(err, "failed to list biosSettings")
		return nil
	}

	for _, biosSettings := range BIOSSettingsList.Items {
		if biosSettings.Spec.ServerRef.Name == host.Name {
			// states where we do not want to requeue for host changes
			if biosSettings.Spec.ServerMaintenanceRef == nil ||
				biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateApplied ||
				biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateFailed {
				return nil
			}
			if biosSettings.Spec.ServerMaintenanceRef.Name != host.Spec.ServerMaintenanceRef.Name {
				return nil
			}
			return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: biosSettings.Namespace, Name: biosSettings.Name}}}
		}
	}
	return nil
}

func (r *BiosSettingsReconciler) enqueueBiosSettingsByBMC(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.BMC)

	serverList := &metalv1alpha1.ServerList{}
	if err := clientutils.ListAndFilter(ctx, r.Client, serverList, func(object client.Object) (bool, error) {
		server := object.(*metalv1alpha1.Server)
		return server.Spec.BMCRef != nil && server.Spec.BMCRef.Name == host.Name, nil
	}); err != nil {
		log.V(1).Error(err, "failed to list Server created by this BMC resources", "BMC", host.Name)
		return nil
	}

	reqs := make([]ctrl.Request, 0)
	for _, server := range serverList.Items {
		// skip if no bios settings ref for the server
		if server.Spec.BIOSSettingsRef == nil {
			continue
		}
		biosSettings := &metalv1alpha1.BIOSSettings{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: metav1.NamespaceNone, Name: server.Spec.BIOSSettingsRef.Name}, biosSettings); err != nil {
			log.V(1).Error(err, "failed to get biosSettings from server", "Server", server.Name, "BIOSSettingsRef", server.Spec.BIOSSettingsRef)
			continue
		}
		// only enqueue if bios settings is in progress state
		if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
			acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
			resetBMC, err := r.getCondition(acc, biosSettings.Status.Conditions, BMCConditionReset)
			if err != nil {
				log.V(1).Error(err, "failed to get reset BMC condition")
				continue
			}
			if resetBMC.Status == metav1.ConditionTrue {
				continue
			}
			// enqueue only if the BMC reset is requested for this BMC
			if resetBMC.Reason == BMCReasonReset {
				reqs = append(reqs, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: biosSettings.Namespace, Name: biosSettings.Name}})
			}
		}
	}
	return reqs
}

func (r *BiosSettingsReconciler) enqueueBiosSettingsByBiosVersionResource(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	BIOSVersion := obj.(*metalv1alpha1.BIOSVersion)
	if BIOSVersion.Status.State != metalv1alpha1.BIOSVersionStateCompleted {
		return nil
	}

	BIOSSettingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := r.List(ctx, BIOSSettingsList); err != nil {
		log.Error(err, "failed to list biosSettings")
		return nil
	}

	for _, biosSettings := range BIOSSettingsList.Items {
		if biosSettings.Spec.ServerRef.Name == BIOSVersion.Spec.ServerRef.Name {
			if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateApplied || biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateFailed {
				return nil
			}
			return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: biosSettings.Namespace, Name: biosSettings.Name}}}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BiosSettingsReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSSettings{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosSettingsByServerRefs)).
		Watches(&metalv1alpha1.BIOSVersion{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosSettingsByBiosVersionResource)).
		Watches(&metalv1alpha1.BMC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosSettingsByBMC)).
		Complete(r)
}
