// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/redfish"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ServerMaintenanceFinalizer is the finalizer for the ServerMaintenance resource.
	ServerMaintenanceFinalizer = "metal.ironcore.dev/servermaintenance"
)

// ServerMaintenanceReconciler reconciles a ServerMaintenance object
type ServerMaintenanceReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Insecure       bool
	ResyncInterval time.Duration
	BMCOptions     bmc.Options
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServerMaintenance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *ServerMaintenanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	if err := r.Get(ctx, req.NamespacedName, serverMaintenance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, log, serverMaintenance)
}

func (r *ServerMaintenanceReconciler) reconcileExists(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	if !serverMaintenance.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, serverMaintenance)
	}
	return r.reconcile(ctx, log, serverMaintenance)
}

func (r *ServerMaintenanceReconciler) reconcile(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(serverMaintenance) {
		log.V(1).Info("Skipped ServerMaintenance reconciliation")
		return ctrl.Result{}, nil
	}

	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: serverMaintenance.Spec.ServerRef.Name}, server); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get server: %w", err)

	}
	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, serverMaintenance, ServerMaintenanceFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	// set the servermaintenance state to pending if it is not set
	if serverMaintenance.Status.State == "" {
		if modified, err := r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStatePending); err != nil || modified {
			return ctrl.Result{}, err
		}
	}
	return r.ensureServerMaintenanceStateTransition(ctx, log, serverMaintenance)
}

