// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"

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
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/redfish"
)

// OOBMSettingsReconciler reconciles a OOBMSettings object
type OOBMSettingsReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.BMCOptions
}

const OoBMSettingFinalizer = "firmware.ironcore.dev/out-of-band-management"

const OoBMSettingCreatorLabel = "firmware.ironcore.dev/CreatedBy"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=oobmsettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=oobmsettings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=oobmsettings/finalizers,verbs=update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *OOBMSettingsReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	OoBMSetting := &metalv1alpha1.OOBMSettings{}
	if err := r.Get(ctx, req.NamespacedName, OoBMSetting); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling OoBM Settings")

	return r.reconcileExists(ctx, log, OoBMSetting)
}

// Determine whether reconciliation is required. It's not required if:
// - object is being deleted;
// - object does not contain reference to server;
// - object contains reference to server, but server references to another object with lower version;
func (r *OOBMSettingsReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if !OoBMSetting.DeletionTimestamp.IsZero() {
		log.V(1).Info("object is being deleted")
		return r.reconcileDeletion(ctx, log, OoBMSetting)
	}

	// if object does not refer to server object - stop reconciliation
	if OoBMSetting.Spec.ServerRef == nil {
		log.V(1).Info("object does not refer to server object")
		return ctrl.Result{}, nil
	}

	// if referred server contains reference to different OoBM object - stop reconciliation
	BMC, err := r.getOoBManager(ctx, log, OoBMSetting)
	if err != nil {
		log.V(1).Info("referred server object could not be fetched")
		return ctrl.Result{}, err
	}
	// patch server with OoBM reference
	if BMC.Spec.OoBMSettingRef == nil {
		if err := r.patchOoBMSettingRefOnBMC(ctx, log, BMC, &corev1.LocalObjectReference{Name: OoBMSetting.Name}); err != nil {
			return ctrl.Result{}, err
		}
	} else if BMC.Spec.OoBMSettingRef.Name != OoBMSetting.Name {
		referredOoBMSetting, err := r.getReferredOoBMSetting(ctx, log, BMC.Spec.OoBMSettingRef)
		if err != nil {
			log.V(1).Info("referred server contains reference to different OoBM object, unable to fetch the referenced OoBM setting")
			return ctrl.Result{}, err
		}
		// check if the current OoBM setting version is newer and update reference if it is newer
		// todo : handle version checks correctly
		if referredOoBMSetting.Spec.OOBMSettings.Version < OoBMSetting.Spec.OOBMSettings.Version {
			log.V(1).Info("Updating OoBM reference to the latest OoBM version")
			if err := r.patchOoBMSettingRefOnBMC(ctx, log, BMC, &corev1.LocalObjectReference{Name: OoBMSetting.Name}); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return r.reconcile(ctx, log, OoBMSetting, BMC)
}

func (r *OOBMSettingsReconciler) reconcileDeletion(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(OoBMSetting, OoBMSettingFinalizer) {
		return ctrl.Result{}, nil
	}
	if err := r.cleanupReferences(ctx, log, OoBMSetting); err != nil {
		log.Error(err, "failed to cleanup references")
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured references were cleaned up")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, OoBMSetting, OoBMSettingFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("OoBMSetting is deleted")
	return ctrl.Result{}, nil
}

func (r *OOBMSettingsReconciler) cleanupServerMaintenanceReferences(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	cleanUpmaintenanceRef bool,
) error {
	if OoBMSetting.Spec.ServerMaintenanceRef == nil {
		return nil
	}
	// try to get the serverMaintaince created
	serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, OoBMSetting.Spec.ServerMaintenanceRef)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get referred serverMaintenance obj from OoBMSetting: %w", err)
	}
	// if we got the server ref. by and its not being deleted
	if err == nil && serverMaintenance.DeletionTimestamp.IsZero() {
		// if the serverMaintenace is not created by serverBIOS Controller, dont delete.
		labelsOnMaintenance := serverMaintenance.GetLabels()

		// created by the controller
		if key, ok := labelsOnMaintenance[OoBMSettingCreatorLabel]; ok && key == OoBMSetting.Name {
			// if the serverBIOS is not being deleted, update the
			log.V(1).Info("Deleting server maintenance", "serverMaintenance Name", serverMaintenance.Name, "state", serverMaintenance.Status.State)
			if err := r.Delete(ctx, serverMaintenance); err != nil {
				return err
			}
		} else { // not created by controller
			log.V(1).Info("server maintenance status not updated as its provided by user", "serverMaintenance Name", serverMaintenance.Name, "state", serverMaintenance.Status.State)
		}
	}
	// if already deleted or have req mark as completed it remove reference
	if (apierrors.IsNotFound(err) || err == nil) && cleanUpmaintenanceRef {
		err = r.patchMaintenanceRequestRefOnOoBMSetting(ctx, log, OoBMSetting, nil)
		if err != nil {
			return fmt.Errorf("failed to clean up serverMaintenance ref in OoBMSetting status: %w", err)
		}
	}
	return nil
}

func (r *OOBMSettingsReconciler) cleanupReferences(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
) (err error) {
	if err := r.cleanupServerMaintenanceReferences(ctx, log, OoBMSetting, true); err != nil {
		return err
	}

	if OoBMSetting.Spec.ServerRef != nil {
		BMC, err := r.getOoBManager(ctx, log, OoBMSetting)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		// if we can not find the server, nothing else to clean up
		if apierrors.IsNotFound(err) {
			return nil
		}
		// if we have found the server, check if ref is this OoBMSetting and remove it
		if err == nil {
			if BMC.Spec.OoBMSettingRef != nil {
				if BMC.Spec.OoBMSettingRef.Name != OoBMSetting.Name {
					return nil
				}
				return r.patchOoBMSettingRefOnBMC(ctx, log, BMC, nil)
			} else {
				// nothing else to clean up
				return nil
			}
		}
	}

	return err
}

func (r *OOBMSettingsReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	BMC *metalv1alpha1.BMC,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(OoBMSetting) {
		log.V(1).Info("Skipped OoBM Setting reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, OoBMSetting, OoBMSettingFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	return r.ensureServerMaintenanceStateTransition(ctx, log, OoBMSetting, BMC)
}

func (r *OOBMSettingsReconciler) ensureServerMaintenanceStateTransition(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	BMC *metalv1alpha1.BMC,
) (ctrl.Result, error) {
	switch OoBMSetting.Status.State {
	case "":
		//todo: check that in initial state there is no pending OoBM maintenance left behind,

		// move to upgrade to check if version matches
		err := r.updateOoBMSettingStatus(ctx, log, OoBMSetting, metalv1alpha1.OoBMMaintenanceStateInVersionUpgrade)
		return ctrl.Result{}, err
	case metalv1alpha1.OoBMMaintenanceStateInVersionUpgrade:
		return r.handleVersionUpgradeState(ctx, log, OoBMSetting, BMC)
	case metalv1alpha1.OoBMMaintenanceStateInSettingUpdate:
		return r.handleSettingUpdateState(ctx, log, OoBMSetting, BMC)
	case metalv1alpha1.OoBMMaintenanceStateSynced:
		return r.handleSettingSyncedState(ctx, log, OoBMSetting, BMC)
	case metalv1alpha1.OoBMMaintenanceStateFailed:
		return r.handleFailedState(ctx, log, OoBMSetting, BMC)
	}
	log.V(1).Info("Unknown State found", "OoBMSetting state", OoBMSetting.Status.State)
	return ctrl.Result{}, nil
}

func (r *OOBMSettingsReconciler) handleVersionUpgradeState(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	BMC *metalv1alpha1.BMC,
) (ctrl.Result, error) {

	//check if the version match
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, BMC, r.Insecure, r.BMCOptions)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	// fetch the current OoBM version from the server bmc
	// oombtodo
	currentOoBMVersion, err := bmcClient.GetBMCVersion(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to load OoBM version: %w for BMC %v", err, BMC.Name)
	}

	// todo: handle version check correctly to detect and upgrade only to higher version
	if currentOoBMVersion == OoBMSetting.Spec.OOBMSettings.Version {
		// move status to inMaintenance to check if settings needs to be upgraded
		log.V(1).Info("OoBM version matches")
		err := r.updateOoBMSettingStatus(ctx, log, OoBMSetting, metalv1alpha1.OoBMMaintenanceStateInSettingUpdate)
		return ctrl.Result{}, err
	} else if currentOoBMVersion > OoBMSetting.Spec.OOBMSettings.Version {
		log.V(1).Info("OoBM downgrade is not supported", "currentOoBMVersion", currentOoBMVersion, "requested", OoBMSetting.Spec.OOBMSettings.Version)
		err := r.updateOoBMSettingStatus(ctx, log, OoBMSetting, metalv1alpha1.OoBMMaintenanceStateFailed)
		return ctrl.Result{}, err
	}
	log.V(1).Info("OoBM version needs upgrade", "current", currentOoBMVersion, "required", OoBMSetting.Spec.OOBMSettings.Version)

	// OoBM upgrade always need BMC reboot, Hence need maintenance request.
	if requeue, err := r.requestMaintenanceOnServer(ctx, log, OoBMSetting); err != nil || requeue {
		return ctrl.Result{}, err
	}

	// wait for maintenance request to be granted
	if ok := r.checkIfMaintenanceGranted(ctx, log, OoBMSetting); !ok {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with version upgrade")
		return ctrl.Result{}, err
	}

	// todo: do actual upgrade here.
	//time.Sleep(1 * time.Second)
	log.V(1).Info("upgraded OOBM version")

	// reque to check version and move to setting update
	return ctrl.Result{Requeue: true}, err
}

func (r *OOBMSettingsReconciler) handleSettingUpdateState(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	BMC *metalv1alpha1.BMC,
) (ctrl.Result, error) {

	_, settingsDiff, err := r.getOoBMSettingDifference(ctx, log, OoBMSetting, BMC)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get OoBM settings: %w", err)
	}
	// if setting is not different, complete the OoBMSettings tasks
	if len(settingsDiff) == 0 {
		// move status to completed
		err := r.updateOoBMSettingStatus(ctx, log, OoBMSetting, metalv1alpha1.OoBMMaintenanceStateSynced)
		return ctrl.Result{}, err
	}

	if req, err := r.checkAndRequestMaintenance(ctx, log, OoBMSetting, BMC, &settingsDiff); err != nil || req {
		return ctrl.Result{}, err
	}

	// check if the maintenance is granted
	if ok := r.checkIfMaintenanceGranted(ctx, log, OoBMSetting); !ok {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with updating settings", "reason", err)
		return ctrl.Result{}, err
	}

	return r.applySettingUpdateStateTransition(ctx, log, OoBMSetting, BMC, &settingsDiff)
}

func (r *OOBMSettingsReconciler) checkAndRequestMaintenance(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	BMC *metalv1alpha1.BMC,
	settingsDiff *redfish.SettingsAttributes,
) (bool, error) {
	// check if we need to request maintenance if we dont have it already
	// note: having this check will reduce the call made to BMC.
	if OoBMSetting.Spec.ServerMaintenanceRef == nil {
		bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, BMC, r.Insecure, r.BMCOptions)
		if err != nil {
			return false, err
		}
		defer bmcClient.Logout()
		resetReq, err := bmcClient.CheckBMCAttributes(*settingsDiff)
		if resetReq {
			// request maintenance if needed, even if err was reported.
			requeue, errMainReq := r.requestMaintenanceOnServer(ctx, log, OoBMSetting)
			return requeue, errors.Join(err, errMainReq)
		}
		if err != nil {
			return false, fmt.Errorf("failed to check BMC settings provided: %w", err)
		}
	}
	return false, nil
}

