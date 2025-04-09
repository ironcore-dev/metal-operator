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

// ServerBIOSReconciler reconciles a ServerBIOS object
type ServerBIOSReconciler struct {
	client.Client
	ManagerNamespace string
	Insecure         bool
	Scheme           *runtime.Scheme
	BMCOptions       bmc.BMCOptions
}

const serverBIOSFinalizer = "firmware.ironcore.dev/serverbios"

const serverBIOSCreatorLabel = "firmware.ironcore.dev/created-by"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbioses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbioses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbioses/finalizers,verbs=update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerBIOSReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	serverBIOS := &metalv1alpha1.ServerBIOS{}
	if err := r.Get(ctx, req.NamespacedName, serverBIOS); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling serverBIOS")

	return r.reconcileExists(ctx, log, serverBIOS)
}

// Determine whether reconciliation is required. It's not required if:
// - object is being deleted;
// - object does not contain reference to server;
// - object contains reference to server, but server references to another object with lower version;
func (r *ServerBIOSReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if !serverBIOS.DeletionTimestamp.IsZero() {
		log.V(1).Info("object is being deleted")
		return r.reconcileDeletion(ctx, log, serverBIOS)
	}

	// if object does not refer to server object - stop reconciliation
	if serverBIOS.Spec.ServerRef == nil {
		log.V(1).Info("object does not refer to server object")
		return ctrl.Result{}, nil
	}

	// if referred server contains reference to different ServerBIOS object - stop reconciliation
	server, err := r.getReferredServer(ctx, log, serverBIOS.Spec.ServerRef)
	if err != nil {
		log.V(1).Info("referred server object could not be fetched")
		return ctrl.Result{}, err
	}
	// patch server with serverbios reference
	if server.Spec.BIOSSettingsRef == nil {
		if err := r.patchServerBIOSRefOnServer(ctx, log, server, &corev1.LocalObjectReference{Name: serverBIOS.Name}); err != nil {
			return ctrl.Result{}, err
		}
		// because we requeue server only after serverMaintenance is created. we need to manually requeue here.
		return ctrl.Result{Requeue: true}, nil
	}

	if server.Spec.BIOSSettingsRef.Name != serverBIOS.Name {
		referredBIOSSetting, err := r.getReferredserverBIOS(ctx, log, server.Spec.BIOSSettingsRef)
		if err != nil {
			log.V(1).Info("referred server contains reference to different ServerBIOS object, unable to fetch the referenced bios setting")
			return ctrl.Result{}, err
		}
		// check if the current BIOS setting version is newer and update reference if it is newer
		// todo : handle version checks correctly
		if referredBIOSSetting.Spec.BIOS.Version < serverBIOS.Spec.BIOS.Version {
			log.V(1).Info("Updating BIOSSetting reference to the latest BIOS version")
			if err := r.patchServerBIOSRefOnServer(ctx, log, server, &corev1.LocalObjectReference{Name: serverBIOS.Name}); err != nil {
				return ctrl.Result{}, err
			}
			// because we requeue server only after serverMaintenance is created. we need to manually requeue here.
			return ctrl.Result{Requeue: true}, nil
		}
	}

	return r.reconcile(ctx, log, serverBIOS, server)
}

func (r *ServerBIOSReconciler) reconcileDeletion(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(serverBIOS, serverBIOSFinalizer) {
		return ctrl.Result{}, nil
	}
	if err := r.cleanupReferences(ctx, log, serverBIOS); err != nil {
		log.Error(err, "failed to cleanup references")
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured references were cleaned up")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, serverBIOS, serverBIOSFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("serverBIOS is deleted")
	return ctrl.Result{}, nil
}

func (r *ServerBIOSReconciler) cleanupServerMaintenanceReferences(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) error {
	if serverBIOS.Spec.ServerMaintenanceRef == nil {
		return nil
	}
	// try to get the serverMaintaince created
	serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, serverBIOS.Spec.ServerMaintenanceRef)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get referred serverMaintenance obj from serverBIOS: %w", err)
	}

	// if we got the server ref. by and its not being deleted
	if err == nil && serverMaintenance.DeletionTimestamp.IsZero() {
		// if the serverMaintenace is not created by serverBIOS Controller, dont delete.
		labelsOnMaintenance := serverMaintenance.GetLabels()

		// created by the controller
		if key, ok := labelsOnMaintenance[serverBIOSCreatorLabel]; ok && key == serverBIOS.Name {
			// if the serverBIOS is not being deleted, update the
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
		err = r.patchMaintenanceRequestRefOnServerBIOS(ctx, log, serverBIOS, nil)
		if err != nil {
			return fmt.Errorf("failed to clean up serverMaintenance ref in serverBIOS status: %w", err)
		}
	}
	return nil
}

