// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ironcore-dev/metal-operator/bmc"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbios,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbios/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbios/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServerBIOS object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *ServerBIOSReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	serverBIOS := &metalv1alpha1.ServerBIOS{}
	if err := r.Get(ctx, req.NamespacedName, serverBIOS); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling serverBIOS")

	return r.reconciliationRequired(ctx, log, serverBIOS)
}

// Determine whether reconciliation is required. It's not required if:
// - object is being deleted;
// - object does not contain reference to server;
// - object contains reference to server, but server references to another object with lower version;
func (r *ServerBIOSReconciler) reconciliationRequired(
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
		if err := r.patchServerBIOSRefOnServer(ctx, log, &server, &corev1.LocalObjectReference{Name: serverBIOS.Name}); err != nil {
			return ctrl.Result{}, err
		}
	} else if server.Spec.BIOSSettingsRef.Name != serverBIOS.Name {
		referredBIOSSetting, err := r.getReferredserverBIOS(ctx, log, server.Spec.BIOSSettingsRef)
		if err != nil {
			log.V(1).Info("referred server contains reference to different ServerBIOS object, unable to fetch the referenced bios setting")
			return ctrl.Result{}, err
		}
		// check if the current BIOS setting version is newer and update reference if it is newer
		// todo : handle version checks correctly
		if referredBIOSSetting.Spec.BIOS.Version < serverBIOS.Spec.BIOS.Version {
			log.V(1).Info("Updating BIOSSetting reference to the latest BIOS version")
			if err := r.patchServerBIOSRefOnServer(ctx, log, &server, &corev1.LocalObjectReference{Name: serverBIOS.Name}); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return r.reconcile(ctx, log, serverBIOS, &server)
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

	_, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, serverBIOS, serverBIOSFinalizer)
	return ctrl.Result{}, err
}

func (r *ServerBIOSReconciler) cleanupReferences(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (err error) {
	if serverBIOS.Spec.ServerMaintenanceRef != nil {
		// try to get the serverMaintaince created
		serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, serverBIOS.Spec.ServerMaintenanceRef)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get referred serverMaintenance obj from serverBIOS: %w", err)
		}
		// if we got it, try to delete it
		if err == nil {
			if errD := r.Delete(ctx, &serverMaintenance); errD != nil {
				if !apierrors.IsNotFound(errD) {
					return fmt.Errorf("failed to delete referred serverMaintenance obj: %w", err)
				}
				log.V(1).Info("referred serverMaintenance gone")
			}
		}
		// if already deleted or have deleted it remove reference
		if apierrors.IsNotFound(err) || err == nil {
			err = r.patchMaintenanceRequestRefOnServerBIOS(ctx, log, serverBIOS, nil)
			if err != nil {
				return fmt.Errorf("failed to clean up serverMaintenance ref in serverBIOS status: %w", err)
			}
		}
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
				return r.patchServerBIOSRefOnServer(ctx, log, &server, nil)
			} else {
				// nothing else to clean up
				return nil
			}
		}
	}

	return err
}

