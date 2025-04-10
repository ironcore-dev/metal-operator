// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/ironcore-dev/metal-operator/bmc"
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

// BiosMaintenanceReconciler reconciles a BiosMaintenance object
type BiosMaintenanceReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.BMCOptions
}

const biosMaintenanceFinalizer = "firmware.ironcore.dev/biosmaintenance"

const biosMaintenanceCreatorLabel = "firmware.ironcore.dev/created-by"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosmaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosmaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosmaintenances/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BiosMaintenanceReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	biosMaintenance := &metalv1alpha1.BiosMaintenance{}
	if err := r.Get(ctx, req.NamespacedName, biosMaintenance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling biosMaintenance")

	return r.reconcileExists(ctx, log, biosMaintenance)
}

// Determine whether reconciliation is required. It's not required if:
// - object is being deleted;
// - object does not contain reference to server;
// - object contains reference to server, but server references to another object with lower version;
func (r *BiosMaintenanceReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if !biosMaintenance.DeletionTimestamp.IsZero() {
		log.V(1).Info("object is being deleted")
		return r.reconcileDeletion(ctx, log, biosMaintenance)
	}

	// if object does not refer to server object - stop reconciliation
	if biosMaintenance.Spec.ServerRef == nil {
		log.V(1).Info("object does not refer to server object")
		return ctrl.Result{}, nil
	}

	// if referred server contains reference to different BiosMaintenance object - stop reconciliation
	server, err := r.getReferredServer(ctx, log, biosMaintenance.Spec.ServerRef)
	if err != nil {
		log.V(1).Info("referred server object could not be fetched")
		return ctrl.Result{}, err
	}
	// patch server with biosmaintenance reference
	if server.Spec.BIOSSettingsRef == nil {
		if err := r.patchBiosMaintenanceRefOnServer(ctx, log, server, &corev1.LocalObjectReference{Name: biosMaintenance.Name}); err != nil {
			return ctrl.Result{}, err
		}
	} else if server.Spec.BIOSSettingsRef.Name != biosMaintenance.Name {
		referredBIOSSetting, err := r.getReferredbiosMaintenance(ctx, log, server.Spec.BIOSSettingsRef)
		if err != nil {
			log.V(1).Info("referred server contains reference to different BiosMaintenance object, unable to fetch the referenced bios setting")
			return ctrl.Result{}, err
		}
		// check if the current BIOS setting version is newer and update reference if it is newer
		// todo : handle version checks correctly
		if referredBIOSSetting.Spec.BiosSettings.Version < biosMaintenance.Spec.BiosSettings.Version {
			log.V(1).Info("Updating BIOSSetting reference to the latest BIOS version")
			if err := r.patchBiosMaintenanceRefOnServer(ctx, log, server, &corev1.LocalObjectReference{Name: biosMaintenance.Name}); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return r.reconcile(ctx, log, biosMaintenance, server)
}

func (r *BiosMaintenanceReconciler) reconcileDeletion(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(biosMaintenance, biosMaintenanceFinalizer) {
		return ctrl.Result{}, nil
	}
	if err := r.cleanupReferences(ctx, log, biosMaintenance); err != nil {
		log.Error(err, "failed to cleanup references")
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured references were cleaned up")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosMaintenance, biosMaintenanceFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("biosMaintenance is deleted")
	return ctrl.Result{}, nil
}

func (r *BiosMaintenanceReconciler) cleanupServerMaintenanceReferences(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
) error {
	if biosMaintenance.Spec.ServerMaintenanceRef == nil {
		return nil
	}
	// try to get the serverMaintaince created
	serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, biosMaintenance.Spec.ServerMaintenanceRef)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get referred serverMaintenance obj from biosMaintenance: %w", err)
	}

	// if we got the server ref. by and its not being deleted
	if err == nil && serverMaintenance.DeletionTimestamp.IsZero() {
		// if the serverMaintenace is not created by biosMaintenance Controller, dont delete.
		labelsOnMaintenance := serverMaintenance.GetLabels()

		// created by the controller
		if key, ok := labelsOnMaintenance[biosMaintenanceCreatorLabel]; ok && key == biosMaintenance.Name {
			// if the biosMaintenance is not being deleted, update the
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
		err = r.patchMaintenanceRequestRefOnBiosMaintenance(ctx, log, biosMaintenance, nil)
		if err != nil {
			return fmt.Errorf("failed to clean up serverMaintenance ref in biosMaintenance status: %w", err)
		}
	}
	return nil
}

func (r *BiosMaintenanceReconciler) cleanupReferences(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
) (err error) {
	if err := r.cleanupServerMaintenanceReferences(ctx, log, biosMaintenance); err != nil {
		return err
	}

	if biosMaintenance.Spec.ServerRef != nil {
		server, err := r.getReferredServer(ctx, log, biosMaintenance.Spec.ServerRef)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		// if we can not find the server, nothing else to clean up
		if apierrors.IsNotFound(err) {
			return nil
		}
		// if we have found the server, check if ref is this serevrBIOS and remove it
		if err == nil {
			if server.Spec.BIOSSettingsRef != nil {
				if server.Spec.BIOSSettingsRef.Name != biosMaintenance.Name {
					return nil
				}
				return r.patchBiosMaintenanceRefOnServer(ctx, log, server, nil)
			} else {
				// nothing else to clean up
				return nil
			}
		}
	}

	return err
}

func (r *BiosMaintenanceReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(biosMaintenance) {
		log.V(1).Info("Skipped BIOS Setting reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, biosMaintenance, biosMaintenanceFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	return r.ensureServerMaintenanceStateTransition(ctx, log, biosMaintenance, server)
}

func (r *BiosMaintenanceReconciler) ensureServerMaintenanceStateTransition(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	switch biosMaintenance.Status.State {
	case "":
		//todo: check that in initial state there is no pending BIOS maintenance left behind,

		// move to upgrade to check if version matches
		err := r.updateBiosMaintenanceStatus(ctx, log, biosMaintenance, metalv1alpha1.BiosMaintenanceStateInVersionUpgrade)
		return ctrl.Result{}, err
	case metalv1alpha1.BiosMaintenanceStateInVersionUpgrade:
		return r.handleVersionUpgradeState(ctx, log, biosMaintenance, server)
	case metalv1alpha1.BiosMaintenanceStateInSettingUpdate:
		return r.handleSettingUpdateState(ctx, log, biosMaintenance, server)
	case metalv1alpha1.BiosMaintenanceStateSynced:
		return r.handleSettingSyncedState(ctx, log, biosMaintenance, server)
	case metalv1alpha1.BiosMaintenanceStateFailed:
		return r.handleFailedState(ctx, log, biosMaintenance, server)
	}
	log.V(1).Info("Unknown State found", "biosMaintenance state", biosMaintenance.Status.State)
	return ctrl.Result{}, nil
}

func (r *BiosMaintenanceReconciler) handleVersionUpgradeState(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {

	//check if the version match
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	// fetch the current bios version from the server bmc
	currentBiosVersion, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemUUID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to load bios version: %w for server %v", err, server.Name)
	}

	// todo: handle version check correctly to detect and upgrade only to higher version
	if currentBiosVersion == biosMaintenance.Spec.BiosSettings.Version {
		// move status to inMaintenance to check if settings needs to be upgraded
		log.V(1).Info("BIOS version matches")
		err := r.updateBiosMaintenanceStatus(ctx, log, biosMaintenance, metalv1alpha1.BiosMaintenanceStateInSettingUpdate)
		return ctrl.Result{}, err
	} else if currentBiosVersion > biosMaintenance.Spec.BiosSettings.Version {
		log.V(1).Info("BIOS downgrade is not supported", "currentBiosVersion", currentBiosVersion, "requested", biosMaintenance.Spec.BiosSettings.Version)
		err := r.updateBiosMaintenanceStatus(ctx, log, biosMaintenance, metalv1alpha1.BiosMaintenanceStateFailed)
		return ctrl.Result{}, err
	}

	// BIOS upgrade always need server reboot, Hence need maintenance request.
	if requeue, err := r.requestMaintenanceOnServer(ctx, log, biosMaintenance, server); err != nil || requeue {
		return ctrl.Result{}, err
	}

	// wait for maintenance request to be granted
	if ok := r.checkIfMaintenanceGranted(log, biosMaintenance, server); !ok {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with version upgrade")
		return ctrl.Result{}, nil
	}

	// todo: do actual upgrade here.
	//time.Sleep(1 * time.Second)
	log.V(1).Info("Updated BiosMaintenance settings")

	// reque to check version and move to setting update
	return ctrl.Result{Requeue: true}, err
}

func (r *BiosMaintenanceReconciler) handleSettingUpdateState(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {

	_, settingsDiff, err := r.getBiosSettingDifference(ctx, log, biosMaintenance, server)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings: %w", err)
	}
	// if setting is not different, complete the BIOS tasks
	if len(settingsDiff) == 0 {
		// move status to completed
		err := r.updateBiosMaintenanceStatus(ctx, log, biosMaintenance, metalv1alpha1.BiosMaintenanceStateSynced)
		return ctrl.Result{}, err
	}

	if req, err := r.checkAndRequestMaintenance(ctx, log, biosMaintenance, server, settingsDiff); err != nil || req {
		return ctrl.Result{}, err
	}

	// check if the maintenance is granted
	if ok := r.checkIfMaintenanceGranted(log, biosMaintenance, server); !ok {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with updating settings")
		return ctrl.Result{}, nil
	}

	return r.applySettingUpdateStateTransition(ctx, log, biosMaintenance, server, settingsDiff)
}

func (r *BiosMaintenanceReconciler) checkAndRequestMaintenance(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
	settingsDiff map[string]string,
) (bool, error) {
	// check if we need to request maintenance if we dont have it already
	// note: having this check will reduce the call made to BMC.
	if biosMaintenance.Spec.ServerMaintenanceRef == nil {
		bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
		if err != nil {
			return false, err
		}
		defer bmcClient.Logout()

		resetReq, err := bmcClient.CheckBiosAttributes(settingsDiff)
		if resetReq {
			// request maintenance if needed, even if err was reported.
			requeue, errMainReq := r.requestMaintenanceOnServer(ctx, log, biosMaintenance, server)
			return requeue, errors.Join(err, errMainReq)
		}
		if err != nil {
			return false, fmt.Errorf("failed to check BMC settings provided: %w", err)
		}
	}
	return false, nil
}

func (r *BiosMaintenanceReconciler) applySettingUpdateStateTransition(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
	settingsDiff map[string]string,
) (ctrl.Result, error) {
	switch biosMaintenance.Status.UpdateSettingState {
	case "":
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.updateBIOSSettingUpdateStatus(ctx, log, biosMaintenance, metalv1alpha1.BiosSettingUpdateStateIssue)
			return ctrl.Result{}, err
		}
		err := r.patchServerMaintenancePowerState(ctx, log, biosMaintenance, metalv1alpha1.PowerOn)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to turn on server %w", err)
		}
		return ctrl.Result{}, err
	case metalv1alpha1.BiosSettingUpdateStateIssue:
		// todo: make it idepotent
		bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
		if err != nil {
			return ctrl.Result{}, err
		}
		defer bmcClient.Logout()

		err = bmcClient.SetBiosAttributesOnReset(ctx, server.Spec.SystemUUID, settingsDiff)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set BMC settings: %w", err)
		}

		// if we dont need (have not requested maintenance) reboot. skip reboot steps.
		nextState := metalv1alpha1.BiosSettingUpdateWaitOnServerRebootPowerOff
		if biosMaintenance.Spec.ServerMaintenanceRef == nil {
			nextState = metalv1alpha1.BiosSettingUpdateStateVerification
		}

		err = r.updateBIOSSettingUpdateStatus(ctx, log, biosMaintenance, nextState)
		return ctrl.Result{}, err
	case metalv1alpha1.BiosSettingUpdateWaitOnServerRebootPowerOff:
		// expected state it to be off and initial state is to be on.
		// todo: check that the BiosMaintenance setting is actually been issued.
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.patchServerMaintenancePowerState(ctx, log, biosMaintenance, metalv1alpha1.PowerOff)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOffPowerState) {
			err := r.updateBIOSSettingUpdateStatus(ctx, log, biosMaintenance, metalv1alpha1.BiosSettingUpdateWaitOnServerRebootPowerOn)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	case metalv1alpha1.BiosSettingUpdateWaitOnServerRebootPowerOn:
		// expected power state it to be on and initial state is to be off.
		// todo: check that the BiosMaintenance setting is actually been issued.
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOffPowerState) {
			err := r.patchServerMaintenancePowerState(ctx, log, biosMaintenance, metalv1alpha1.PowerOn)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.updateBIOSSettingUpdateStatus(ctx, log, biosMaintenance, metalv1alpha1.BiosSettingUpdateStateVerification)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	case metalv1alpha1.BiosSettingUpdateStateVerification:
		// make sure the setting has actually applied.
		_, settingsDiff, err := r.getBiosSettingDifference(ctx, log, biosMaintenance, server)

		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings: %w", err)
		}
		// if setting is not different, complete the BIOS tasks
		if len(settingsDiff) == 0 {
			// move  biosMaintenance state to completed, and revert the settingUpdate state to initial
			err := r.updateBiosMaintenanceStatus(ctx, log, biosMaintenance, metalv1alpha1.BiosMaintenanceStateSynced)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, fmt.Errorf("waiting on the BIOS setting to take place")
	}
	log.V(1).Info("Unknown State found", "biosMaintenance UpdateSetting state", biosMaintenance.Status.UpdateSettingState)
	// stop reconsile as we can not proceed with unknown state
	return ctrl.Result{}, nil
}

func (r *BiosMaintenanceReconciler) handleSettingSyncedState(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	// clean up maintenance crd and references.
	if err := r.cleanupServerMaintenanceReferences(ctx, log, biosMaintenance); err != nil {
		return ctrl.Result{}, err
	}

	diffPresent, settingsDiff, err := r.getBiosSettingDifference(ctx, log, biosMaintenance, server)

	if err != nil {
		log.V(1).Error(err, "unable to fetch and check BIOSSettings")
		return ctrl.Result{}, err
	}
	if diffPresent || len(settingsDiff) > 0 {
		err := r.updateBiosMaintenanceStatus(ctx, log, biosMaintenance, "")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Done with bios setting update", "ctx", ctx, "biosMaintenance", biosMaintenance, "server", server)
	return ctrl.Result{}, nil
}

func (r *BiosMaintenanceReconciler) handleFailedState(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {

	if biosMaintenance.Spec.ServerMaintenanceRef != nil {
		serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, server.Spec.ServerMaintenanceRef)
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
		// todo: revist this logic to either create maintenance or not, put server in Error state on failed bios settings maintenance
		//			this would need update on servermaintenanceContoller to go handle this usecase.

		// request maintenance if needed,
		// if biosMaintenance.Spec.ServerMaintenanceRef == nil {
		// 	if err := r.requestMaintenanceOnServer(ctx, log, biosMaintenance, server); err != nil {
		// 		return ctrl.Result{}, err
		// 	}
		// }
		// move maintenance to failed state directly.
	}

	log.V(1).Info("Failed to update bios setting", "ctx", ctx, "biosMaintenance", biosMaintenance, "server", server)
	return ctrl.Result{}, nil
}

func (r *BiosMaintenanceReconciler) getBiosSettingDifference(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
) (updatedNeeded bool, diff map[string]string, err error) {
	// todo: need to also account for future pending changes reported for bios
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		return false, diff, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	keys := slices.Collect(maps.Keys(biosMaintenance.Spec.BiosSettings.SettingsMap))

	currentSettings, err := bmcClient.GetBiosAttributeValues(ctx, server.Spec.SystemUUID, keys)
	if err != nil {
		log.V(1).Info("Failed to get with bios setting", "error", err)
		return false, diff, fmt.Errorf("failed to get BIOS settings: %w", err)
	}

	diff = map[string]string{}
	for key, value := range biosMaintenance.Spec.BiosSettings.SettingsMap {
		res, ok := currentSettings[key]
		if ok {
			if res.(string) != value {
				diff[key] = value
			}
		} else {
			diff[key] = value
		}
	}

	// fetch the current bios version from the server bmc
	currentBiosVersion, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemUUID)
	if err != nil {
		return false, diff, fmt.Errorf("failed to load bios version: %w for server %v", err, server.Name)
	}

	// todo: handle version check to detect and upgrade only higher version
	if currentBiosVersion != biosMaintenance.Spec.BiosSettings.Version {
		return true, diff, nil
	}

	return false, diff, nil
}

func (r *BiosMaintenanceReconciler) checkForRequiredPowerStatus(
	server *metalv1alpha1.Server,
	powerState metalv1alpha1.ServerPowerState,
) bool {
	return server.Status.PowerState == powerState
}

func (r *BiosMaintenanceReconciler) checkIfMaintenanceGranted(
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
) bool {

	if biosMaintenance.Spec.ServerMaintenanceRef == nil {
		return true
	}

	if server.Status.State == metalv1alpha1.ServerStateMaintenance {
		if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.UID != biosMaintenance.Spec.ServerMaintenanceRef.UID {
			// server in maintenance for other tasks. or
			// server maintenance ref is wrong in either server or biosMaintenance
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

func (r *BiosMaintenanceReconciler) requestMaintenanceOnServer(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	server *metalv1alpha1.Server,
) (bool, error) {

	// if Server maintenance ref is already given. no further action required.
	if biosMaintenance.Spec.ServerMaintenanceRef != nil {
		return false, nil
	}

	serverMaintenance := &metalv1alpha1.ServerMaintenance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.ManagerNamespace,
			Name:      biosMaintenance.Name,
		}}

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
		serverMaintenance.SetLabels(map[string]string{biosMaintenanceCreatorLabel: biosMaintenance.Name})
		serverMaintenance.Spec.Policy = biosMaintenance.Spec.ServerMaintenancePolicy
		serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
		serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
		if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
			serverMaintenance.Status.State = ""
		}
		return controllerutil.SetControllerReference(biosMaintenance, serverMaintenance, r.Client.Scheme())
	})
	if err != nil {
		return false, fmt.Errorf("failed to create or patch serverMaintenance: %w", err)
	}
	log.V(1).Info("Created serverMaintenance", "serverMaintenance", serverMaintenance.Name, "serverMaintenance label", serverMaintenance.Labels, "Operation", opResult)

	err = r.patchMaintenanceRequestRefOnBiosMaintenance(ctx, log, biosMaintenance, serverMaintenance)
	if err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in biosMaintenance status: %w", err)
	}

	log.V(1).Info("Patched serverMaintenance on biosMaintenance")

	return true, nil
}

