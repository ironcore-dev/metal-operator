// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

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
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

// BiosSettingsReconciler reconciles a BIOSSettings object
type BiosSettingsReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.BMCOptions
}

const biosSettingsFinalizer = "firmware.ironcore.dev/biossettings"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
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
	if !biosSettings.DeletionTimestamp.IsZero() {
		log.V(1).Info("object is being deleted")
		return r.delete(ctx, log, biosSettings)
	}

	return r.reconcile(ctx, log, biosSettings)
}

func (r *BiosSettingsReconciler) delete(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(biosSettings, biosSettingsFinalizer) {
		return ctrl.Result{}, nil
	}
	if err := r.cleanupReferences(ctx, log, biosSettings); err != nil {
		log.Error(err, "failed to cleanup references")
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured references were cleaned up")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosSettings, biosSettingsFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("biosSettings is deleted")
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

	// if we got the server ref. by and its not being deleted
	if err == nil && serverMaintenance.DeletionTimestamp.IsZero() {
		// created by the controller
		if metav1.IsControlledBy(serverMaintenance, biosSettings) {
			// if the biosSettings is not being deleted, update the
			log.V(1).Info("Deleting server maintenance", "serverMaintenance Name", serverMaintenance.Name, "state", serverMaintenance.Status.State)
			if err := r.Delete(ctx, serverMaintenance); err != nil {
				return err
			}
		} else { // not created by controller
			log.V(1).Info("server maintenance status not updated as its provided by user", "serverMaintenance Name", serverMaintenance.Name, "state", serverMaintenance.Status.State)
		}
	}
	// if already deleted or we deleted it or its created by user, remove reference
	if apierrors.IsNotFound(err) || err == nil {
		err = r.patchMaintenanceRequestRefOnBiosSettings(ctx, log, biosSettings, nil)
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
			log.V(1).Info("referred Server is gone")
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

func (r *BiosSettingsReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(biosSettings) {
		log.V(1).Info("Skipped BIOS Setting reconciliation")
		return ctrl.Result{}, nil
	}

	// if object does not refer to server object - stop reconciliation
	if biosSettings.Spec.ServerRef == nil {
		log.V(1).Info("object does not refer to server object")
		return ctrl.Result{}, nil
	}

	// if referred server contains reference to different BIOSSettings object - stop reconciliation
	server, err := r.getReferredServer(ctx, log, biosSettings.Spec.ServerRef)
	if err != nil {
		log.V(1).Info("referred server object could not be fetched")
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
			log.V(1).Info("referred server contains reference to different BIOSSettings object, unable to fetch the referenced bios setting")
			return ctrl.Result{}, err
		}
		// check if the current BIOS setting version is newer and update reference if it is newer
		// todo : handle version checks correctly
		if referredBIOSSetting.Spec.BIOSSettings.Version < biosSettings.Spec.BIOSSettings.Version {
			log.V(1).Info("Updating BIOSSetting reference to the latest BIOS version")
			if err := r.patchBiosSettingsRefOnServer(ctx, log, server, &corev1.LocalObjectReference{Name: biosSettings.Name}); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, biosSettings, biosSettingsFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	return r.ensureServerMaintenanceStateTransition(ctx, log, biosSettings, server)
}

func (r *BiosSettingsReconciler) ensureServerMaintenanceStateTransition(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	switch biosSettings.Status.State {
	case "", metalv1alpha1.BIOSSettingsStatePending:
		pendingPresent, pendingSettings, err := r.checkforPendingSettingsOnBIOS(ctx, log, server, nil)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get pending settings on bios: %w", err)
		}
		if len(pendingSettings) > 0 || pendingPresent {
			log.V(1).Info("Pending bios setting tasks found", "biosSettings pending tasks", pendingSettings)
			err := r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStateFailed)
			return ctrl.Result{}, err
		}
		err = r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStateInProgress)
		return ctrl.Result{}, err
	case metalv1alpha1.BIOSSettingsStateInProgress:
		return r.handleSettingInProgressState(ctx, log, biosSettings, server)
	case metalv1alpha1.BIOSSettingsStateApplied:
		return r.handleSettingAppliedState(ctx, log, biosSettings, server)
	case metalv1alpha1.BIOSSettingsStateFailed:
		return r.handleFailedState(ctx, log, biosSettings, server)
	}
	log.V(1).Info("Unknown State found", "biosSettings state", biosSettings.Status.State)
	return ctrl.Result{}, nil
}

func (r *BiosSettingsReconciler) handleSettingInProgressState(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {

	currentBiosVersion, settingsDiff, err := r.getBIOSVersionAndSettingDifference(ctx, log, biosSettings, server)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings: %w", err)
	}
	// if setting is not different, complete the BIOS tasks, does not matter if the bios version do not match
	if len(settingsDiff) == 0 {
		// move status to completed
		err := r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStateApplied)
		return ctrl.Result{}, err
	}

	// todo:wait on the result from the resource which does upgrade to requeue.
	if currentBiosVersion != biosSettings.Spec.BIOSSettings.Version {
		log.V(1).Info("Pending BIOS version upgrade.", "current bios Version", currentBiosVersion, "required version", biosSettings.Spec.BIOSSettings.Version)
		return ctrl.Result{}, nil
	}

	if req, err := r.checkAndRequestMaintenance(ctx, log, biosSettings, server, settingsDiff); err != nil || req {
		return ctrl.Result{}, err
	}

	// check if the maintenance is granted
	if ok := r.checkIfMaintenanceGranted(log, biosSettings, server); !ok {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with updating settings")
		return ctrl.Result{}, nil
	}

	return r.applySettingUpdateStateTransition(ctx, log, biosSettings, server, settingsDiff)
}