func (r *ServerMaintenanceReconciler) ensureServerMaintenanceStateTransition(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	switch serverMaintenance.Status.State {
	case metalv1alpha1.ServerMaintenanceStatePending:
		return r.handlePendingState(ctx, log, serverMaintenance)
	case metalv1alpha1.ServerMaintenanceStatePreapareMaintenance:
		return r.handlePrepareMaintenanceState(ctx, log, serverMaintenance)
	case metalv1alpha1.ServerMaintenanceStateInMaintenance:
		log.V(1).Info("Server is under maintenance", "ServerMaintenance", serverMaintenance.Name)
		server, err := r.getServerRef(ctx, serverMaintenance)
		if err != nil {
			return ctrl.Result{}, err
		}

		if changed, _, err := r.isBootConfigurationChanged(ctx, log, serverMaintenance); err != nil {
			return ctrl.Result{}, err
		} else if changed {
			return r.moveToPrepareMaintenanceState(ctx, log, serverMaintenance, server)
		}
		err = r.setAndPatchServerPowerState(ctx, log, server, serverMaintenance.Spec.ServerPower)
		return ctrl.Result{}, err
	case metalv1alpha1.ServerMaintenanceStateFailed:
		return r.handleFailedState(log, serverMaintenance)
	}
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) handlePendingState(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (result ctrl.Result, err error) {
	server, err := r.getServerRef(ctx, serverMaintenance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if server.Spec.ServerMaintenanceRef != nil {
		if server.Spec.ServerMaintenanceRef.UID != serverMaintenance.UID {
			log.V(1).Info("Server is already in maintenance", "Server", server.Name, "Maintenance", server.Spec.ServerMaintenanceRef.Name)
			return ctrl.Result{}, nil
		}
	}
	if server.Spec.ServerClaimRef == nil {
		log.V(1).Info("Server has no claim, move to maintenance right away", "Server", server.Name)
		return r.moveToPrepareMaintenanceState(ctx, log, serverMaintenance, server)
	}
	serverClaim := &metalv1alpha1.ServerClaim{}
	if err := r.Get(ctx,
		client.ObjectKey{
			Name:      server.Spec.ServerClaimRef.Name,
			Namespace: server.Spec.ServerClaimRef.Namespace,
		}, serverClaim); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get server claim: %w", err)
		}
		log.V(1).Info("ServerClaim gone")
		return ctrl.Result{}, nil
	}
	claimAnnotations := map[string]string{
		metalv1alpha1.ServerMaintenanceNeededLabelKey: "true",
	}
	if serverMaintenance.Annotations[metalv1alpha1.ServerMaintenanceReasonAnnotationKey] != "" {
		claimAnnotations[metalv1alpha1.ServerMaintenanceReasonAnnotationKey] = serverMaintenance.Annotations[metalv1alpha1.ServerMaintenanceReasonAnnotationKey]
	}
	if err := r.patchServerClaimAnnotation(ctx, log, serverClaim, claimAnnotations); err != nil {
		return ctrl.Result{}, err
	}
	if serverMaintenance.Spec.Policy == metalv1alpha1.ServerMaintenancePolicyOwnerApproval {
		claimAnnotations := serverClaim.GetAnnotations()
		if _, ok := claimAnnotations[metalv1alpha1.ServerMaintenanceApprovalKey]; !ok {
			log.V(1).Info("Server not approved for maintenance, waiting for approval", "Server", server.Name)
			return ctrl.Result{}, nil
		}
		log.V(1).Info("Server approved for maintenance", "Server", server.Name, "Maintenance", serverMaintenance.Name)
		return r.moveToPrepareMaintenanceState(ctx, log, serverMaintenance, server)
	}
	if serverMaintenance.Spec.Policy == metalv1alpha1.ServerMaintenancePolicyEnforced {
		log.V(1).Info("Enforcing maintenance", "Server", server.Name, "Maintenance", serverMaintenance.Name)
		return r.moveToPrepareMaintenanceState(ctx, log, serverMaintenance, server)
	}
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) handlePrepareMaintenanceState(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	server, err := r.getServerRef(ctx, serverMaintenance)
	if err != nil {
		return ctrl.Result{}, err
	}

	// rest of the operation can be done only if the server is in maintenance state
	// else, we will hit conflict with ServerClaim controller which will still be asserting Specs on Serevr
	if server.Status.State != metalv1alpha1.ServerStateMaintenance {
		log.V(1).Info("Waiting for server to be in maintenance state", "Server", server.Name, "CurrentState", server.Status.State)
		return ctrl.Result{}, nil
	}

	var config *metalv1alpha1.ServerBootConfiguration
	changed := false
	if changed, config, err = r.isBootConfigurationChanged(ctx, log, serverMaintenance); err != nil {
		return ctrl.Result{}, err
	} else if changed || config == nil {
		// turn server power off before applying new boot configuration
		// this will help schedule the job to change the boot order for the server
		// note: if the server is already off, this will be a no-op
		if server.Spec.Power != metalv1alpha1.PowerOff && server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
			if err := r.setAndPatchServerPowerState(ctx, log, server, metalv1alpha1.PowerOff); err != nil {
				return ctrl.Result{}, err
			}
			// requeue to give time to the server to power off
			return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
		}
		// if no config found or if it has changed, we need to create or path it
		config, err = r.applyServerBootConfiguration(ctx, log, serverMaintenance, server)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Note: this is to check skip of BootConfiguration
	// implication of skipping is that the server will still continue to run the OS during Maintenance
	// might be subjected to reboots
	val, found := serverMaintenance.GetAnnotations()[metalv1alpha1.OperationSkipBootConfiguration]
	if config == nil && (!found || val != metalv1alpha1.OperationBootConfigurationSkip) {
		log.V(1).Info("No ServerBootConfigurationTemplate boot configuration", "Server", server.Name)
		_, err = r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStateFailed)
		return ctrl.Result{}, err
	}

	if config != nil {
		if config.Status.State == metalv1alpha1.ServerBootConfigurationStatePending || config.Status.State == "" {
			log.V(1).Info("Server boot configuration is pending", "Server", server.Name)
			return ctrl.Result{}, nil
		}
		if config.Status.State == metalv1alpha1.ServerBootConfigurationStateError {
			if modified, err := r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStateFailed); err != nil || modified {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if config.Status.State == metalv1alpha1.ServerBootConfigurationStateReady {
			log.V(1).Info("Server maintenance boot configuration is ready", "Server", server.Name)
			// now we change the boot order to the server and power it on to complete the configuration
			if requeue, err := r.configureBootOrder(ctx, log, server, serverMaintenance); err != nil {
				return ctrl.Result{}, err
			} else if requeue {
				return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
			}
		}
	}
	if server.Spec.Power != serverMaintenance.Spec.ServerPower {
		if err := r.setAndPatchServerPowerState(ctx, log, server, serverMaintenance.Spec.ServerPower); err != nil {
			return ctrl.Result{}, err
		}
	}
	_, err = r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStateInMaintenance)
	return ctrl.Result{}, err
}