func (r *ServerBIOSReconciler) reconcile(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(serverBIOS) {
		log.V(1).Info("Skipped BIOS Setting reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, serverBIOS, serverBIOSFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	// set the BIOS state to pending if it is not set
	if serverBIOS.Status.State == "" {
		if err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStatePending); err != nil {
			return ctrl.Result{}, err
		}
	}

	return r.ensureServerMaintenanceStateTransition(ctx, log, serverBIOS, server)
}

func (r *ServerBIOSReconciler) ensureServerMaintenanceStateTransition(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, server *metalv1alpha1.Server) (ctrl.Result, error) {
	switch serverBIOS.Status.State {
	case metalv1alpha1.BIOSMaintenanceStatePending:
		return r.handleInPendingState(ctx, log, serverBIOS, server)
	case metalv1alpha1.BIOSMaintenanceStateInVersionUpgrade:
		return r.handleVersionUpgradeState(ctx, log, serverBIOS, server)
	case metalv1alpha1.BIOSMaintenanceStateInSettingUpdate:
		return r.handleSettingUpdateState(ctx, log, serverBIOS, server)
	case metalv1alpha1.BIOSMaintenanceStateCompleted:
		return r.handleCompletedState(ctx, log, serverBIOS, server)
	case metalv1alpha1.BIOSMaintenanceStateFailed:
		return r.handleFailedState(ctx, log, serverBIOS, server)
	}
	return ctrl.Result{}, nil
}

func (r *ServerBIOSReconciler) handleInPendingState(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, server *metalv1alpha1.Server) (ctrl.Result, error) {
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

	// todo: handle version check to detect and upgrade only higher version
	if currentBiosVersion != serverBIOS.Spec.BIOS.Version {
		// upgrade the version before applying the bios setting
		if err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateInVersionUpgrade); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// move status to Maintenance to check the BIOS setting
	if err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateInSettingUpdate); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil

}
func (r *ServerBIOSReconciler) handleVersionUpgradeState(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, server *metalv1alpha1.Server) (ctrl.Result, error) {

	// BIOS upgrade always need server reboot, Hence need maintenance request.
	if serverBIOS.Spec.ServerMaintenanceRef == nil {
		err := r.requestMaintenanceOnServer(ctx, log, serverBIOS, server)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// wait for maintenance request to be granted
	if err := r.waitOnMaintenanceRequest(ctx, log, serverBIOS, server); err != nil {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with version upgrade", "reason", err)
		return ctrl.Result{Requeue: true}, nil
	}

	// todo: do actual upgrade here.
	time.Sleep(1 * time.Second)
	log.V(1).Info("Updated Server BIOS settings")

	// move status to inMaintenance to check if settings needs to be upgraded
	if err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateInSettingUpdate); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *ServerBIOSReconciler) handleSettingUpdateState(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, server *metalv1alpha1.Server) (ctrl.Result, error) {
	// check if we had requested a maintenance and the system is rebooted
	if err := r.waitOnMaintenanceRequest(ctx, log, serverBIOS, server); err != nil {
		log.V(1).Info("Waiting for maintenance to be granted before continuing with updating settings", "reason", err)
		return ctrl.Result{Requeue: true}, nil
	}

	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	keys := make([]string, 0, len(serverBIOS.Spec.BIOS.Settings))
	for k := range serverBIOS.Spec.BIOS.Settings {
		keys = append(keys, k)
	}

	currentSettings, err := bmcClient.GetBiosAttributeValues(ctx, server.Spec.SystemUUID, keys)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BIOS settings: %w", err)
	}

	diff := map[string]string{}
	for key, value := range serverBIOS.Spec.BIOS.Settings {
		res, ok := currentSettings[key]
		if ok {
			if res != value {
				diff[key] = value
			}
		} else {
			diff[key] = value
		}
	}

	// if setting is not different, complete the BIOS tasks
	if len(diff) == 0 {
		// move status to completed
		if err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateCompleted); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// check if we need to request maintenance if we dont have it already
	if serverBIOS.Spec.ServerMaintenanceRef == nil {
		resetReq, err := bmcClient.CheckBiosAttributes(diff)
		if resetReq {
			// request maintenance if needed, even if err was reported.
			errMainReq := r.requestMaintenanceOnServer(ctx, log, serverBIOS, server)
			if err != nil {
				return ctrl.Result{}, errors.Join(err, errMainReq)
			}
			// reque to check for the maintenance is granted and continue with workflow
			return ctrl.Result{Requeue: true}, err
		}
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to check BMC settings provided: %w", err)
		}
	}

	_, err = bmcClient.SetBiosAttributes(ctx, server.Spec.SystemUUID, diff)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set BMC settings: %w", err)
	}

	// move status to completed
	if err := r.updateServerBIOSStatus(ctx, log, serverBIOS, metalv1alpha1.BIOSMaintenanceStateCompleted); err != nil {
		return ctrl.Result{}, err
	}

	// completed the BIOS tasks
	return ctrl.Result{}, nil
}

