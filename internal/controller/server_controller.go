/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/afritzler/metal-operator/internal/api/registry"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	"github.com/afritzler/metal-operator/internal/ignition"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

const (
	DefaultIgnitionSecretKeyName = "ignition"
	ServerFinalizer              = "metal.ironcore.dev/server"
)

const (
	powerOpOn   = "PowerOn"
	powerOpOff  = "PowerOff"
	powerOpNoOP = "NoOp"
)

// ServerReconciler reconciles a Server object
type ServerReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Insecure         bool
	ManagerNamespace string
	ProbeImage       string
	RegistryURL      string
	ProbeOSImage     string
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/finalizers,verbs=update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, server)
}

func (r *ServerReconciler) reconcileExists(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if !server.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, server)
	}
	return r.reconcile(ctx, log, server)
}

func (r *ServerReconciler) delete(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log.V(1).Info("Deleting server")

	if server.Spec.BootConfigurationRef != nil {
		if err := r.Delete(ctx, &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: server.Spec.BootConfigurationRef.Namespace,
				Name:      server.Spec.BootConfigurationRef.Name,
			}}); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to delete server bootconfiguration: %w", err)
		}
		log.V(1).Info("Deleted server boot configuration")
	}

	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, server, ServerFinalizer); !apierrors.IsNotFound(err) || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured that the finalizer has been removed")

	log.V(1).Info("Deleted server")
	return ctrl.Result{}, nil
}