func (r *BiosSettingsReconciler) checkAndRequestMaintenance(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
	settingsDiff redfish.SettingsAttributes,
) (bool, error) {
	// check if we need to request maintenance if we dont have it already
	// note: having this check will reduce the call made to BMC.
	if biosSettings.Spec.ServerMaintenanceRef == nil {
		bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
		if err != nil {
			return false, err
		}
		defer bmcClient.Logout()

		resetReq, err := bmcClient.CheckBiosAttributes(settingsDiff)
		if resetReq {
			// request maintenance if needed, even if err was reported.
			requeue, errMainReq := r.requestMaintenanceOnServer(ctx, log, biosSettings, server)
			return requeue, errors.Join(err, errMainReq)
		}
		if err != nil {
			return false, fmt.Errorf("failed to check BMC settings provided: %w", err)
		}
	}
	return false, nil
}

func (r *BiosSettingsReconciler) applySettingUpdateStateTransition(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
	settingsDiff redfish.SettingsAttributes,
) (ctrl.Result, error) {
	switch biosSettings.Status.UpdateSettingState {
	case "":
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.updateBIOSSettingUpdateStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingUpdateStateIssue)
			return ctrl.Result{}, err
		}
		// we need to request maintenance to get the server to power-On to apply the BIOS settings
		if biosSettings.Spec.ServerMaintenanceRef == nil {
			log.V(1).Info("server powered off, request maintenance to turn the server On")
			if requeue, err := r.requestMaintenanceOnServer(ctx, log, biosSettings, server); err != nil || requeue {
				return ctrl.Result{}, err
			}
		}

		err := r.patchServerMaintenancePowerState(ctx, log, biosSettings, metalv1alpha1.PowerOn)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to turn on server %w", err)
		}
		log.V(1).Info("Reconciled biosSettings at Pending state")
		return ctrl.Result{}, err
	case metalv1alpha1.BIOSSettingUpdateStateIssue:
		bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
		if err != nil {
			return ctrl.Result{}, err
		}
		defer bmcClient.Logout()

		// check if the pending tasks not present on the bios settings
		pendingPresent, _, err := r.checkforPendingSettingsOnBIOS(ctx, log, server, nil)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get pending BIOS settings: %w", err)
		}
		var pendingSettingsDiff redfish.SettingsAttributes
		if !pendingPresent {
			err = bmcClient.SetBiosAttributesOnReset(ctx, server.Spec.SystemUUID, settingsDiff)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to set BMC settings: %w", err)
			}
		}

		// get latest pending settings, and expect it to be zero different from the required settings.
		pendingPresent, pendingSettingsDiff, err = r.checkforPendingSettingsOnBIOS(ctx, log, server, settingsDiff)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get pending BIOS settings: %w", err)
		}

		// at this point the bios setting update needs to be already issued.
		if !pendingPresent {
			// todo: fail after X amount of time
			log.V(1).Info("bios Setting update issued to bmc not accepted. retrying....")
			return ctrl.Result{}, errors.Join(err, fmt.Errorf("bios setting issued to bmc not accepted"))
		}

		// latest pending settings to be zero different from the required settings.
		if len(pendingSettingsDiff) > 0 {
			log.V(1).Info("Unknown pending BIOS settings found", "Unknown pending settings", pendingSettingsDiff)
			err := r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStateFailed)
			return ctrl.Result{}, err
		}

		// if we dont need (have not requested maintenance) reboot. skip reboot steps.
		nextState := metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOff
		if biosSettings.Spec.ServerMaintenanceRef == nil {
			nextState = metalv1alpha1.BIOSSettingUpdateStateVerification
		}

		err = r.updateBIOSSettingUpdateStatus(ctx, log, biosSettings, nextState)
		log.V(1).Info("Reconciled biosSettings at update Settings state")
		return ctrl.Result{}, err
	case metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOff:
		// expected state it to be off and initial state is to be on.
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.patchServerMaintenancePowerState(ctx, log, biosSettings, metalv1alpha1.PowerOff)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOffPowerState) {
			err := r.updateBIOSSettingUpdateStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOn)
			return ctrl.Result{}, err
		}
		log.V(1).Info("Reconciled biosSettings at reboot wait for power off")
		return ctrl.Result{}, nil
	case metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOn:
		// expected power state it to be on and initial state is to be off.
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOffPowerState) {
			err := r.patchServerMaintenancePowerState(ctx, log, biosSettings, metalv1alpha1.PowerOn)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.updateBIOSSettingUpdateStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingUpdateStateVerification)
			return ctrl.Result{}, err
		}
		log.V(1).Info("Reconciled biosSettings at reboot wait for power on")
		return ctrl.Result{}, nil
	case metalv1alpha1.BIOSSettingUpdateStateVerification:
		// make sure the setting has actually applied.
		_, settingsDiff, err := r.getBIOSVersionAndSettingDifference(ctx, log, biosSettings, server)

		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings: %w", err)
		}
		// if setting is not different, complete the BIOS tasks
		if len(settingsDiff) == 0 {
			// move  biosSettings state to completed, and revert the settingUpdate state to initial
			err := r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStateApplied)
			return ctrl.Result{}, err
		}
		// todo: can take some time to setting to take place. might need to fail after certain time.
		log.V(1).Info("Reconciled biosSettings at wait for verification")
		return ctrl.Result{}, fmt.Errorf("waiting on the BIOS setting to take place")
	}
	log.V(1).Info("Unknown State found", "biosSettings UpdateSetting state", biosSettings.Status.UpdateSettingState)
	// stop reconsile as we can not proceed with unknown state
	return ctrl.Result{}, nil
}

