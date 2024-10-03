// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/ironcore-dev/metal-operator/bmc"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"github.com/ironcore-dev/metal-operator/internal/ignition"
	"github.com/stmcginnis/gofish/redfish"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	DefaultIgnitionSecretKeyName  = "ignition"
	DefaultIgnitionFormatKey      = "format"
	DefaultIgnitionFormatValue    = "fcos"
	ServerFinalizer               = "metal.ironcore.dev/server"
	InternalAnnotationTypeKeyName = "metal.ironcore.dev/type"
	InternalAnnotationTypeValue   = "Internal"
)

const (
	powerOpOn   = "PowerOn"
	powerOpOff  = "PowerOff"
	powerOpNoOP = "NoOp"
)

// ServerReconciler reconciles a Server object
type ServerReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	Insecure               bool
	ManagerNamespace       string
	ProbeImage             string
	RegistryURL            string
	ProbeOSImage           string
	RegistryResyncInterval time.Duration
	EnforceFirstBoot       bool
	EnforcePowerOff        bool
	ResyncInterval         time.Duration
	PowerPollingInterval   time.Duration
	PowerPollingTimeout    time.Duration
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

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

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, server, ServerFinalizer); err != nil || modified {
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
	if modified, err := r.handleAnnotionOperations(ctx, log, server); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Handled annotation operations")

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

	if err := r.updateServerStatus(ctx, log, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update server status: %w", err)
	}
	log.V(1).Info("Updated Server status")

	if err := r.applyBootOrder(ctx, log, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update server bios boot order: %w", err)
	}
	log.V(1).Info("Updated Server BIOS boot order")

	requeue, err := r.ensureServerStateTransition(ctx, log, server)
	if requeue && err == nil {
		return ctrl.Result{Requeue: requeue, RequeueAfter: r.ResyncInterval}, nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to ensure server state transition: %w", err)
	}

	log.V(1).Info("Reconciled Server")
	return ctrl.Result{}, nil
}

// Server state-machine:
//
// A Server goes through the following stages:
// Initial -> Discovery -> Available -> Reserved -> Tainted -> Available ...
//
// Initial:
// In the initial state we create a ServerBootConfiguration and an Ignition to start the Probe server on the
// Server. The Server is patched to the state Discovery.
//
// Discovery:
// In the discovery state we expect the Server to come up with the Probe server running.
// This Probe server registers with the managers /registry/{uuid} endpoint it's address, so the reconciler can
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
		return r.handleInitialState(ctx, log, server)
	case metalv1alpha1.ServerStateDiscovery:
		return r.handleDiscoveryState(ctx, log, server)
	case metalv1alpha1.ServerStateAvailable:
		return r.handleAvailableState(ctx, log, server)
	case metalv1alpha1.ServerStateReserved:
		return r.handleReservedState(ctx, log, server)
	default:
		return false, nil
	}
}

func (r *ServerReconciler) handleInitialState(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	if requeue, err := r.ensureInitialConditions(ctx, log, server); err != nil || requeue {
		return requeue, err
	}
	log.V(1).Info("Initial conditions for Server met")

	if err := r.ensureServerPowerState(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to ensure server power state: %w", err)
	}
	log.V(1).Info("Ensured power state for Server")

	if err := r.applyBootConfigurationAndIgnitionForDiscovery(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to apply server boot configuration: %w", err)
	}
	log.V(1).Info("Applied Server boot configuration")

	if err := r.pxeBootServer(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to set PXE boot for server: %w", err)
	}
	log.V(1).Info("Set PXE Boot for Server")

	if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateDiscovery); err != nil || modified {
		return false, err
	}
	return false, nil
}

func (r *ServerReconciler) handleDiscoveryState(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	if ready, err := r.serverBootConfigurationIsReady(ctx, server); err != nil || !ready {
		log.V(1).Info("Server boot configuration is not ready. Retrying ...")
		return true, err
	}
	log.V(1).Info("Server boot configuration is ready")

	serverBase := server.DeepCopy()
	server.Spec.Power = metalv1alpha1.PowerOn
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return false, fmt.Errorf("failed to update server power state: %w", err)
	}
	log.V(1).Info("Updated Server power state", "PowerState", metalv1alpha1.PowerOn)

	if err := r.ensureServerPowerState(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to ensure server power state: %w", err)
	}
	log.V(1).Info("Server state set to power on")

	ready, err := r.extractServerDetailsFromRegistry(ctx, log, server)
	if !ready && err == nil {
		log.V(1).Info("Server agent did not post info to registry")
		return true, nil
	}
	if err != nil {
		log.V(1).Info("Could not get server details from registry.")
		return false, err
	}
	log.V(1).Info("Extracted Server details")

	if err := r.invalidateRegistryEntryForServer(log, server); err != nil {
		return false, fmt.Errorf("failed to invalidate registry entry for server: %w", err)
	}
	log.V(1).Info("Removed Server from Registry")

	log.V(1).Info("Setting Server state set to available")
	if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateAvailable); err != nil || modified {
		return false, err
	}
	return false, nil
}