func (r *ServerMaintenanceReconciler) applyServerBootConfiguration(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance, server *metalv1alpha1.Server) (*metalv1alpha1.ServerBootConfiguration, error) {
	if maintenance.Spec.ServerBootConfigurationTemplate == nil {
		log.V(1).Info("No ServerBootConfigurationTemplate specified")
		return nil, nil
	}

	log.V(1).Info("Creating/Patching server maintenance boot configuration", "Server", server.Name)
	config := &metalv1alpha1.ServerBootConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      maintenance.Name,
			Namespace: maintenance.Namespace,
		},
	}
	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, config, func() error {
		config.Spec = maintenance.Spec.ServerBootConfigurationTemplate.Spec
		config.Status.State = ""
		return controllerutil.SetControllerReference(maintenance, config, r.Scheme)
	})
	if err != nil {
		return config, fmt.Errorf("failed to create server boot configuration: %w", err)
	}
	log.V(1).Info("Created or patched Config", "Config", config.Name, "Operation", opResult)
	serverBase := server.DeepCopy()
	server.Spec.BootConfigurationRef = &v1.ObjectReference{
		Namespace:  config.Namespace,
		Name:       config.Name,
		UID:        config.UID,
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerBootConfiguration",
	}
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return config, fmt.Errorf("failed to patch server maintenance boot configuration ref: %w", err)
	}
	return config, nil
}

func (r *ServerMaintenanceReconciler) setAndPatchServerPowerState(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server, power metalv1alpha1.Power) error {
	if server.Spec.Power == power {
		return nil
	}
	serverBase := server.DeepCopy()
	server.Spec.Power = power
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch server power state: %w", err)
	}
	log.V(1).Info("Patched server power state", "Server", server.Name, "Power", server.Spec.Power)

	return nil
}

func (r *ServerMaintenanceReconciler) updateServerRef(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance, server *metalv1alpha1.Server) error {
	if server.Spec.ServerMaintenanceRef != nil {
		log.V(1).Info("Server is already in Maintenance", "Server", server.Name, "Maintenance", server.Spec.ServerMaintenanceRef.Name)
		return nil
	}
	server.Spec.ServerMaintenanceRef = &v1.ObjectReference{
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerMaintenance",
		Namespace:  maintenance.Namespace,
		Name:       maintenance.Name,
		UID:        maintenance.UID,
	}
	// use update to not overwrite ServerMaintenanceRef if another maintenance was quicker
	if err := r.Update(ctx, server); err != nil {
		return fmt.Errorf("failed to patch maintenance ref for server: %w", err)
	}
	log.V(1).Info("Updated ServerMaintenance reference on Server", "Server", server.Name, "ServerMaintenanceeRef", maintenance.Name)

	return nil
}

func (r *ServerMaintenanceReconciler) handleFailedState(log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	log.V(1).Info("ServerMaintenance failed", "ServerMaintenance", serverMaintenance.Name)
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) delete(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	if serverMaintenance.Spec.ServerRef == nil {
		return ctrl.Result{}, nil
	}
	server, err := r.getServerRef(ctx, serverMaintenance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue, err := r.revertMaintenanceBootConfig(ctx, log, server, serverMaintenance); err != nil {
		return ctrl.Result{}, err
	} else if requeue {
		log.V(1).Info("Wait for boot order to be reverted on server", "Server", server.Name)
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
	}
	log.V(1).Info("Boot order reverted on server", "Server", server.Name)
	// make sure the server is powered on before removing maintenance
	// this will ensure the server is booted into default boot order
	if server.Spec.Power != metalv1alpha1.PowerOn && server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
		if err := r.setAndPatchServerPowerState(ctx, log, server, metalv1alpha1.PowerOn); err != nil {
			return ctrl.Result{}, err
		}
		// requeue to give time to the server to power off
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
	}

	if err := r.cleanup(ctx, log, server, serverMaintenance); err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, serverMaintenance, ServerMaintenanceFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) getServerRef(ctx context.Context, serverMaintenance *metalv1alpha1.ServerMaintenance) (*metalv1alpha1.Server, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: serverMaintenance.Spec.ServerRef.Name}, server); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get server: %w", err)
		}
		return nil, fmt.Errorf("server not found")
	}
	return server, nil
}