func (r *ServerReconciler) reconcile(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (ctrl.Result, error) {
	log.V(1).Info("Reconciling Server")
	if shouldIgnoreReconciliation(server) {
		log.V(1).Info("Skipped Server reconciliation")
		return ctrl.Result{}, nil
	}

	// do late state initialization
	if server.Status.State == "" {
		if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateInitial); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, server, ServerFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	if server.Spec.ServerClaimRef != nil {
		if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateReserved); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	// TODO: This needs be reworked later as the Server cleanup has to happen here. For now we just transition the server
	// 		 back to available state.
	if server.Spec.ServerClaimRef == nil && server.Status.State == metalv1alpha1.ServerStateReserved {
		if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateAvailable); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	requeue, err := r.ensureServerStateTransition(ctx, log, server)
	if requeue && err == nil {
		return ctrl.Result{Requeue: requeue, RequeueAfter: 10 * time.Second}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure server state transition: %w", err)
	}

	log.V(1).Info("Reconciled Server")
	return ctrl.Result{}, nil
}

// Server state-machine:
//
// A Server goes through the following stages:
// Initial -> Available -> Reserved -> Tainted -> Available ...
//
// Initial:
// In the initial state we create a ServerBootConfiguration and an Ignition to start the Probe server on the
// Server. This Probe server registers with the managers /registry/{uuid} endpoint it's address, so the reconciler can
// fetch the server details from this endpoint. Once completed the Server is patched to the state Available.
//
// Available:
// In the available state, a Server can be claimed by a ServerClaim. Here the claim reconciler takes over to
// generate the necessary boot configuration. In the available state the Power state and indicator LEDs are being controlled.
//
// Reserved:
// A Server in a reserved state can not be claimed by another claim.
//
// Tainted:
// A tainted Server needs to be sanitized (clean up disks etc.). This is done in a similar way as in the
// initial state where the server reconciler will create a BootConfiguration and an Ignition secret to
// boot the server with a cleanup agent. This agent has also an endpoint to report its health state.
//
// Maintenance:
// A Maintenance state represents a special case where certain operations like BIOS updates should be performed.
func (r *ServerReconciler) ensureServerStateTransition(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	switch server.Status.State {
	case metalv1alpha1.ServerStateInitial:
		if err := r.updateServerStatus(ctx, log, server); err != nil {
			return false, err
		}
		log.V(1).Info("Updated Server status")

		// apply boot configuration
		config, err := r.applyBootConfigurationAndIgnitionForDiscovery(ctx, server)
		if err != nil {
			return false, fmt.Errorf("failed to apply server boot configuration: %w", err)
		}
		log.V(1).Info("Applied Server boot configuration")

		if ready, err := r.serverBootConfigurationIsReady(ctx, server); err != nil || !ready {
			log.V(1).Info("Server boot configuration is not ready. Retrying ...")
			return false, err
		}
		log.V(1).Info("Server boot configuration is ready")

		if err := r.pxeBootServer(ctx, log, server); err != nil {
			return false, fmt.Errorf("failed to boot server: %w", err)
		}
		log.V(1).Info("Booted Server in PXE")

		ready, err := r.extractServerDetailsFromRegistry(ctx, log, server)
		if !ready && err == nil {
			log.V(1).Info("Server agent did not post info to registry")
			return true, nil
		}
		if err != nil {
			log.V(1).Info("Could not get server details from registry.")
			// TODO: instead of requeue subscribe to registry events and requeue Server objects in SetupWithManager
			return false, err
		}
		log.V(1).Info("Extracted Server details")

		if err := r.ensureInitialBootConfigurationIsDeleted(ctx, config); err != nil {
			return false, fmt.Errorf("failed to ensure server initial boot configuration is deleted: %w", err)
		}
		log.V(1).Info("Ensured initial boot configuration is deleted")

		// TODO: fix that by providing the power state to the ensure method
		server.Spec.Power = metalv1alpha1.PowerOff
		if err := r.ensureServerPowerState(ctx, log, server); err != nil {
			return false, fmt.Errorf("failed to shutdown server: %w", err)
		}
		log.V(1).Info("Server state set to power off")

		log.V(1).Info("Setting Server state set to available")
		if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateAvailable); err != nil || modified {
			return false, err
		}
	case metalv1alpha1.ServerStateAvailable:
		if err := r.updateServerStatus(ctx, log, server); err != nil {
			return false, err
		}
		log.V(1).Info("Updated Server status")

		if err := r.ensureServerPowerState(ctx, log, server); err != nil {
			return false, fmt.Errorf("failed to ensure server power state: %w", err)
		}
		if err := r.ensureIndicatorLED(ctx, log, server); err != nil {
			return false, fmt.Errorf("failed to ensure server indicator led: %w", err)
		}
		log.V(1).Info("Reconciled available state")
	case metalv1alpha1.ServerStateReserved:

		if err := r.updateServerStatus(ctx, log, server); err != nil {
			return false, err
		}
		log.V(1).Info("Updated Server status")

		if err := r.ensureServerPowerState(ctx, log, server); err != nil {
			return false, fmt.Errorf("failed to ensure server power state: %w", err)
		}
		if err := r.ensureIndicatorLED(ctx, log, server); err != nil {
			return false, fmt.Errorf("failed to ensure server indicator led: %w", err)
		}
		log.V(1).Info("Reconciled reserved state")
	}
	return false, nil
}

func (r *ServerReconciler) updateServerStatus(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		log.V(1).Info("Server has no BMC connection configured")
		return nil
	}
	bmcClient, err := GetBMCClientFromBMCName(ctx, r.Client, server.Spec.BMCRef.Name, r.Insecure)
	if err != nil {
		return fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	systemInfo, err := bmcClient.GetSystemInfo(server.Spec.UUID)
	if err != nil {
		return fmt.Errorf("failed to get system info for Server: %w", err)
	}

	serverBase := server.DeepCopy()
	server.Status.PowerState = metalv1alpha1.ServerPowerState(systemInfo.PowerState)
	server.Status.SerialNumber = systemInfo.SerialNumber
	server.Status.SKU = systemInfo.SKU
	server.Status.Manufacturer = systemInfo.Manufacturer
	server.Status.IndicatorLED = metalv1alpha1.IndicatorLED(systemInfo.IndicatorLED)

	if err := r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch Server status: %w", err)
	}

	return nil
}