func (r *ServerReconciler) handleAvailableState(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	serverBase := server.DeepCopy()
	server.Spec.Power = metalv1alpha1.PowerOff
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return false, fmt.Errorf("failed to update server power state: %w", err)
	}
	log.V(1).Info("Updated Server power state", "PowerState", metalv1alpha1.PowerOff)

	if err := r.ensureServerPowerState(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to ensure server power state: %w", err)
	}
	log.V(1).Info("Server state set to power off")

	if err := r.ensureInitialBootConfigurationIsDeleted(ctx, server); err != nil {
		return false, fmt.Errorf("failed to ensure server initial boot configuration is deleted: %w", err)
	}
	log.V(1).Info("Ensured initial boot configuration is deleted")

	if err := r.ensureIndicatorLED(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to ensure server indicator led: %w", err)
	}
	log.V(1).Info("Reconciled available state")
	return true, nil
}

func (r *ServerReconciler) handleReservedState(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	if ready, err := r.serverBootConfigurationIsReady(ctx, server); err != nil || !ready {
		log.V(1).Info("Server boot configuration is not ready. Retrying ...")
		return true, err
	}
	log.V(1).Info("Server boot configuration is ready")

	// TODO: handle working Reserved Server that was suddenly powered off but needs to boot from disk
	if server.Status.PowerState == metalv1alpha1.ServerOffPowerState {
		if err := r.pxeBootServer(ctx, log, server); err != nil {
			return false, fmt.Errorf("failed to boot server: %w", err)
		}
		log.V(1).Info("Server is powered off, booting Server in PXE")
	}
	if err := r.ensureServerPowerState(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to ensure server power state: %w", err)
	}

	if err := r.ensureIndicatorLED(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to ensure server indicator led: %w", err)
	}
	log.V(1).Info("Reconciled reserved state")
	return true, nil
}

func (r *ServerReconciler) ensureServerBootConfigRef(ctx context.Context, server *metalv1alpha1.Server, config *metalv1alpha1.ServerBootConfiguration) error {
	serverBase := server.DeepCopy()
	server.Spec.BootConfigurationRef = &v1.ObjectReference{
		Namespace:  config.Namespace,
		Name:       config.Name,
		UID:        config.UID,
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerBootConfiguration",
	}
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return err
	}

	return nil
}

func (r *ServerReconciler) updateServerStatus(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		log.V(1).Info("Server has no BMC connection configured")
		return nil
	}
	bmcClient, err := GetBMCClientForServer(ctx, r.Client, server, r.Insecure)
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
	server.Status.Model = systemInfo.Model
	server.Status.IndicatorLED = metalv1alpha1.IndicatorLED(systemInfo.IndicatorLED)

	if err := r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch Server status: %w", err)
	}

	return nil
}

func (r *ServerReconciler) applyBootConfigurationAndIgnitionForDiscovery(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	// apply server boot configuration
	bootConfig := &metalv1alpha1.ServerBootConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "metal.ironcore.dev/v1alpha1",
			Kind:       "ServerBootConfiguration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: r.ManagerNamespace,
			Annotations: map[string]string{
				InternalAnnotationTypeKeyName: InternalAnnotationTypeValue,
			},
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

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, bootConfig, nil)
	if err != nil {
		return fmt.Errorf("failed to create or patch ServerBootConfiguration: %w", err)
	}
	log.V(1).Info("Created or patched", "ServerBootConfiguration", bootConfig.Name, "Operation", opResult)

	if err := r.ensureServerBootConfigRef(ctx, server, bootConfig); err != nil {
		return err
	}

	if err := r.applyDefaultIgnitionForServer(ctx, log, server, bootConfig, r.RegistryURL); err != nil {
		return err
	}

	return nil
}

func (r *ServerReconciler) applyDefaultIgnitionForServer(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server, bootConfig *metalv1alpha1.ServerBootConfiguration, registryURL string) error {
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
			DefaultIgnitionFormatKey:     []byte(DefaultIgnitionFormatValue),
			DefaultIgnitionSecretKeyName: ignitionData,
		},
	}

	if err := controllerutil.SetControllerReference(bootConfig, ignitionSecret, r.Client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference for default ignitionSecret: %w", err)
	}

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, ignitionSecret, nil)
	if err != nil {
		return fmt.Errorf("failed to create or patch Ignition Secret: %w", err)
	}
	log.V(1).Info("Created or patched Ignition Secret", "Secret", ignitionSecret.Name, "Operation", opResult)

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