func (r *OOBMSettingsReconciler) applySettingUpdateStateTransition(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	BMC *metalv1alpha1.BMC,
	settingsDiff *redfish.SettingsAttributes,
) (ctrl.Result, error) {
	switch OoBMSetting.Status.UpdateSettingState {
	case "":
		if r.checkForRequiredPowerStatus(BMC, metalv1alpha1.OnPowerState) {
			err := r.updateOoBMSettingUpdateStatus(ctx, log, OoBMSetting, metalv1alpha1.OoBMSettingUpdateStateIssue)
			return ctrl.Result{}, err
		}
		err := r.patchServerManagerMaintenancePowerState(ctx, log, OoBMSetting, metalv1alpha1.PowerOn)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to turn on server %w", err)
		}
		return ctrl.Result{}, err
	case metalv1alpha1.OoBMSettingUpdateStateIssue:
		// todo: make it idepotent
		bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, BMC, r.Insecure, r.BMCOptions)
		if err != nil {
			return ctrl.Result{}, err
		}
		defer bmcClient.Logout()

		err = bmcClient.SetBMCAttributesImediately(ctx, *settingsDiff)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set BMC settings: %w", err)
		}
		nextState := metalv1alpha1.OoBMSettingUpdateWaitOnServerRebootPowerOff
		if OoBMSetting.Spec.ServerMaintenanceRef == nil {
			nextState = metalv1alpha1.OoBMSettingUpdateStateVerification
		}
		err = r.updateOoBMSettingUpdateStatus(ctx, log, OoBMSetting, nextState)
		return ctrl.Result{}, err
	case metalv1alpha1.OoBMSettingUpdateWaitOnServerRebootPowerOff:
		// expected state it to be off and initial state is to be on.
		// todo: check that the server OoBM setting is actually been issued.
		if r.checkForRequiredPowerStatus(BMC, metalv1alpha1.OnPowerState) {
			err := r.patchServerManagerMaintenancePowerState(ctx, log, OoBMSetting, metalv1alpha1.PowerOff)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(BMC, metalv1alpha1.OffPowerState) {
			err := r.updateOoBMSettingUpdateStatus(ctx, log, OoBMSetting, metalv1alpha1.OoBMSettingUpdateWaitOnServerRebootPowerOn)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	case metalv1alpha1.OoBMSettingUpdateWaitOnServerRebootPowerOn:
		// expected power state it to be on and initial state is to be off.
		// todo: check that the server OoBM setting is actually been issued.
		if r.checkForRequiredPowerStatus(BMC, metalv1alpha1.OffPowerState) {
			err := r.patchServerManagerMaintenancePowerState(ctx, log, OoBMSetting, metalv1alpha1.PowerOn)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(BMC, metalv1alpha1.OnPowerState) {
			err := r.updateOoBMSettingUpdateStatus(ctx, log, OoBMSetting, metalv1alpha1.OoBMSettingUpdateStateVerification)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	case metalv1alpha1.OoBMSettingUpdateStateVerification:
		// make sure the setting has actually applied.
		_, settingsDiff, err := r.getOoBMSettingDifference(ctx, log, OoBMSetting, BMC)

		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get OoBM settings: %w", err)
		}
		// if setting is not different, complete the OoBM settings tasks
		if len(settingsDiff) == 0 {
			// move  OoBMSetting state to completed, and revert the settingUpdate state to initial
			err := r.updateOoBMSettingStatus(ctx, log, OoBMSetting, metalv1alpha1.OoBMMaintenanceStateSynced)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, fmt.Errorf("waiting on the OoBM setting to take place")
	}
	log.V(1).Info("Unknown State found", "OoBMSetting UpdateSetting state", OoBMSetting.Status.UpdateSettingState)
	// stop reconsile as we can not proceed with unknown state
	return ctrl.Result{}, nil
}

func (r *OOBMSettingsReconciler) handleSettingSyncedState(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	BMC *metalv1alpha1.BMC,
) (ctrl.Result, error) {
	// clean up maintenance crd and references.
	if err := r.cleanupServerMaintenanceReferences(ctx, log, OoBMSetting, true); err != nil {
		return ctrl.Result{}, err
	}

	diffPresent, settingsDiff, err := r.getOoBMSettingDifference(ctx, log, OoBMSetting, BMC)

	if err != nil {
		log.V(1).Error(err, "unable to fetch and check OoBMSettings")
		return ctrl.Result{}, err
	}
	if diffPresent || len(settingsDiff) > 0 {
		err := r.updateOoBMSettingStatus(ctx, log, OoBMSetting, "")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Done with OoBM setting update", "ctx", ctx, "OoBMSetting", OoBMSetting, "OoBM bmc", BMC)
	return ctrl.Result{}, nil
}

func (r *OOBMSettingsReconciler) handleFailedState(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	BMC *metalv1alpha1.BMC,
) (ctrl.Result, error) {

	if OoBMSetting.Spec.ServerMaintenanceRef != nil {
		serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, OoBMSetting.Spec.ServerMaintenanceRef)
		if err != nil {
			log.V(1).Error(err, "unable to fetch serverMaintenance")
			return ctrl.Result{}, err
		}

		serverMaintenanceBase := serverMaintenance.DeepCopy()
		serverMaintenance.Status.State = metalv1alpha1.ServerMaintenanceStateCompleted
		if err := r.Status().Patch(ctx, serverMaintenance, client.MergeFrom(serverMaintenanceBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch serverMaintenance status to failed: %w", err)
		}
	} else {
		log.V(1).Info("Handle failed setting update with no maintenance reference")
		// todo: revist this logic to either create maintenance or not, put server in Error state on failed OoBM settings maintenance
		//			this would need update on servermaintenanceContoller to go handle this usecase.

		// request maintenance if needed,
		// if OoBMSetting.Spec.ServerMaintenanceRef == nil {
		// 	if err := r.requestMaintenanceOnServer(ctx, log, OoBMSetting, server); err != nil {
		// 		return ctrl.Result{}, err
		// 	}
		// }
		// move maintenance to failed state directly.
	}

	log.V(1).Info("Failed to update OoBM setting", "ctx", ctx, "OoBMSetting", OoBMSetting, "BMC", BMC)
	return ctrl.Result{}, nil
}

func (r *OOBMSettingsReconciler) getOoBMSettingDifference(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	BMC *metalv1alpha1.BMC,
) (updatedNeeded bool, diff redfish.SettingsAttributes, err error) {
	// todo: need to also account for future pending changes reported for OoBM
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, BMC, r.Insecure, r.BMCOptions)
	if err != nil {
		return false, diff, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	keys := make([]string, 0, len(OoBMSetting.Spec.OOBMSettings.Settings))
	for k := range OoBMSetting.Spec.OOBMSettings.Settings {
		keys = append(keys, k)
	}

	currentSettings, err := bmcClient.GetBMCAttributeValues(ctx, keys)
	if err != nil {
		log.V(1).Info("Failed to get with OoBM setting", "error", err)
		return false, diff, fmt.Errorf("failed to get OoBM settings: %w", err)
	}

	diff = redfish.SettingsAttributes{}
	var errs []error
	for key, value := range OoBMSetting.Spec.OOBMSettings.Settings {
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
		return false, diff, fmt.Errorf("failed to find diff for some OoBM settings: %v", errs)
	}

	log.V(1).Info("TEMP OoBM setting", "current", currentSettings, "diff", diff, "required", OoBMSetting.Spec.OOBMSettings.Settings)

	// fetch the current OoBM version from the server bmc
	currentOoBMVersion, err := bmcClient.GetBMCVersion(ctx)
	if err != nil {
		return false, diff, fmt.Errorf("failed to load OoBM version: %w for BMC %v", err, BMC.Name)
	}

	// todo: handle version check to detect and upgrade only higher version
	if currentOoBMVersion != OoBMSetting.Spec.OOBMSettings.Version {
		return true, diff, nil
	}

	return false, diff, nil
}

func (r *OOBMSettingsReconciler) checkForRequiredPowerStatus(
	BMC *metalv1alpha1.BMC,
	powerState metalv1alpha1.BMCPowerState,
) bool {
	return BMC.Status.PowerState == powerState
}

func (r *OOBMSettingsReconciler) checkIfMaintenanceGranted(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
) bool {

	if OoBMSetting.Spec.ServerMaintenanceRef == nil {
		return true
	}

	// todo: handle multiple servers related to this BMC
	// like: server, err := r.getRelatedServer(ctx, log, OoBMSetting)
	server, err := r.getReferredServer(ctx, log, OoBMSetting.Spec.ServerRef)

	if err != nil {
		log.V(1).Error(err, "Failed to get ref. server to determine maintenance state ")
		return false
	}

	if server.Status.State == metalv1alpha1.ServerStateMaintenance {
		if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.UID != OoBMSetting.Spec.ServerMaintenanceRef.UID {
			// server in maintenance for other tasks. or
			// server maintenance ref is wrong in either server or OoBMSetting
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

func (r *OOBMSettingsReconciler) requestMaintenanceOnServer(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
) (bool, error) {

	// if Server maintenance ref is already given. no further action required.
	if OoBMSetting.Spec.ServerMaintenanceRef != nil || OoBMSetting.Spec.ServerMaintenancePolicy == metalv1alpha1.ServerMaintenancePolicyEnforced {
		return false, nil
	}
	// todo: handle multiple servers related to this BMC, by creating a ServerMaintenanceSet
	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	serverMaintenance.Name = OoBMSetting.Name
	serverMaintenance.Namespace = r.ManagerNamespace

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
		serverMaintenance.SetLabels(map[string]string{OoBMSettingCreatorLabel: OoBMSetting.Name})
		serverMaintenance.Spec.Policy = OoBMSetting.Spec.ServerMaintenancePolicy
		serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
		serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: OoBMSetting.Spec.ServerRef.Name}
		if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
			serverMaintenance.Status.State = ""
		}
		return controllerutil.SetControllerReference(OoBMSetting, serverMaintenance, r.Client.Scheme())
	})
	if err != nil {
		return false, fmt.Errorf("failed to create or patch serverMaintenance: %w", err)
	}
	log.V(1).Info("Created serverMaintenance", "serverMaintenance", serverMaintenance.Name, "serverMaintenance label", serverMaintenance.Labels, "Operation", opResult)

	err = r.patchMaintenanceRequestRefOnOoBMSetting(ctx, log, OoBMSetting, serverMaintenance)
	if err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in OoBMSetting status: %w", err)
	}

	log.V(1).Info("Patched serverMaintenance on OoBMSetting")

	return true, nil
}

func (r *OOBMSettingsReconciler) getOoBManager(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
) (*metalv1alpha1.BMC, error) {

	var refName string
	if OoBMSetting.Spec.BMCRef == nil {
		server, err := r.getReferredServer(ctx, log, OoBMSetting.Spec.ServerRef)
		if err != nil {
			log.V(1).Error(err, "failed to get referred server")
			return nil, err
		}
		if server.Spec.BMCRef != nil {
			refName = server.Spec.BMCRef.Name
		} else {
			return nil, fmt.Errorf("no bmc is referred by the server")
		}
	} else {
		refName = OoBMSetting.Spec.BMCRef.Name
	}

	key := client.ObjectKey{Name: refName}
	BMC := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, key, BMC); err != nil {
		log.V(1).Error(err, "failed to get referred server's Manager")
		return BMC, err
	}

	return BMC, nil
}