func (r *ServerMaintenanceReconciler) cleanup(
	ctx context.Context,
	log logr.Logger,
	server *metalv1alpha1.Server,
	serverMaintenance *metalv1alpha1.ServerMaintenance) error {
	if server != nil && server.Spec.ServerMaintenanceRef != nil {
		if err := r.removeMaintenanceRefFromServer(ctx, server); err != nil {
			log.Error(err, "failed to remove maintenance ref from server")
		}
	}
	if server.Spec.BootConfigurationRef != nil &&
		serverMaintenance.Spec.ServerBootConfigurationTemplate != nil {
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serverMaintenance.Name,
				Namespace: serverMaintenance.Namespace,
			},
		}
		if err := r.Delete(ctx, config); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete serverbootconfig: %w", err)
			}
		}
		// note: remove the bootConfig this controller set
		if err := r.removeBootConfigRefFromServer(ctx, log, config, server); err != nil {
			return fmt.Errorf("failed to remove maintenance boot config ref from server: %w", err)
		}
	}

	if server.Spec.ServerClaimRef == nil {
		return nil
	}
	serverClaim := &metalv1alpha1.ServerClaim{}
	if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.ServerClaimRef.Name, Namespace: server.Spec.ServerClaimRef.Namespace}, serverClaim); err != nil {
		return fmt.Errorf("failed to get server claim: %w", err)
	}
	serverClaimBase := serverClaim.DeepCopy()
	metautils.DeleteAnnotations(serverClaim, []string{
		metalv1alpha1.ServerMaintenanceApprovalKey,
		metalv1alpha1.ServerMaintenanceNeededLabelKey,
		metalv1alpha1.ServerMaintenanceReasonAnnotationKey,
	})
	if err := r.Patch(ctx, serverClaim, client.MergeFrom(serverClaimBase)); err != nil {
		return fmt.Errorf("failed to patch server claim annotations: %w", err)
	}
	return nil
}