func (r *ServerBIOSReconciler) cleanupReferences(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (err error) {
	if err := r.cleanupServerMaintenanceReferences(ctx, log, serverBIOS); err != nil {
		return err
	}

	if serverBIOS.Spec.ServerRef != nil {
		server, err := r.getReferredServer(ctx, log, serverBIOS.Spec.ServerRef)
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
				if server.Spec.BIOSSettingsRef.Name != serverBIOS.Name {
					return nil
				}
				return r.patchServerBIOSRefOnServer(ctx, log, server, nil)
			} else {
				// nothing else to clean up
				return nil
			}
		}
	}

	return err
}

func (r *ServerBIOSReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(serverBIOS) {
		log.V(1).Info("Skipped BIOS Setting reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, serverBIOS, serverBIOSFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	return r.ensureServerMaintenanceStateTransition(ctx, log, serverBIOS, server)
}

func (r *ServerBIOSReconciler) ensureServerMaintenanceStateTransition(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	switch serverBIOS.Status.State {
	case "":
		//todo: check that in initial state there is no pending BIOS maintenance left behind,

		// move to upgrade to check if version matches
		err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateInVersionUpgrade)
		return ctrl.Result{}, err
	case metalv1alpha1.BIOSMaintenanceStateInVersionUpgrade:
		return r.handleVersionUpgradeState(ctx, log, serverBIOS, server)
	case metalv1alpha1.BIOSMaintenanceStateInSettingUpdate:
		return r.handleSettingUpdateState(ctx, log, serverBIOS, server)
	case metalv1alpha1.BIOSMaintenanceStateSynced:
		return r.handleSettingSyncedState(ctx, log, serverBIOS, server)
	case metalv1alpha1.BIOSMaintenanceStateFailed:
		return r.handleFailedState(ctx, log, serverBIOS, server)
	}
	log.V(1).Info("Unknown State found", "serverBIOS state", serverBIOS.Status.State)
	return ctrl.Result{}, nil
}

func (r *ServerBIOSReconciler) handleVersionUpgradeState(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
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
	if currentBiosVersion == serverBIOS.Spec.BIOS.Version {
		// move status to inMaintenance to check if settings needs to be upgraded
		log.V(1).Info("BIOS version matches")
		err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateInSettingUpdate)
		return ctrl.Result{}, err
	} else if currentBiosVersion > serverBIOS.Spec.BIOS.Version {
		log.V(1).Info("BIOS downgrade is not supported", "currentBiosVersion", currentBiosVersion, "requested", serverBIOS.Spec.BIOS.Version)
		err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateFailed)
		return ctrl.Result{}, err
	}

	// BIOS upgrade always need server reboot, Hence need maintenance request.
	if requeue, err := r.requestMaintenanceOnServer(ctx, log, serverBIOS, server); err != nil || requeue {
		return ctrl.Result{}, err
	}

	// wait for maintenance request to be granted
	if ok := r.checkIfMaintenanceGranted(log, serverBIOS, server); !ok {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with version upgrade")
		return ctrl.Result{}, nil
	}

	// todo: do actual upgrade here.
	//time.Sleep(1 * time.Second)
	log.V(1).Info("Updated Server BIOS settings")

	// reque to check version and move to setting update
	return ctrl.Result{Requeue: true}, err
}

func (r *ServerBIOSReconciler) handleSettingUpdateState(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {

	_, settingsDiff, err := r.getBiosSettingDifference(ctx, log, serverBIOS, server)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings: %w", err)
	}
	// if setting is not different, complete the BIOS tasks
	if len(settingsDiff) == 0 {
		// move status to completed
		err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateSynced)
		return ctrl.Result{}, err
	}

	if req, err := r.checkAndRequestMaintenance(ctx, log, serverBIOS, server, settingsDiff); err != nil || req {
		return ctrl.Result{}, err
	}

	// check if the maintenance is granted
	if ok := r.checkIfMaintenanceGranted(log, serverBIOS, server); !ok {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with updating settings")
		return ctrl.Result{}, nil
	}

	return r.applySettingUpdateStateTransition(ctx, log, serverBIOS, server, settingsDiff)
}