func (r *BiosSettingsReconciler) handleSettingAppliedState(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	// clean up maintenance crd and references.
	if err := r.cleanupServerMaintenanceReferences(ctx, log, biosSettings); err != nil {
		return ctrl.Result{}, err
	}

	_, settingsDiff, err := r.getBIOSVersionAndSettingDifference(ctx, log, biosSettings, server)

	if err != nil {
		log.V(1).Error(err, "unable to fetch and check BIOSSettings")
		return ctrl.Result{}, err
	}
	if len(settingsDiff) > 0 {
		err := r.updateBiosSettingsStatus(ctx, log, biosSettings, metalv1alpha1.BIOSSettingsStatePending)
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
	log.V(1).Info("Handle failed setting update with no maintenance reference")
	// todo: revisit this logic to either create maintenance if not present, put server in Error state on failed bios settings maintenance
	log.V(1).Info("Failed to update bios setting", "ctx", ctx, "biosSettings", biosSettings, "server", server)
	return ctrl.Result{}, nil
}

func (r *BiosSettingsReconciler) checkforPendingSettingsOnBIOS(
	ctx context.Context,
	log logr.Logger,
	server *metalv1alpha1.Server,
	settingsDiff redfish.SettingsAttributes,
) (pendingSettingPresent bool, pendingSettingsDiff redfish.SettingsAttributes, err error) {
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		return pendingSettingPresent, pendingSettingsDiff, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	pendingSettingsDiff, err = bmcClient.GetBiosPendingAttributeValues(ctx, server.Spec.SystemUUID)
	if err != nil {
		return pendingSettingPresent, pendingSettingsDiff, err
	}
	pendingSettingPresent = len(pendingSettingsDiff) > 0

	// if settingsDiff is provided find the difference between settingsDiff and pending
	if len(settingsDiff) > 0 {
		log.V(1).Info("checking for the difference in the pending settings than that of required")
		if !pendingSettingPresent {
			return pendingSettingPresent, settingsDiff, nil
		}
		unknownpendingSettings := make(redfish.SettingsAttributes, len(settingsDiff))
		for name, value := range settingsDiff {
			if pendingValue, ok := pendingSettingsDiff[name]; ok && value != pendingValue {
				unknownpendingSettings[name] = pendingValue
			}
		}
		return pendingSettingPresent, unknownpendingSettings, nil
	}

	return pendingSettingPresent, pendingSettingsDiff, nil
}

func (r *BiosSettingsReconciler) getBIOSVersionAndSettingDifference(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	server *metalv1alpha1.Server,
) (currentbiosVersion string, diff redfish.SettingsAttributes, err error) {
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		return "", diff, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	keys := slices.Collect(maps.Keys(biosSettings.Spec.BIOSSettings.SettingsMap))

	currentSettings, err := bmcClient.GetBiosAttributeValues(ctx, server.Spec.SystemUUID, keys)
	if err != nil {
		log.V(1).Info("Failed to get with bios setting", "error", err)
		return "", diff, fmt.Errorf("failed to get BIOS settings: %w", err)
	}

	diff = redfish.SettingsAttributes{}
	var errs []error
	for key, value := range biosSettings.Spec.BIOSSettings.SettingsMap {
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

	if len(errs) > 0 {
		log.V(1).Info("Failed to get bios setting differences for some settings", "error", errs)
		return "", diff, fmt.Errorf("failed to find diff for some bios settings: %v", errs)
	}

	// fetch the current bios version from the server bmc
	currentBiosVersion, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemUUID)
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

	err = r.patchMaintenanceRequestRefOnBiosSettings(ctx, log, biosSettings, serverMaintenance)
	if err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in biosSettings status: %w", err)
	}

	log.V(1).Info("Patched serverMaintenance on biosSettings")

	return true, nil
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
	if server.Spec.BIOSSettingsRef == biosSettingsReference {
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
) error {
	biosSettingsBase := biosSettings.DeepCopy()

	if serverMaintenance == nil {
		biosSettings.Spec.ServerMaintenanceRef = nil
	} else {
		biosSettings.Spec.ServerMaintenanceRef = &corev1.ObjectReference{
			APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			Kind:       "ServerMaintenance",
			Namespace:  serverMaintenance.Namespace,
			Name:       serverMaintenance.Name,
			UID:        serverMaintenance.UID,
		}
	}

	if err := r.Patch(ctx, biosSettings, client.MergeFrom(biosSettingsBase)); err != nil {
		log.V(1).Error(err, "failed to patch bios settings ref")
		return err
	}

	return nil
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

func (r *BiosSettingsReconciler) updateBiosSettingsStatus(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	state metalv1alpha1.BIOSSettingsState,
) error {

	if biosSettings.Status.State == state {
		return nil
	}

	biosSettingsBase := biosSettings.DeepCopy()
	biosSettings.Status.State = state

	if state == metalv1alpha1.BIOSSettingsStateApplied {
		biosSettings.Status.UpdateSettingState = ""
	}

	if err := r.Status().Patch(ctx, biosSettings, client.MergeFrom(biosSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSSettings status: %w", err)
	}

	log.V(1).Info("Updated biosSettings state ", "new state", state)

	return nil
}

func (r *BiosSettingsReconciler) updateBIOSSettingUpdateStatus(
	ctx context.Context,
	log logr.Logger,
	biosSettings *metalv1alpha1.BIOSSettings,
	state metalv1alpha1.BIOSSettingUpdateState,
) error {

	if biosSettings.Status.UpdateSettingState == state {
		return nil
	}

	biosSettingsBase := biosSettings.DeepCopy()
	biosSettings.Status.UpdateSettingState = state

	if err := r.Status().Patch(ctx, biosSettings, client.MergeFrom(biosSettingsBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSSettings UpdateSetting status: %w", err)
	}

	log.V(1).Info("Updated biosSettings UpdateSetting state ", "new state", state)

	return nil
}

func (r *BiosSettingsReconciler) enqueueBiosSettingsByRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)
	BIOSSettingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := r.List(ctx, BIOSSettingsList); err != nil {
		log.Error(err, "failed to list biosSettings")
		return nil
	}
	var req []ctrl.Request

	for _, biosSettings := range BIOSSettingsList.Items {
		if biosSettings.Spec.ServerRef.Name == host.Name && biosSettings.Spec.ServerMaintenanceRef != nil {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: biosSettings.Namespace, Name: biosSettings.Name},
			})
		}
	}
	return req
}

// SetupWithManager sets up the controller with the Manager.
func (r *BiosSettingsReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSSettings{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosSettingsByRefs)).
		Complete(r)
}