func (r *ServerMaintenanceReconciler) moveToPrepareMaintenanceState(
	ctx context.Context,
	log logr.Logger,
	serverMaintenance *metalv1alpha1.ServerMaintenance,
	server *metalv1alpha1.Server,
) (ctrl.Result, error) {
	if err := r.updateServerRef(ctx, log, serverMaintenance, server); err != nil {
		return ctrl.Result{}, err
	}

	if modified, err := r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStatePreapareMaintenance); err != nil || modified {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) configureBootOrder(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server, maintenance *metalv1alpha1.ServerMaintenance) (bool, error) {
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		return false, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	switch maintenance.Spec.ServerBootConfigurationTemplate.Boottype {
	case metalv1alpha1.BootTypeOneOff:
		// note: with this option, the next server boot up will enter pxe boot.
		// if Maintenance Spec.ServerPower is PowerOn, server will be powered on and will boot into pxe
		// if Maintenance Spec.ServerPower is PowerOff, server will be powered off and will boot into pxe on next power on
		// we do not need to save the default boot order as the server will boot into default boot order on next boot up
		if err := bmcClient.SetPXEBootOnce(ctx, server.Spec.SystemURI); err != nil {
			return false, fmt.Errorf("failed to set PXE boot once: %w", err)
		}
		log.V(1).Info("Configured PXE boot once for server",
			"Server", server.Name)
		status := &metalv1alpha1.ServerMaintenanceBootOrder{
			State: metalv1alpha1.BootOrderOneOffPxeBootSuccess,
		}
		if err := r.patchBootOrderStatus(ctx, log, maintenance, status); err != nil {
			return false, fmt.Errorf("failed to patch maintenance default boot order: %w", err)
		}
		// end state, continue with maintenance
		return false, nil
	case metalv1alpha1.BootTypePersistent:
		bootOrder, err := bmcClient.GetBootOrder(ctx, server.Spec.SystemURI)
		if err != nil {
			return false, fmt.Errorf("failed to get boot order: %w", err)
		}
		// if the boot order length is less than 2, we cannot change pxe as first boot
		if len(bootOrder) < 2 {
			log.V(1).Info("boot order for server can not be changed as it has less than 2 boot options", "bootOrder", bootOrder)
			status := &metalv1alpha1.ServerMaintenanceBootOrder{
				State: metalv1alpha1.BootOrderConfigNoOp,
			}
			if err := r.patchBootOrderStatus(ctx, log, maintenance, status); err != nil {
				return false, fmt.Errorf("failed to patch maintenance default boot order: %w", err)
			}
			// end state, continue with maintenance
			return false, nil
		}
		bootOptions, err := bmcClient.GetBootOptions(ctx, server.Spec.SystemURI)
		if err != nil {
			return false, fmt.Errorf("failed to get boot options: %w", err)
		}

		var isDiskBootOption = func(bootOption *redfish.BootOption) bool {
			diskIndicators := []string{"/HD(", "/Sata(", "/NVMe(", "/Scsi(", "/USB("}
			for _, indicator := range diskIndicators {
				if strings.Contains(bootOption.UefiDevicePath, indicator) {
					return true
				}
			}
			return false
		}

		var isPxeBootOption = func(bootOption *redfish.BootOption) bool {
			return strings.Contains(strings.ToLower(bootOption.DisplayName), "pxe")
		}

		bootOrderChanged := rearrangeBootOrder(bootOrder, bootOptions, isPxeBootOption, isDiskBootOption)

		if reflect.DeepEqual(bootOrder, bootOrderChanged) {
			// boot order is already set to pxe first, nothing to do
			log.V(1).Info("boot order for server is set to pxe first",
				"Server", server.Name,
				"BootOrder", bootOrder)
			status := &metalv1alpha1.ServerMaintenanceBootOrder{}
			if maintenance.Status.BootOrderStatus == nil {
				status = &metalv1alpha1.ServerMaintenanceBootOrder{
					State: metalv1alpha1.BootOrderConfigNoOp,
				}
			} else {
				status = maintenance.Status.BootOrderStatus.DeepCopy()
				status.State = metalv1alpha1.BootOrderConfigSuccess
			}
			if err := r.patchBootOrderStatus(ctx, log, maintenance, status); err != nil {
				return false, fmt.Errorf("failed to patch maintenance default boot order: %w", err)
			}
			// end state, continue with maintenance
			return false, nil
		}

		if maintenance.Status.BootOrderStatus == nil {
			// note in this option, the server will always boot into pxe until changed again
			// the job needs to be completed, else any other BIOS config job triggered will fail as exsisting job is not completed
			if err := bmcClient.SetBootOrder(ctx, server.Spec.SystemURI, redfish.Boot{BootOrder: bootOrderChanged}); err != nil {
				return false, fmt.Errorf("failed to set PXE boot once: %w", err)
			}
			log.V(1).Info("Configured boot order for server",
				"Server", server.Name,
				"BootType", maintenance.Spec.ServerBootConfigurationTemplate.Boottype,
				"BootOrder", bootOrderChanged)
			// save the current boot order to revert back after maintenance
			status := &metalv1alpha1.ServerMaintenanceBootOrder{
				DefaultBootOrder: bootOrder,
				State:            metalv1alpha1.BootOrderConfigInProgress,
			}
			if err := r.patchBootOrderStatus(ctx, log, maintenance, status); err != nil {
				return false, fmt.Errorf("failed to patch maintenance default boot order: %w", err)
			}
			log.V(1).Info("Saved default boot order for server to revert post maintenance", "Server", server.Name, "BootOrder", bootOrder)
			// requeue to give time to the server to actually do the work and verify it
			return true, nil
		}
		// we need to complete the job to set the boot order correctly. Turn the server power on
		if server.Spec.Power != metalv1alpha1.PowerOn && server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
			if err := r.setAndPatchServerPowerState(ctx, log, server, metalv1alpha1.PowerOn); err != nil {
				return false, err
			}
			log.V(1).Info("Requested server power On to complete boot order change",
				"Server", server.Name)
			// requeue to give time to the server to power off
			return true, nil
		}
		log.V(1).Info("wait for boot order to be set on server", "Server", server.Name)
		return true, nil
	default:
		return false, fmt.Errorf("unknown boot type: %s", maintenance.Spec.ServerBootConfigurationTemplate.Boottype)
	}
}