func (r *ServerBIOSReconciler) handleCompletedState(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, server *metalv1alpha1.Server) (ctrl.Result, error) {
	// todo: may be we need to reschedule it to be able to periodically check the bios setting?
	log.V(1).Info("Done with bios setting update", "ctx", ctx, "serverBIOS", serverBIOS, "server", server)
	return ctrl.Result{Requeue: false}, nil
}

func (r *ServerBIOSReconciler) handleFailedState(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, server *metalv1alpha1.Server) (ctrl.Result, error) {
	// todo: may be we need to put the server in maintenance to be able to get it to right state
	log.V(1).Info("Done with bios setting update", "ctx", ctx, "serverBIOS", serverBIOS, "server", server)
	return ctrl.Result{Requeue: true}, nil
}

func (r *ServerBIOSReconciler) waitOnMaintenanceRequest(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, server *metalv1alpha1.Server) error {
	var waitForMaintenance bool
	switch serverBIOS.Status.State {
	case metalv1alpha1.BIOSMaintenanceStateInSettingUpdate:
		if serverBIOS.Spec.ServerMaintenanceRef == nil {
			// if no maintenance req, nothing to do
			return nil
		} else {
			waitForMaintenance = true
		}
	case metalv1alpha1.BIOSMaintenanceStateInVersionUpgrade:
		if serverBIOS.Spec.ServerMaintenanceRef == nil {
			return nil
		} else {
			waitForMaintenance = true
		}
	}

	if waitForMaintenance {
		serverMaintenance, err := r.getReferredServerMaintenance(ctx, log, serverBIOS.Spec.ServerMaintenanceRef)
		if err != nil {
			return fmt.Errorf("unable to get the referred server maintenance")
		}

		if serverMaintenance.Status.State == metalv1alpha1.ServerMaintenanceStateInMaintenance {
			if server.Spec.ServerMaintenanceRef == nil {
				// server in maintenance for other tasks.
				log.V(1).Info("Server does not have maintenance reference", "Server", server.Name, "Maintenance", server.Spec.ServerMaintenanceRef)
				return fmt.Errorf("server missing maintenance reference")
			} else if server.Spec.ServerMaintenanceRef.UID != serverBIOS.Spec.ServerMaintenanceRef.UID {
				// server in maintenance for other tasks.
				log.V(1).Info("Server is already in maintenance for other tasks", "Server", server.Name, "Maintenance", server.Spec.ServerMaintenanceRef.Name)
				return fmt.Errorf("server is already in maintenance by other tasks")
			} else {
				if server.Spec.Power == metalv1alpha1.PowerOn {
					// server in Maintenance for us and restarted
					// note, restart happens in two step here
					// maintenance is scheduled with power off.
					// if its power on state, we assume the server to be restared.
					// todo: give ability to restart the power.
					return nil
				} else if server.Spec.Power == metalv1alpha1.PowerOff {
					err := r.ensurePowerStateForServer(ctx, log, metalv1alpha1.PowerOn, server)
					if err != nil {
						return err
					}
					return fmt.Errorf("waiting for reboot to complete")
				} else {
					// server in maintenance, but the power state in not as expected.
					// todo: may be we reboot the system manually?
					return fmt.Errorf("server in maintenance but unknown power state")
				}
			}
		} else {
			// we still need to wait for server to enter maintenance
			return fmt.Errorf("requested maintenance has not yet been granted")
		}
	}

	return nil
}

func (r *ServerBIOSReconciler) ensurePowerStateForServer(ctx context.Context, log logr.Logger, power metalv1alpha1.Power, server *metalv1alpha1.Server) error {

	serverBase := server.DeepCopy()
	server.Spec.Power = power
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch power state for server: %w", err)
	}

	log.V(1).Info("Patched desired Power of the claimed Server", "Server", server.Name, "state", power)
	return nil
}