func (r *ServerBIOSReconciler) checkAndRequestMaintenance(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
	settingsDiff map[string]string,
) (bool, error) {
	// check if we need to request maintenance if we dont have it already
	// note: having this check will reduce the call made to BMC.
	if serverBIOS.Spec.ServerMaintenanceRef == nil {
		bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
		if err != nil {
			return false, err
		}
		defer bmcClient.Logout()

		resetReq, err := bmcClient.CheckBiosAttributes(settingsDiff)
		if resetReq {
			// request maintenance if needed, even if err was reported.
			requeue, errMainReq := r.requestMaintenanceOnServer(ctx, log, serverBIOS, server)
			return requeue, errors.Join(err, errMainReq)
		}
		if err != nil {
			return false, fmt.Errorf("failed to check BMC settings provided: %w", err)
		}
	}
	return false, nil
}

func (r *ServerBIOSReconciler) applySettingUpdateStateTransition(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
	settingsDiff map[string]string,
) (ctrl.Result, error) {
	switch serverBIOS.Status.UpdateSettingState {
	case "":
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.updateBIOSSettingUpdateStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSSettingUpdateStateIssue)
			return ctrl.Result{}, err
		}
		err := r.patchServerMaintenancePowerState(ctx, log, serverBIOS, metalv1alpha1.PowerOn)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to turn on server %w", err)
		}
		return ctrl.Result{}, err
	case metalv1alpha1.BIOSSettingUpdateStateIssue:
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
		nextState := metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOff
		if serverBIOS.Spec.ServerMaintenanceRef == nil {
			nextState = metalv1alpha1.BIOSSettingUpdateStateVerification
		}

		err = r.updateBIOSSettingUpdateStatus(ctx, log, serverBIOS, nextState)
		return ctrl.Result{}, err
	case metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOff:
		// expected state it to be off and initial state is to be on.
		// todo: check that the server bios setting is actually been issued.
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.patchServerMaintenancePowerState(ctx, log, serverBIOS, metalv1alpha1.PowerOff)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOffPowerState) {
			err := r.updateBIOSSettingUpdateStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOn)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	case metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOn:
		// expected power state it to be on and initial state is to be off.
		// todo: check that the server bios setting is actually been issued.
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOffPowerState) {
			err := r.patchServerMaintenancePowerState(ctx, log, serverBIOS, metalv1alpha1.PowerOn)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reboot %w", err)
			}
		}
		if r.checkForRequiredPowerStatus(server, metalv1alpha1.ServerOnPowerState) {
			err := r.updateBIOSSettingUpdateStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSSettingUpdateStateVerification)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	case metalv1alpha1.BIOSSettingUpdateStateVerification:
		// make sure the setting has actually applied.
		_, settingsDiff, err := r.getBiosSettingDifference(ctx, log, serverBIOS, server)

		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings: %w", err)
		}
		// if setting is not different, complete the BIOS tasks
		if len(settingsDiff) == 0 {
			// move  serverBIOS state to completed, and revert the settingUpdate state to initial
			err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateSynced)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, fmt.Errorf("waiting on the BIOS setting to take place")
	}
	log.V(1).Info("Unknown State found", "serverBIOS UpdateSetting state", serverBIOS.Status.UpdateSettingState)
	// stop reconsile as we can not proceed with unknown state
	return ctrl.Result{}, nil
}