// func (r *OOBMSettingsReconciler) getRelatedServer(
// 	ctx context.Context,
// 	log logr.Logger,
// 	OoBMSetting *metalv1alpha1.OOBMSettings,
// ) ([]*metalv1alpha1.Server, error) {
// 	if OoBMSetting.Spec.ServerRef == nil {
// 		BMC, err := r.getOoBManager(ctx, log, OoBMSetting)
// 		if err != nil {
// 			log.V(1).Error(err, "failed to get referred BMC")
// 			return nil, err
// 		}
// 		servers, err := bmcClient.GetSystems(ctx)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to get servers from BMC %s: %w", bmcObj.Name, err)
// 		}
// 		var result []*metalv1alpha1.Server
// 		var errs []error
// 		for i, s := range servers {
// 			currServer, err := r.getReferredServer(ctx, log, &corev1.LocalObjectReference{Name: bmcutils.GetServerNameFromBMCandIndex(i, BMC)})
// 			if err != nil {
// 				errs = append(errs, fmt.Errorf("failed to get server %s: %w", s.SerialNumber, err))
// 				continue
// 			}
// 			result = append(result, currServer)

// 		}
// 		if len(errs) > 0 {
// 			return result, fmt.Errorf("errors occurred during fetching related servers: %v", errs)
// 		}
// 		return result, nil
// 	}