func (r *ServerMaintenanceReconciler) revertMaintenanceBootConfig(
	ctx context.Context,
	log logr.Logger,
	server *metalv1alpha1.Server,
	serverMaintenance *metalv1alpha1.ServerMaintenance,
) (bool, error) {
	// we need to revert the boot order only if it was changed by the maintenance
	if serverMaintenance.Status.BootOrderStatus != nil && serverMaintenance.Status.BootOrderStatus.DefaultBootOrder != nil {
		bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
		if err != nil {
			return false, fmt.Errorf("failed to create BMC client: %w", err)
		}
		defer bmcClient.Logout()
		bootOrder, err := bmcClient.GetBootOrder(ctx, server.Spec.SystemURI)
		if err != nil {
			return false, fmt.Errorf("failed to get boot order: %w", err)
		}
		if serverMaintenance.Status.BootOrderStatus.State == metalv1alpha1.BootOrderConfigSuccess {
			// if the server is not off, we need to turn it off first to change the boot order
			// this will help schedule the job to change the boot order for the server
			// note: if the server is already off, this will be a no-op
			if server.Spec.Power != metalv1alpha1.PowerOff || server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
				if err := r.setAndPatchServerPowerState(ctx, log, server, metalv1alpha1.PowerOff); err != nil {
					return false, err
				}
				// requeue to give time to the server to power off
				return true, nil
			}

			// sanitize the boot order to remove any invalid boot options
			// this case can happen if the bios settings were changed boot options during maintenance
			// we remove only the invalid boot options and keep the rest in same order
			// because the settings will take care of new added boot device and apend it to end
			currentBootDeviceMap := make(map[string]struct{})
			for _, bootDevice := range bootOrder {
				currentBootDeviceMap[bootDevice] = struct{}{}
			}
			var sanitizedBootOrder []string
			for _, bootDevice := range serverMaintenance.Status.BootOrderStatus.DefaultBootOrder {
				if _, ok := currentBootDeviceMap[bootDevice]; !ok {
					log.V(1).Info("Removing invalid boot option from default boot order",
						"Server", server.Name,
						"BootDevice", bootDevice)
					continue
				}
				sanitizedBootOrder = append(sanitizedBootOrder, bootDevice)
			}

			err = bmcClient.SetBootOrder(ctx, server.Spec.SystemURI, redfish.Boot{
				BootOrder: sanitizedBootOrder,
			})
			if err != nil {
				return false, fmt.Errorf("failed to revert boot order: %w", err)
			}
			// save the current boot order to revert back after maintenance
			status := &metalv1alpha1.ServerMaintenanceBootOrder{
				DefaultBootOrder: serverMaintenance.Status.BootOrderStatus.DefaultBootOrder,
				State:            metalv1alpha1.BootOrderConfigSuccessRevertInProgress,
			}
			if err := r.patchBootOrderStatus(ctx, log, serverMaintenance, status); err != nil {
				return false, fmt.Errorf("failed to patch maintenance default boot order: %w", err)
			}
			log.V(1).Info("Patched revert to default boot order", "Server", server.Name, "BootOrder", nil)
			// requeue to give time to the server to actually do the work and verify it
			return true, nil
		}

		if serverMaintenance.Status.BootOrderStatus.State == metalv1alpha1.BootOrderConfigSuccessRevertInProgress {
			if server.Spec.Power != metalv1alpha1.PowerOn || server.Status.PowerState != metalv1alpha1.ServerOnPowerState {
				if err := r.setAndPatchServerPowerState(ctx, log, server, metalv1alpha1.PowerOn); err != nil {
					return false, err
				}
				// requeue to give time to the server to power on
				return true, nil
			}

			revertCompleted := false
			if reflect.DeepEqual(bootOrder, serverMaintenance.Status.BootOrderStatus.DefaultBootOrder) {
				revertCompleted = true
			} else {
				// sometimes, changing biosSettings leads to change in number of boot options
				// hence, we need check the default boot order and ignore extra from current boot order
				for idx, bootDevice := range serverMaintenance.Status.BootOrderStatus.DefaultBootOrder {
					// cases where boot option is disabled in bios settings
					if idx >= len(bootOrder) {
						break
					}
					if bootDevice != bootOrder[idx] {
						log.V(1).Info("boot order for server is not yet reverted to default. Waiting...",
							"Server", server.Name,
							"CurrentBootOrder", bootOrder,
							"DefaultBootOrder", serverMaintenance.Status.BootOrderStatus.DefaultBootOrder)
						return true, nil
					}
				}
				revertCompleted = true
			}
			if revertCompleted {
				log.V(1).Info("boot order for server has been reverted to default",
					"Server", server.Name,
					"BootOrder", bootOrder)
				if err := r.patchBootOrderStatus(ctx, log, serverMaintenance, nil); err != nil {
					return false, fmt.Errorf("failed to patch maintenance default boot order: %w", err)
				}
				// end state, continue with maintenance complete
				return false, nil
			}
			log.V(1).Info("wait for boot order to be reverted on server", "Server", server.Name)
			return true, nil
		}
	}

	log.V(1).Info("boot override during maintenance is reverted", "Server", serverMaintenance.Name)
	return false, nil
}

