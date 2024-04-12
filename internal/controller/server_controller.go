// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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
		if err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateInitial); err != nil {
			return ctrl.Result{}, err
		}
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, server, ServerFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	if err := r.updateServerStatus(ctx, log, server); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Updated Server status")

	// Server state-machine:
	//
	// A Server goes through the following stages:
	// Initial -> Available -> Reserved -> Tainted -> Available ...
	//
	// Initial: In the initial state we create a ServerBootConfiguration and an Ignition to start the Probe server on
	//			the Server. This Probe server registers with the managers /registry/{uuid} endpoint it's address, so
	//			the reconciler can fetch the server details from this endpoint. Once completed the Server is patched
	//			to the state Available.
	//
	// Available: In the available state, a Server can be claimed by a ServerClaim. Here the claim reconciler takes over
	//			  to generate the necessary boot configuration. In the available state the Power state and indicator LEDs
	//			  are being controlled.
	//
	// Reserved: A Server in a reserved state can not be claimed by another claim.
	//
	// Tainted: A tainted Server needs to be sanitized (clean up disks etc.). This is done in a similar way as in the
	//			initial state where the server reconciler will create a BootConfiguration and an Ignition secret to
	//			boot the server with a cleanup agent. This agent has also an endpoint to report its health state.
	//
	// Maintenance: A Maintenance state represents a special case where certain operations like BIOS updates should be
	// 				performed.
	switch server.Status.State {
	case metalv1alpha1.ServerStateInitial:
		// apply boot configuration
		if err := r.applyBootConfigurationAndIgnitionForDiscovery(ctx, server); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to apply server boot configuration: %w", err)
		}
		log.V(1).Info("Applied Server boot configuration")

		if ready, err := r.serverBootConfigurationIsReady(ctx, server); err != nil || !ready {
			log.V(1).Info("Server boot configuration is not ready")
			return ctrl.Result{}, err
		}
		log.V(1).Info("Server boot configuration is ready")

		if err := r.pxeBootServer(ctx, server); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to boot server: %w", err)
		}
		log.V(1).Info("Booted Server in PXE")

		if finished, err := r.extractServerDetailsFromRegistry(ctx, server); err != nil || !finished {
			log.V(1).Info("Could not get server details from registry.")
			// TODO: instead of requeue subscribe to registry events and requeue Server objects in SetupWithManager
			return ctrl.Result{}, err
		}
		log.V(1).Info("Extracted Server details")

		if err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateAvailable); err != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("Server state set to available")
	case metalv1alpha1.ServerStateAvailable:
		if err := r.ensureServerPowerState(ctx, log, server); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure server power state: %w", err)
		}
		if err := r.ensureIndicatorLED(ctx, log, server); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to ensure server indicator led: %w", err)
		}
		log.V(1).Info("Reconciled available state")
	}

	log.V(1).Info("Reconciled Server")
	return ctrl.Result{}, nil
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

func (r *ServerReconciler) applyBootConfigurationAndIgnitionForDiscovery(ctx context.Context, server *metalv1alpha1.Server) error {
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
			Image: r.ProbeImage,
		},
	}

	if err := r.Patch(ctx, bootConfig, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to apply server boot configuration: %w", err)
	}

	serverBase := server.DeepCopy()
	server.Spec.BootConfigurationRef = &v1.ObjectReference{
		Kind:       "ServerBootConfiguration",
		Namespace:  r.ManagerNamespace,
		Name:       server.Name,
		UID:        bootConfig.UID,
		APIVersion: "metal.ironcore.dev/v1alpha1",
	}

	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch server boot configuration ref in server: %w", err)
	}

	if err := r.applyDefaultIgnitionForServer(ctx, server, bootConfig, r.RegistryURL); err != nil {
		return fmt.Errorf("failed to apply default server ignitionSecret: %w", err)
	}

	return nil
}

func (r *ServerReconciler) applyDefaultIgnitionForServer(
	ctx context.Context,
	server *metalv1alpha1.Server,
	bootConfig *metalv1alpha1.ServerBootConfiguration,
	registryURL string,
) error {
	probeFlags := fmt.Sprintf("--registry-url=%s,--server-uuid=%s", registryURL, server.Spec.UUID)
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

func (r *ServerReconciler) pxeBootServer(ctx context.Context, server *metalv1alpha1.Server) error {
	bmcClient, err := GetBMCClientFromBMCName(ctx, r.Client, server.Spec.BMCRef.Name, r.Insecure)
	defer bmcClient.Logout()

	if err != nil {
		return fmt.Errorf("failed to get BMC client: %w", err)
	}
	if err := bmcClient.SetPXEBootOnce(server.Spec.UUID); err != nil {
		return fmt.Errorf("failed to set PXE boot one for server: %w", err)
	}

	if err := bmcClient.PowerOn(); err != nil {
		return fmt.Errorf("failed to power on server: %w", err)
	}
	return nil
}

func (r *ServerReconciler) extractServerDetailsFromRegistry(ctx context.Context, server *metalv1alpha1.Server) (bool, error) {
	resp, err := http.Get(fmt.Sprintf("%s/systems/%s", r.RegistryURL, server.Spec.UUID))
	if err != nil {
		return false, fmt.Errorf("failed to fetch server details: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("could not find server details: %s", resp.Status)
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

func (r *ServerReconciler) patchServerState(ctx context.Context, server *metalv1alpha1.Server, state metalv1alpha1.ServerState) error {
	serverBase := server.DeepCopy()
	server.Status.State = state
	if err := r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch server state: %w", err)
	}
	return nil
}

func (r *ServerReconciler) ensureServerPowerState(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
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
		if err := bmcClient.PowerOn(); err != nil {
			return fmt.Errorf("failed to power on server: %w", err)
		}
	}
	if powerOp == powerOpOff {
		if err := bmcClient.PowerOff(); err != nil {
			return fmt.Errorf("failed to power off server: %w", err)
		}
	}
	log.V(1).Info("Ensured server power state", "PowerState", server.Spec.Power)

	return nil
}

func (r *ServerReconciler) ensureIndicatorLED(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
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