func (r *ServerBIOSReconciler) requestMaintenanceOnServer(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, server *metalv1alpha1.Server) error {

	// if Server maintenance ref is already given. no further action required.
	if serverBIOS.Spec.ServerMaintenanceRef != nil {
		return nil
	}

	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	serverMaintenance.Name = serverBIOS.Name
	serverMaintenance.Namespace = r.ManagerNamespace
	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, serverMaintenance, func() error {
		serverMaintenance.Spec.Policy = serverBIOS.Spec.ServerMaintenancePolicy
		serverMaintenance.Spec.ServerPower = metalv1alpha1.PowerOff
		serverMaintenance.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}

		return controllerutil.SetControllerReference(serverBIOS, serverMaintenance, r.Client.Scheme())
	})
	if err != nil {
		return fmt.Errorf("failed to create or patch serverMaintenance: %w", err)
	}
	log.V(1).Info("Created serverMaintenance", "serverMaintenance", serverMaintenance.Name, "Operation", opResult)

	err = r.patchMaintenanceRequestRefOnServerBIOS(ctx, log, serverBIOS, serverMaintenance)

	if err != nil {
		return fmt.Errorf("failed to patch serverMaintenance ref in serverBIOS status: %w", err)
	}

	return nil
}

func (r *ServerBIOSReconciler) getReferredServer(
	ctx context.Context,
	log logr.Logger,
	serverRef *corev1.LocalObjectReference,
) (metalv1alpha1.Server, error) {
	key := client.ObjectKey{Name: serverRef.Name}
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, key, server); err != nil {
		log.V(1).Error(err, "failed to get referred server")
		return *server, err
	}
	return *server, nil
}

func (r *ServerBIOSReconciler) getReferredServerMaintenance(
	ctx context.Context,
	log logr.Logger,
	serverMaintenanceRef *corev1.ObjectReference,
) (metalv1alpha1.ServerMaintenance, error) {
	key := client.ObjectKey{Name: serverMaintenanceRef.Name, Namespace: r.ManagerNamespace}
	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	if err := r.Get(ctx, key, serverMaintenance); err != nil {
		log.V(1).Error(err, "failed to get referred serverMaintenance obj")
		return *serverMaintenance, err
	}
	return *serverMaintenance, nil
}

func (r *ServerBIOSReconciler) getReferredserverBIOS(
	ctx context.Context,
	log logr.Logger,
	referredBIOSSetteingRef *corev1.LocalObjectReference,
) (metalv1alpha1.ServerBIOS, error) {
	key := client.ObjectKey{Name: referredBIOSSetteingRef.Name, Namespace: metav1.NamespaceNone}
	serverBIOS := metalv1alpha1.ServerBIOS{}
	if err := r.Get(ctx, key, &serverBIOS); err != nil {
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
) error {
	if server.Spec.BIOSSettingsRef == serverBIOSReference {
		return nil
	}

	var err error
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
		serverBIOS.Spec.ServerMaintenanceRef = &corev1.ObjectReference{}
	} else {
		serverBIOS.Spec.ServerMaintenanceRef = &corev1.ObjectReference{
			APIVersion: "metal.ironcore.dev/v1alpha1",
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

func (r *ServerBIOSReconciler) updateServerBIOSStatus(ctx context.Context, log logr.Logger, serverBIOS *metalv1alpha1.ServerBIOS, state metalv1alpha1.BIOSMaintenanceState) error {
	serverBIOSbase := serverBIOS.DeepCopy()
	serverBIOS.Status.State = state

	if err := r.Status().Patch(ctx, serverBIOS, client.MergeFrom(serverBIOSbase)); err != nil {
		return fmt.Errorf("failed to patch Server status: %w", err)
	}

	log.V(1).Info("Updated serverBIOS state ", "new state", state)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBIOSReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBIOS{}).
		Complete(r)
}