func (r *ServerMaintenanceReconciler) removeBootConfigRefFromServer(
	ctx context.Context,
	log logr.Logger,
	config *metalv1alpha1.ServerBootConfiguration,
	server *metalv1alpha1.Server,
) error {
	if server == nil {
		return nil
	}
	if ref := server.Spec.BootConfigurationRef; ref == nil || (ref.Name != config.Name && ref.Namespace != config.Namespace) {
		return nil
	}
	serverBase := server.DeepCopy()
	// remove the boot configuration ref by the maintenance only
	// this will be replaced by the reserved boot configuration if provided, by serverClaim
	server.Spec.BootConfigurationRef = nil
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	log.V(1).Info("Removed maintenance boot configuration ref from server", "Server", server.Name)
	return nil
}

func (r *ServerMaintenanceReconciler) patchBootOrderStatus(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance, bootOrderStatus *metalv1alpha1.ServerMaintenanceBootOrder) error {
	maintenanceBase := maintenance.DeepCopy()
	maintenance.Status.BootOrderStatus = bootOrderStatus
	if err := r.Status().Patch(ctx, maintenance, client.MergeFrom(maintenanceBase)); err != nil {
		return fmt.Errorf("failed to patch maintenance default boot order: %w", err)
	}
	log.V(1).Info("Patched maintenance boot order status", "ServerMaintenance", maintenance.Name, "BootOrderStatus", bootOrderStatus)
	return nil
}

func (r *ServerMaintenanceReconciler) removeMaintenanceRefFromServer(ctx context.Context, server *metalv1alpha1.Server) error {
	serverBase := server.DeepCopy()
	server.Spec.ServerMaintenanceRef = nil
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch claim ref for server: %w", err)
	}
	return nil
}

func (r *ServerMaintenanceReconciler) patchMaintenanceState(ctx context.Context, serverMaintenance *metalv1alpha1.ServerMaintenance, state metalv1alpha1.ServerMaintenanceState) (bool, error) {
	if serverMaintenance.Status.State == state {
		return false, nil
	}
	base := serverMaintenance.DeepCopy()
	serverMaintenance.Status.State = state
	if err := r.Status().Patch(ctx, serverMaintenance, client.MergeFrom(base)); err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance state: %w", err)
	}
	return true, nil
}

func (r *ServerMaintenanceReconciler) patchServerClaimAnnotation(ctx context.Context, log logr.Logger, serverClaim *metalv1alpha1.ServerClaim, set map[string]string) error {
	anno := serverClaim.GetAnnotations()
	change := false
	for k, v := range set {
		if anno[k] != v {
			change = true
			break
		}
	}
	if !change {
		return nil
	}
	metautils.SetAnnotations(serverClaim, set)
	if err := r.Update(ctx, serverClaim); err != nil {
		return fmt.Errorf("failed to update serverclaim annotations: %w", err)
	}
	log.V(1).Info("Updated server claim annotations", "ServerClaim", serverClaim.Name, "Annotations", set)
	return nil
}