func (r *ServerBIOSReconciler) handleSettingSyncedState(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	// clean up maintenance crd and references.
	if err := r.cleanupServerMaintenanceReferences(ctx, log, serverBIOS); err != nil {
		return ctrl.Result{}, err
	}

	diffPresent, settingsDiff, err := r.getBiosSettingDifference(ctx, log, serverBIOS, server)

	if err != nil {
		log.V(1).Error(err, "unable to fetch and check BIOSSettings")
		return ctrl.Result{}, err
	}
	if diffPresent || len(settingsDiff) > 0 {
		err := r.updateServerBIOSStatus(ctx, log, serverBIOS, "")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Done with bios setting update", "ctx", ctx, "serverBIOS", serverBIOS, "server", server)
	return ctrl.Result{}, nil
}

func (r *ServerBIOSReconciler) handleFailedState(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {

	if serverBIOS.Spec.ServerMaintenanceRef != nil {
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
		// if serverBIOS.Spec.ServerMaintenanceRef == nil {
		// 	if err := r.requestMaintenanceOnServer(ctx, log, serverBIOS, server); err != nil {
		// 		return ctrl.Result{}, err
		// 	}
		// }
		// move maintenance to failed state directly.
	}

	log.V(1).Info("Failed to update bios setting", "ctx", ctx, "serverBIOS", serverBIOS, "server", server)
	return ctrl.Result{}, nil
}

func (r *ServerBIOSReconciler) getBiosSettingDifference(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
) (updatedNeeded bool, diff map[string]string, err error) {
	// todo: need to also account for future pending changes reported for bios
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		return false, diff, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	keys := slices.Collect(maps.Keys(serverBIOS.Spec.BIOS.Settings))

	currentSettings, err := bmcClient.GetBiosAttributeValues(ctx, server.Spec.SystemUUID, keys)
	if err != nil {
		log.V(1).Info("Failed to get with bios setting", "error", err)
		return false, diff, fmt.Errorf("failed to get BIOS settings: %w", err)
	}

	diff = map[string]string{}
	for key, value := range serverBIOS.Spec.BIOS.Settings {
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
	if currentBiosVersion != serverBIOS.Spec.BIOS.Version {
		return true, diff, nil
	}

	return false, diff, nil
}

func (r *ServerBIOSReconciler) checkForRequiredPowerStatus(
	server *metalv1alpha1.Server,
	powerState metalv1alpha1.ServerPowerState,
) bool {
	return server.Status.PowerState == powerState
}

func (r *ServerBIOSReconciler) checkIfMaintenanceGranted(
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
) bool {

	if serverBIOS.Spec.ServerMaintenanceRef == nil {
		return true
	}

	if server.Status.State == metalv1alpha1.ServerStateMaintenance {
		if server.Spec.ServerMaintenanceRef == nil || server.Spec.ServerMaintenanceRef.UID != serverBIOS.Spec.ServerMaintenanceRef.UID {
			// server in maintenance for other tasks. or
			// server maintenance ref is wrong in either server or serverBIOS
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

func (r *ServerBIOSReconciler) requestMaintenanceOnServer(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	server *metalv1alpha1.Server,
) (bool, error) {

	// if Server maintenance ref is already given. no further action required.
	if serverBIOS.Spec.ServerMaintenanceRef != nil {
		return false, nil
	}

	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	serverMaintenance.Name = serverBIOS.Name
	serverMaintenance.Namespace = r.ManagerNamespace

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
		serverMaintenance.SetLabels(map[string]string{serverBIOSCreatorLabel: serverBIOS.Name})
		serverMaintenance.Spec.Policy = serverBIOS.Spec.ServerMaintenancePolicy
		serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOn
		serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
		if serverMaintenance.Status.State != metalv1alpha1.ServerMaintenanceStateInMaintenance && serverMaintenance.Status.State != "" {
			serverMaintenance.Status.State = ""
		}
		return controllerutil.SetControllerReference(serverBIOS, serverMaintenance, r.Client.Scheme())
	})
	if err != nil {
		return false, fmt.Errorf("failed to create or patch serverMaintenance: %w", err)
	}
	log.V(1).Info("Created serverMaintenance", "serverMaintenance", serverMaintenance.Name, "serverMaintenance label", serverMaintenance.Labels, "Operation", opResult)

	err = r.patchMaintenanceRequestRefOnServerBIOS(ctx, log, serverBIOS, serverMaintenance)
	if err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance ref in serverBIOS status: %w", err)
	}

	log.V(1).Info("Patched serverMaintenance on serverBIOS")

	return true, nil
}

func (r *ServerBIOSReconciler) getReferredServer(
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

func (r *ServerBIOSReconciler) getReferredServerMaintenance(
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

func (r *ServerBIOSReconciler) getReferredserverBIOS(
	ctx context.Context,
	log logr.Logger,
	referredBIOSSetteingRef *corev1.LocalObjectReference,
) (*metalv1alpha1.ServerBIOS, error) {
	key := client.ObjectKey{Name: referredBIOSSetteingRef.Name, Namespace: metav1.NamespaceNone}
	serverBIOS := &metalv1alpha1.ServerBIOS{}
	if err := r.Get(ctx, key, serverBIOS); err != nil {
		log.V(1).Error(err, "failed to get referred BIOSSetting")
		return serverBIOS, err
	}
	return serverBIOS, nil
}

func (r *ServerBIOSReconciler) patchServerBIOSRefOnServer(
	ctx context.Context,
	log logr.Logger,
	server *metalv1alpha1.Server,
	serverBIOSReference *corev1.LocalObjectReference,
) (err error) {
	if server.Spec.BIOSSettingsRef == serverBIOSReference {
		return nil
	}

	serverBase := server.DeepCopy()
	server.Spec.BIOSSettingsRef = serverBIOSReference
	if err = r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		log.V(1).Error(err, "failed to patch bios settings ref")
		return err
	}
	return err
}

func (r *ServerBIOSReconciler) patchMaintenanceRequestRefOnServerBIOS(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	serverMaintenance *metalv1alpha1.ServerMaintenance,
) error {
	serverBIOSBase := serverBIOS.DeepCopy()

	if serverMaintenance == nil {
		serverBIOS.Spec.ServerMaintenanceRef = nil
	} else {
		serverBIOS.Spec.ServerMaintenanceRef = &corev1.ObjectReference{
			APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			Kind:       "ServerMaintenance",
			Namespace:  serverMaintenance.Namespace,
			Name:       serverMaintenance.Name,
			UID:        serverMaintenance.UID,
		}
	}

	if err := r.Patch(ctx, serverBIOS, client.MergeFrom(serverBIOSBase)); err != nil {
		log.V(1).Error(err, "failed to patch bios settings ref")
		return err
	}

	return nil
}

func (r *ServerBIOSReconciler) patchServerMaintenancePowerState(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	powerState metalv1alpha1.Power,
) error {
	serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, serverBIOS.Spec.ServerMaintenanceRef)
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

func (r *ServerBIOSReconciler) updateServerBIOSStatus(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	state metalv1alpha1.BIOSMaintenanceState,
) error {

	if serverBIOS.Status.State == state {
		return nil
	}

	serverBIOSBase := serverBIOS.DeepCopy()
	serverBIOS.Status.State = state

	if state == metalv1alpha1.BIOSMaintenanceStateSynced {
		serverBIOS.Status.UpdateSettingState = ""
	}

	if err := r.Status().Patch(ctx, serverBIOS, client.MergeFrom(serverBIOSBase)); err != nil {
		return fmt.Errorf("failed to patch ServerBIOS status: %w", err)
	}

	log.V(1).Info("Updated serverBIOS state ", "new state", state)

	return nil
}

func (r *ServerBIOSReconciler) updateBIOSSettingUpdateStatus(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	state metalv1alpha1.BIOSSettingUpdateState,
) error {

	if serverBIOS.Status.UpdateSettingState == state {
		return nil
	}

	serverBIOSBase := serverBIOS.DeepCopy()
	serverBIOS.Status.UpdateSettingState = state

	if err := r.Status().Patch(ctx, serverBIOS, client.MergeFrom(serverBIOSBase)); err != nil {
		return fmt.Errorf("failed to patch ServerBIOS UpdateSetting status: %w", err)
	}

	log.V(1).Info("Updated serverBIOS UpdateSetting state ", "new state", state)

	return nil
}

func (r *ServerBIOSReconciler) enqueueServerBIOSByRefs(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)
	serverBiosList := &metalv1alpha1.ServerBIOSList{}
	if err := r.List(ctx, serverBiosList); err != nil {
		log.Error(err, "failed to list serverBIOSes")
		return nil
	}
	var req []ctrl.Request

	for _, serverBIOS := range serverBiosList.Items {
		if serverBIOS.Spec.ServerRef.Name == host.Name && serverBIOS.Spec.ServerMaintenanceRef != nil {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: serverBIOS.Namespace, Name: serverBIOS.Name},
			})
		}
	}
	return req
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBIOSReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBIOS{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueServerBIOSByRefs)).
		Complete(r)
}