func (r *ServerReconciler) ensureInitialConditions(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	if server.Spec.Power == "" && server.Status.PowerState == metalv1alpha1.ServerOffPowerState {
		requeue, err := r.setAndPatchServerPowerState(ctx, log, server, metalv1alpha1.PowerOff)
		if err != nil {
			return false, fmt.Errorf("failed to set server power state: %w", err)
		}
		if requeue {
			return requeue, nil
		}
	}

	if server.Status.State == metalv1alpha1.ServerStateInitial &&
		server.Status.PowerState == metalv1alpha1.ServerOnPowerState &&
		r.EnforceFirstBoot {
		log.V(1).Info("Server in initial state is powered on. Ensure that it is powered off.")
		requeue, err := r.setAndPatchServerPowerState(ctx, log, server, metalv1alpha1.PowerOff)
		if err != nil {
			return false, fmt.Errorf("failed to set server power state: %w", err)
		}
		if requeue {
			return requeue, nil
		}
	}
	return false, nil
}

func (r *ServerReconciler) setAndPatchServerPowerState(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server, powerState metalv1alpha1.Power) (bool, error) {
	op, err := controllerutil.CreateOrPatch(ctx, r.Client, server, func() error {
		server.Spec.Power = powerState
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to patch Server: %w", err)
	}
	if op == controllerutil.OperationResultUpdated {
		log.V(1).Info("Server updated to power off state.")
		return true, nil
	}
	return false, nil
}

func (r *ServerReconciler) serverBootConfigurationIsReady(ctx context.Context, server *metalv1alpha1.Server) (bool, error) {
	if server.Spec.BootConfigurationRef == nil {
		return false, nil
	}
	config := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: server.Spec.BootConfigurationRef.Namespace, Name: server.Spec.BootConfigurationRef.Name}, config); err != nil {
		return false, err
	}
	return config.Status.State == metalv1alpha1.ServerBootConfigurationStateReady, nil
}

func (r *ServerReconciler) pxeBootServer(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	if server == nil || server.Spec.BootConfigurationRef == nil {
		log.V(1).Info("Server not ready for netboot")
		return nil
	}

	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		return fmt.Errorf("can only PXE boot server with valid BMC ref or inline BMC configuration")
	}

	bmcClient, err := GetBMCClientForServer(ctx, r.Client, server, r.Insecure)
	defer func() {
		if bmcClient != nil {
			bmcClient.Logout()
		}
	}()

	if err != nil {
		return fmt.Errorf("failed to get BMC client: %w", err)
	}
	if err := bmcClient.SetPXEBootOnce(server.Spec.UUID); err != nil {
		return fmt.Errorf("failed to set PXE boot one for server: %w", err)
	}
	return nil
}

func (r *ServerReconciler) extractServerDetailsFromRegistry(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	resp, err := http.Get(fmt.Sprintf("%s/systems/%s", r.RegistryURL, server.Spec.UUID))
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		log.V(1).Info("Did not find server information in registry")
		return false, nil
	}

	if resp == nil {
		return false, fmt.Errorf("failed to find server information in registry")
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

	bmcClient, err := GetBMCClientForServer(ctx, r.Client, server, r.Insecure)
	defer func() {
		if bmcClient != nil {
			bmcClient.Logout()
		}
	}()
	if err != nil {
		return fmt.Errorf("failed to get BMC client: %w", err)
	}

	switch powerOp {
	case powerOpOn:
		if err := bmcClient.PowerOn(server.Spec.UUID); err != nil {
			return fmt.Errorf("failed to power on server: %w", err)
		}
		if err := r.waitForServerPowerState(ctx, log, bmcClient, server, redfish.OnPowerState); err != nil {
			return fmt.Errorf("failed to wait for server power on server: %w", err)
		}
	case powerOpOff:
		powerOffType := bmcClient.PowerOff

		if err := powerOffType(server.Spec.UUID); err != nil {
			return fmt.Errorf("failed to power off server: %w", err)
		}
		if err := r.waitForServerPowerState(ctx, log, bmcClient, server, redfish.OffPowerState); err != nil {
			if r.EnforcePowerOff {
				log.V(1).Info("Failed to wait for server graceful shutdown, retrying with force power off")
				powerOffType = bmcClient.ForcePowerOff
				if err := powerOffType(server.Spec.UUID); err != nil {
					return fmt.Errorf("failed to power off server: %w", err)
				}
				if err := r.waitForServerPowerState(ctx, log, bmcClient, server, redfish.OffPowerState); err != nil {
					return fmt.Errorf("failed to wait for server force power off: %w", err)
				}
			} else {
				return fmt.Errorf("failed to wait for server power off: %w", err)
			}
		}
	}
	log.V(1).Info("Ensured server power state", "PowerState", server.Spec.Power)

	return nil
}