func (r *ServerMaintenanceReconciler) getMaintenanceBootConfigurationRef(
	ctx context.Context,
	log logr.Logger,
	serverMaintenance *metalv1alpha1.ServerMaintenance,
) (*metalv1alpha1.ServerBootConfiguration, error) {
	if serverMaintenance.Spec.ServerBootConfigurationTemplate == nil {
		return nil, nil
	}
	config := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, types.NamespacedName{Name: serverMaintenance.Name, Namespace: serverMaintenance.Namespace}, config); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("ServerBootConfiguration for maintenance not found", "ServerBootConfiguration", serverMaintenance.Name)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get server boot configuration: %w", err)
	}
	return config, nil
}

func (r *ServerMaintenanceReconciler) isBootConfigurationChanged(
	ctx context.Context,
	log logr.Logger,
	serverMaintenance *metalv1alpha1.ServerMaintenance,
) (bool, *metalv1alpha1.ServerBootConfiguration, error) {
	config, err := r.getMaintenanceBootConfigurationRef(ctx, log, serverMaintenance)
	if err != nil {
		return false, config, err
	}
	if config == nil {
		return false, config, nil
	}
	if reflect.DeepEqual(config.Spec, serverMaintenance.Spec.ServerBootConfigurationTemplate.Spec) {
		// no changes
		log.V(1).Info("No changes in ServerBootConfigurationTemplate")
		return false, config, nil
	}
	return true, config, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerMaintenanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerMaintenance{}).
		Owns(&metalv1alpha1.ServerBootConfiguration{}).
		Watches(&metalv1alpha1.Server{}, r.enqueueMaintenanceByServerRefs()).
		Watches(&metalv1alpha1.ServerClaim{}, r.enqueueMaintenanceByClaimRefs()).
		Complete(r)
}

func (r *ServerMaintenanceReconciler) enqueueMaintenanceByServerRefs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)
		server := object.(*metalv1alpha1.Server)
		var req []reconcile.Request

		if server.Spec.ServerMaintenanceRef != nil {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{Namespace: server.Spec.ServerMaintenanceRef.Namespace, Name: server.Spec.ServerMaintenanceRef.Name},
			}}
		}

		maintenanceList := &metalv1alpha1.ServerMaintenanceList{}
		if err := r.List(ctx, maintenanceList); err != nil {
			log.Error(err, "failed to list host serverMaintenances")
			return nil
		}
		for _, maintenance := range maintenanceList.Items {
			if server.Spec.ServerMaintenanceRef != nil && maintenance.Name == server.Spec.ServerMaintenanceRef.Name {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: maintenance.Namespace, Name: maintenance.Name},
				})
				return req
			}
			if maintenance.Spec.ServerRef != nil && maintenance.Spec.ServerRef.Name == server.Name {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: maintenance.Namespace, Name: maintenance.Name},
				})
			}
		}

		return req
	})
}

func (r *ServerMaintenanceReconciler) enqueueMaintenanceByClaimRefs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)
		claim := object.(*metalv1alpha1.ServerClaim)
		var req []reconcile.Request
		annotations := claim.GetAnnotations()
		if _, ok := annotations[metalv1alpha1.ServerMaintenanceNeededLabelKey]; !ok {
			return req
		}

		maintenanceList := &metalv1alpha1.ServerMaintenanceList{}
		if err := r.List(ctx, maintenanceList); err != nil {
			log.Error(err, "failed to list host serverMaintenances")
			return nil
		}
		for _, maintenance := range maintenanceList.Items {
			if maintenance.Spec.ServerRef != nil && maintenance.Spec.ServerRef.Name == claim.Spec.ServerRef.Name {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: maintenance.Namespace, Name: maintenance.Name},
				})
				return req
			}
			if maintenance.Spec.ServerRef == nil {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: maintenance.Namespace, Name: maintenance.Name},
				})
				return req
			}
		}
		return req
	})
}