func (r *BiosMaintenanceReconciler) getReferredServer(
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

func (r *BiosMaintenanceReconciler) getReferredServerMaintenance(
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

func (r *BiosMaintenanceReconciler) getReferredbiosMaintenance(
	ctx context.Context,
	log logr.Logger,
	referredBIOSSetteingRef *corev1.LocalObjectReference,
) (*metalv1alpha1.BiosMaintenance, error) {
	key := client.ObjectKey{Name: referredBIOSSetteingRef.Name, Namespace: metav1.NamespaceNone}
	biosMaintenance := &metalv1alpha1.BiosMaintenance{}
	if err := r.Get(ctx, key, biosMaintenance); err != nil {
		log.V(1).Error(err, "failed to get referred BIOSSetting")
		return biosMaintenance, err
	}
	return biosMaintenance, nil
}

func (r *BiosMaintenanceReconciler) patchBiosMaintenanceRefOnServer(
	ctx context.Context,
	log logr.Logger,
	server *metalv1alpha1.Server,
	biosMaintenanceReference *corev1.LocalObjectReference,
) (err error) {
	if server.Spec.BIOSSettingsRef == biosMaintenanceReference {
		return nil
	}

	serverBase := server.DeepCopy()
	server.Spec.BIOSSettingsRef = biosMaintenanceReference
	if err = r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		log.V(1).Error(err, "failed to patch bios settings ref")
		return err
	}
	return err
}

func (r *BiosMaintenanceReconciler) patchMaintenanceRequestRefOnBiosMaintenance(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	serverMaintenance *metalv1alpha1.ServerMaintenance,
) error {
	biosMaintenanceBase := biosMaintenance.DeepCopy()

	if serverMaintenance == nil {
		biosMaintenance.Spec.ServerMaintenanceRef = nil
	} else {
		biosMaintenance.Spec.ServerMaintenanceRef = &corev1.ObjectReference{
			APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			Kind:       "ServerMaintenance",
			Namespace:  serverMaintenance.Namespace,
			Name:       serverMaintenance.Name,
			UID:        serverMaintenance.UID,
		}
	}

	if err := r.Patch(ctx, biosMaintenance, client.MergeFrom(biosMaintenanceBase)); err != nil {
		log.V(1).Error(err, "failed to patch bios settings ref")
		return err
	}

	return nil
}

func (r *BiosMaintenanceReconciler) patchServerMaintenancePowerState(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	powerState metalv1alpha1.Power,
) error {
	serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, biosMaintenance.Spec.ServerMaintenanceRef)
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

func (r *BiosMaintenanceReconciler) updateBiosMaintenanceStatus(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	state metalv1alpha1.BiosMaintenanceState,
) error {

	if biosMaintenance.Status.State == state {
		return nil
	}

	biosMaintenanceBase := biosMaintenance.DeepCopy()
	biosMaintenance.Status.State = state

	if state == metalv1alpha1.BiosMaintenanceStateSynced {
		biosMaintenance.Status.UpdateSettingState = ""
	}

	if err := r.Status().Patch(ctx, biosMaintenance, client.MergeFrom(biosMaintenanceBase)); err != nil {
		return fmt.Errorf("failed to patch BiosMaintenance status: %w", err)
	}

	log.V(1).Info("Updated biosMaintenance state ", "new state", state)

	return nil
}

func (r *BiosMaintenanceReconciler) updateBIOSSettingUpdateStatus(
	ctx context.Context,
	log logr.Logger,
	biosMaintenance *metalv1alpha1.BiosMaintenance,
	state metalv1alpha1.BiosSettingUpdateState,
) error {

	if biosMaintenance.Status.UpdateSettingState == state {
		return nil
	}

	biosMaintenanceBase := biosMaintenance.DeepCopy()
	biosMaintenance.Status.UpdateSettingState = state

	if err := r.Status().Patch(ctx, biosMaintenance, client.MergeFrom(biosMaintenanceBase)); err != nil {
		return fmt.Errorf("failed to patch BiosMaintenance UpdateSetting status: %w", err)
	}

	log.V(1).Info("Updated biosMaintenance UpdateSetting state ", "new state", state)

	return nil
}

func (r *BiosMaintenanceReconciler) enqueueBiosMaintenanceByRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)
	biosMaintenanceList := &metalv1alpha1.BiosMaintenanceList{}
	if err := r.List(ctx, biosMaintenanceList); err != nil {
		log.Error(err, "failed to list biosMaintenancees")
		return nil
	}
	var req []ctrl.Request

	for _, biosMaintenance := range biosMaintenanceList.Items {
		if biosMaintenance.Spec.ServerRef.Name == host.Name && biosMaintenance.Spec.ServerMaintenanceRef != nil {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: biosMaintenance.Namespace, Name: biosMaintenance.Name},
			})
		}
	}
	return req
}

// SetupWithManager sets up the controller with the Manager.
func (r *BiosMaintenanceReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BiosMaintenance{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosMaintenanceByRefs)).
		Complete(r)
}