func (r *ServerReconciler) waitForServerPowerState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server, powerState redfish.PowerState) error {
	if err := wait.PollUntilContextTimeout(ctx, r.PowerPollingInterval, r.PowerPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
		log.V(1).Info("Waiting for Server to reach target power state", "TargetPowerState", powerState)
		sysInfo, err := bmcClient.GetSystemInfo(server.Spec.UUID)
		if err != nil {
			return false, fmt.Errorf("failed to get system info: %w", err)
		}
		log.V(1).Info("Read Server power state", "PowerState", sysInfo.PowerState, "TargetPowerState", powerState)
		return sysInfo.PowerState == powerState, nil
	}); err != nil {
		return fmt.Errorf("failed to wait for for server power state: %w", err)
	}
	return nil
}

func (r *ServerReconciler) ensureIndicatorLED(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	// TODO: implement
	return nil
}

func (r *ServerReconciler) ensureInitialBootConfigurationIsDeleted(ctx context.Context, server *metalv1alpha1.Server) error {
	if server.Spec.BootConfigurationRef == nil {
		return nil
	}

	config := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: server.Spec.BootConfigurationRef.Namespace, Name: server.Spec.BootConfigurationRef.Name}, config); err != nil {
		return err
	}

	if val, ok := config.Annotations[InternalAnnotationTypeKeyName]; !ok || val != InternalAnnotationTypeValue {
		// hit a non-initial boot config
		return nil
	}

	if err := r.Delete(ctx, config); err != nil {
		return err
	}

	serverBase := server.DeepCopy()
	server.Spec.BootConfigurationRef = nil
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return err
	}

	return nil
}

func (r *ServerReconciler) invalidateRegistryEntryForServer(log logr.Logger, server *metalv1alpha1.Server) error {
	url := fmt.Sprintf("%s/delete/%s", r.RegistryURL, server.Spec.UUID)

	c := &http.Client{}

	// Create the DELETE request
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	// Send the request
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Error(err, "Failed to close response body")
		}
	}(resp.Body)
	return nil
}

func (r *ServerReconciler) applyBootOrder(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		log.V(1).Info("Server has no BMC connection configured")
		return nil
	}
	bmcClient, err := GetBMCClientForServer(ctx, r.Client, server, r.Insecure)
	if err != nil {
		return fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	order, err := bmcClient.GetBootOrder(server.Spec.UUID)
	if err != nil {
		return fmt.Errorf("failed to create BMC client: %w", err)
	}

	sort.Slice(server.Spec.BootOrder, func(i, j int) bool {
		return server.Spec.BootOrder[i].Priority < server.Spec.BootOrder[j].Priority
	})
	newOrder := []string{}
	change := false
	for i, boot := range server.Spec.BootOrder {
		newOrder = append(newOrder, boot.Device)
		if order[i] != boot.Device {
			change = true
		}
	}
	if change {
		return bmcClient.SetBootOrder(server.Spec.UUID, newOrder)
	}
	return nil
}

func (r *ServerReconciler) handleAnnotionOperations(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	annotations := server.GetAnnotations()
	operation, ok := annotations[metalv1alpha1.OperationAnnotation]
	if !ok {
		return false, nil
	}
	bmcClient, err := GetBMCClientForServer(ctx, r.Client, server, r.Insecure)
	if err != nil {
		return false, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()
	log.V(1).Info("Handling operation", "Operation", operation)
	if err := bmcClient.Reset(server.Spec.UUID, redfish.ResetType(operation)); err != nil {
		return false, fmt.Errorf("failed to reset server: %w", err)
	}
	log.V(1).Info("Operation completed", "Operation", operation)
	serverBase := server.DeepCopy()
	delete(annotations, metalv1alpha1.OperationAnnotation)
	server.SetAnnotations(annotations)
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return false, fmt.Errorf("failed to patch server annotations: %w", err)
	}
	return true, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create a channel to send periodic events
	ch := make(chan event.TypedGenericEvent[*metalv1alpha1.Server])

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to send events to the channel at the specified interval
	go func() {
		ticker := time.NewTicker(r.ResyncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Emit an event to trigger reconciliation
				ch <- event.TypedGenericEvent[*metalv1alpha1.Server]{
					Object: &metalv1alpha1.Server{},
				}
			case <-ctx.Done():
				close(ch)
				return
			}
		}
	}()

	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.Server{}).
		Watches(
			&metalv1alpha1.ServerBootConfiguration{},
			r.enqueueServerByServerBootConfiguration(),
		).
		WatchesRawSource(source.Channel(ch, &handler.TypedEnqueueRequestForObject[*metalv1alpha1.Server]{})).
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