// 	server, err := r.getReferredServer(ctx, log, OoBMSetting.Spec.ServerRef)
// 	if err != nil {
// 		log.V(1).Error(err, "failed to get referred Server")
// 		return nil, err
// 	}
// 	return []*metalv1alpha1.Server{server}, nil
// }

func (r *OOBMSettingsReconciler) getReferredServer(
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

func (r *OOBMSettingsReconciler) getReferredServerMaintenance(
	ctx context.Context,
	log logr.Logger,
	serverMaintenanceRef *corev1.ObjectReference,
) (*metalv1alpha1.ServerMaintenance, error) {
	key := client.ObjectKey{Name: serverMaintenanceRef.Name, Namespace: r.ManagerNamespace}
	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	if err := r.Get(ctx, key, serverMaintenance); err != nil {
		log.V(1).Error(err, "failed to get referred serverMaintenance obj")
		return serverMaintenance, err
	}

	return serverMaintenance, nil
}

func (r *OOBMSettingsReconciler) getReferredOoBMSetting(
	ctx context.Context,
	log logr.Logger,
	referredOoBMSetteingRef *corev1.LocalObjectReference,
) (*metalv1alpha1.OOBMSettings, error) {
	key := client.ObjectKey{Name: referredOoBMSetteingRef.Name, Namespace: metav1.NamespaceNone}
	OoBMSetting := &metalv1alpha1.OOBMSettings{}
	if err := r.Get(ctx, key, OoBMSetting); err != nil {
		log.V(1).Error(err, "failed to get referred OoBMSetting")
		return OoBMSetting, err
	}
	return OoBMSetting, nil
}

func (r *OOBMSettingsReconciler) patchOoBMSettingRefOnBMC(
	ctx context.Context,
	log logr.Logger,
	BMC *metalv1alpha1.BMC,
	OoBMSettingReference *corev1.LocalObjectReference,
) error {
	if BMC.Spec.OoBMSettingRef == OoBMSettingReference {
		return nil
	}

	var err error
	BMCBase := BMC.DeepCopy()
	BMC.Spec.OoBMSettingRef = OoBMSettingReference
	if err = r.Patch(ctx, BMC, client.MergeFrom(BMCBase)); err != nil {
		log.V(1).Error(err, "failed to patch OoBM settings ref")
		return err
	}
	return err
}

func (r *OOBMSettingsReconciler) patchMaintenanceRequestRefOnOoBMSetting(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	serverMaintenance *metalv1alpha1.ServerMaintenance,
) error {
	OoBMSettingBase := OoBMSetting.DeepCopy()

	if serverMaintenance == nil {
		OoBMSetting.Spec.ServerMaintenanceRef = nil
	} else {
		OoBMSetting.Spec.ServerMaintenanceRef = &corev1.ObjectReference{
			APIVersion: "metal.ironcore.dev/v1alpha1",
			Kind:       "ServerMaintenance",
			Namespace:  serverMaintenance.Namespace,
			Name:       serverMaintenance.Name,
			UID:        serverMaintenance.UID,
		}
	}

	if err := r.Patch(ctx, OoBMSetting, client.MergeFrom(OoBMSettingBase)); err != nil {
		log.V(1).Error(err, "failed to patch OoBM settings ref")
		return err
	}

	return nil
}

func (r *OOBMSettingsReconciler) patchServerManagerMaintenancePowerState(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	powerState metalv1alpha1.Power,
) (err error) {
	log.V(1).Info("todo patch OoBM settings maintenance BMC power state", "powerState", powerState, "OoBMSetting", OoBMSetting, "ctx", ctx)
	// todo: need to reboot the BMC not the server itself.
	if powerState == "" {
		return fmt.Errorf("wrong power state")
	}
	return err
}

func (r *OOBMSettingsReconciler) updateOoBMSettingStatus(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	state metalv1alpha1.OoBMMaintenanceState,
) error {

	if OoBMSetting.Status.State == state {
		return nil
	}

	OoBMSettingBase := OoBMSetting.DeepCopy()
	OoBMSetting.Status.State = state

	if state == metalv1alpha1.OoBMMaintenanceStateSynced {
		OoBMSetting.Status.UpdateSettingState = ""
	}

	if err := r.Status().Patch(ctx, OoBMSetting, client.MergeFrom(OoBMSettingBase)); err != nil {
		return fmt.Errorf("failed to patch OoBMSetting status: %w", err)
	}

	log.V(1).Info("Updated OoBMSetting state ", "new state", state)

	return nil
}

func (r *OOBMSettingsReconciler) updateOoBMSettingUpdateStatus(
	ctx context.Context,
	log logr.Logger,
	OoBMSetting *metalv1alpha1.OOBMSettings,
	state metalv1alpha1.OoBMSettingUpdateState,
) error {

	if OoBMSetting.Status.UpdateSettingState == state {
		return nil
	}

	OoBMSettingBase := OoBMSetting.DeepCopy()
	OoBMSetting.Status.UpdateSettingState = state

	if err := r.Status().Patch(ctx, OoBMSetting, client.MergeFrom(OoBMSettingBase)); err != nil {
		return fmt.Errorf("failed to patch OoBMSetting UpdateSetting status: %w", err)
	}

	log.V(1).Info("Updated OoBMSetting UpdateSetting state ", "new state", state)

	return nil
}

func (r *OOBMSettingsReconciler) enqueueOoBMSettingByServerRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)
	OoBMSettingList := &metalv1alpha1.OOBMSettingsList{}
	if err := r.List(ctx, OoBMSettingList); err != nil {
		log.Error(err, "failed to list OoBMSettinges")
		return nil
	}
	var req []ctrl.Request

	for _, OoBMSetting := range OoBMSettingList.Items {
		if OoBMSetting.Spec.ServerRef != nil && OoBMSetting.Spec.ServerRef.Name == host.Name && OoBMSetting.Spec.ServerMaintenanceRef != nil {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: OoBMSetting.Namespace, Name: OoBMSetting.Name},
			})
		}
	}
	return req
}

func (r *OOBMSettingsReconciler) enqueueOoBMSettingByBMCRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {

	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.BMC)
	OoBMSettingList := &metalv1alpha1.OOBMSettingsList{}
	if err := r.List(ctx, OoBMSettingList); err != nil {
		log.Error(err, "failed to list OoBMSettinges")
		return nil
	}
	var req []ctrl.Request

	for _, OoBMSetting := range OoBMSettingList.Items {
		if OoBMSetting.Spec.BMCRef != nil && OoBMSetting.Spec.BMCRef.Name == host.Name {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: OoBMSetting.Namespace, Name: OoBMSetting.Name},
			})
		}
	}
	return req
}

// SetupWithManager sets up the controller with the Manager.
func (r *OOBMSettingsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.OOBMSettings{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueOoBMSettingByServerRefs)).
		Watches(&metalv1alpha1.BMC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueOoBMSettingByBMCRefs)).
		Complete(r)
}