func (r *ServerReconciler) applyBootConfigurationAndIgnitionForDiscovery(ctx context.Context, server *metalv1alpha1.Server) (*metalv1alpha1.ServerBootConfiguration, error) {
	// apply server boot configuration
	bootConfig := &metalv1alpha1.ServerBootConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "metal.ironcore.dev/v1alpha1",
			Kind:       "ServerBootConfiguration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: r.ManagerNamespace,
		},
		Spec: metalv1alpha1.ServerBootConfigurationSpec{
			ServerRef: v1.LocalObjectReference{
				Name: server.Name,
			},
			IgnitionSecretRef: &v1.LocalObjectReference{
				Name: server.Name,
			},
			Image: r.ProbeOSImage,
		},
	}

	if err := r.Patch(ctx, bootConfig, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return nil, fmt.Errorf("failed to apply server boot configuration: %w", err)
	}

	if err := r.applyDefaultIgnitionForServer(ctx, server, bootConfig, r.RegistryURL); err != nil {
		return nil, fmt.Errorf("failed to apply default server ignitionSecret: %w", err)
	}

	return bootConfig, nil
}

func (r *ServerReconciler) applyDefaultIgnitionForServer(
	ctx context.Context,
	server *metalv1alpha1.Server,
	bootConfig *metalv1alpha1.ServerBootConfiguration,
	registryURL string,
) error {
	probeFlags := fmt.Sprintf("--registry-url=%s --server-uuid=%s", registryURL, server.Spec.UUID)
	ignitionData, err := r.generateDefaultIgnitionDataForServer(probeFlags)
	if err != nil {
		return fmt.Errorf("failed to generate default ignitionSecret data: %w", err)
	}

	ignitionSecret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.ManagerNamespace,
			Name:      bootConfig.Name,
		},
		Data: map[string][]byte{
			DefaultIgnitionSecretKeyName: ignitionData,
		},
	}

	if err := controllerutil.SetControllerReference(bootConfig, ignitionSecret, r.Client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference for default ignitionSecret: %w", err)
	}

	if err := r.Patch(ctx, ignitionSecret, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to apply default ignitionSecret: %w", err)
	}

	return nil
}

func (r *ServerReconciler) generateDefaultIgnitionDataForServer(flags string) ([]byte, error) {
	ignitionData, err := ignition.GenerateDefaultIgnitionData(ignition.ContainerConfig{
		Image: r.ProbeImage,
		Flags: flags,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate default ignition data: %w", err)
	}

	return ignitionData, nil
}

func (r *ServerReconciler) serverBootConfigurationIsReady(ctx context.Context, server *metalv1alpha1.Server) (bool, error) {
	config := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: r.ManagerNamespace, Name: server.Name}, config); err != nil {
		return false, err
	}
	return config.Status.State == metalv1alpha1.ServerBootConfigurationStateReady, nil
}

func (r *ServerReconciler) pxeBootServer(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	if server == nil {
		log.V(1).Info("PXE boot server is nil")
		return nil
	}

	if server.Spec.BMCRef == nil {
		return fmt.Errorf("can only PXE boot server with valid BMC ref")
	}

	bmcClient, err := GetBMCClientFromBMCName(ctx, r.Client, server.Spec.BMCRef.Name, r.Insecure)
	defer bmcClient.Logout()

	if err != nil {
		return fmt.Errorf("failed to get BMC client: %w", err)
	}
	if err := bmcClient.SetPXEBootOnce(server.Spec.UUID); err != nil {
		return fmt.Errorf("failed to set PXE boot one for server: %w", err)
	}

	// TODO: do a proper restart if Server is already in PowerOn state
	if err := bmcClient.PowerOn(server.Spec.UUID); err != nil {
		return fmt.Errorf("failed to power on server: %w", err)
	}
	return nil
}

func (r *ServerReconciler) extractServerDetailsFromRegistry(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	resp, err := http.Get(fmt.Sprintf("%s/systems/%s", r.RegistryURL, server.Spec.UUID))
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		log.V(1).Info("Did not find server information in registry")
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to fetch server details: %w", err)
	}

	serverDetails := &registry.Server{}
	if err := json.NewDecoder(resp.Body).Decode(serverDetails); err != nil {
		return false, fmt.Errorf("failed to decode server details: %w", err)
	}

	serverBase := server.DeepCopy()
	// update network interfaces
	nics := make([]metalv1alpha1.NetworkInterface, 0, len(serverDetails.NetworkInterfaces))
	for _, s := range serverDetails.NetworkInterfaces {
		nics = append(nics, metalv1alpha1.NetworkInterface{
			Name:       s.Name,
			IP:         metalv1alpha1.MustParseIP(s.IPAddress),
			MACAddress: s.MACAddress,
		})
	}
	server.Status.NetworkInterfaces = nics

	if err := r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return false, fmt.Errorf("failed to patch server status: %w", err)
	}

	return true, nil
}

func (r *ServerReconciler) patchServerState(ctx context.Context, server *metalv1alpha1.Server, state metalv1alpha1.ServerState) (bool, error) {
	if server.Status.State == state {
		return false, nil
	}
	serverBase := server.DeepCopy()
	server.Status.State = state
	if err := r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return false, fmt.Errorf("failed to patch server state: %w", err)
	}
	return true, nil
}

func (r *ServerReconciler) ensureServerPowerState(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	if server.Spec.Power == "" {
		// no desired power state set
		return nil
	}

	powerOp := powerOpNoOP
	if server.Status.PowerState != metalv1alpha1.ServerOnPowerState &&
		server.Status.PowerState != metalv1alpha1.ServerPoweringOnPowerState &&
		server.Spec.Power == metalv1alpha1.PowerOn {
		powerOp = powerOpOn
	}

	if server.Status.PowerState != metalv1alpha1.ServerOffPowerState &&
		server.Status.PowerState != metalv1alpha1.ServerPoweringOffPowerState &&
		server.Spec.Power == metalv1alpha1.PowerOff {
		powerOp = powerOpOff
	}

	if powerOp == powerOpNoOP {
		log.V(1).Info("Server already in target power state")
		return nil
	}

	bmcClient, err := GetBMCClientFromBMCName(ctx, r.Client, server.Spec.BMCRef.Name, r.Insecure)
	defer bmcClient.Logout()
	if err != nil {
		return fmt.Errorf("failed to get BMC client: %w", err)
	}

	if powerOp == powerOpOn {
		if err := bmcClient.PowerOn(server.Spec.UUID); err != nil {
			return fmt.Errorf("failed to power on server: %w", err)
		}
	}
	if powerOp == powerOpOff {
		if err := bmcClient.PowerOff(server.Spec.UUID); err != nil {
			return fmt.Errorf("failed to power off server: %w", err)
		}
	}
	log.V(1).Info("Ensured server power state", "PowerState", server.Spec.Power)

	return nil
}

func (r *ServerReconciler) ensureIndicatorLED(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	// TODO: implement
	return nil
}

func (r *ServerReconciler) ensureInitialBootConfigurationIsDeleted(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) error {
	if err := r.Delete(ctx, config); !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.Server{}).
		Watches(
			&metalv1alpha1.ServerBootConfiguration{},
			r.enqueueServerByServerBootConfiguration(),
		).
		Complete(r)
}

func (r *ServerReconciler) enqueueServerByServerBootConfiguration() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		config := obj.(*metalv1alpha1.ServerBootConfiguration)
		return []ctrl.Request{
			{
				NamespacedName: types.NamespacedName{Name: config.Spec.ServerRef.Name},
			},
		}
	})
}
