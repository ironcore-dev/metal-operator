// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/ironcore-dev/metal-operator/internal/ignition"
	"github.com/stmcginnis/gofish/redfish"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

const (
	// DefaultIgnitionSecretKeyName is the default key name for the ignition secret
	DefaultIgnitionSecretKeyName = "ignition"
	// DefaultIgnitionFormatKey is the key for the ignition format annotation
	DefaultIgnitionFormatKey = "format"
	// DefaultIgnitionFormatValue is the value for the ignition format annotation
	DefaultIgnitionFormatValue = "fcos"
	// SSHKeyPairSecretPrivateKeyName is the key name for the private key in the SSH key pair secret
	SSHKeyPairSecretPrivateKeyName = "pem"
	// SSHKeyPairSecretPublicKeyName is the key name for the public key in the SSH key pair secret
	SSHKeyPairSecretPublicKeyName = "pub"
	// SSHKeyPairSecretPasswordKeyName is the key name for the password in the SSH key pair secret
	SSHKeyPairSecretPasswordKeyName = "password"
	// ServerFinalizer is the finalizer for the server
	ServerFinalizer = "metal.ironcore.dev/server"
	// InternalAnnotationTypeKeyName is the key name for the internal annotation type
	InternalAnnotationTypeKeyName = "metal.ironcore.dev/type"
	// IsDefaultServerBootConfigOSImageKeyName is the key name for the is default OS image annotation
	IsDefaultServerBootConfigOSImageKeyName = "metal.ironcore.dev/is-default-os-image"
	// InternalAnnotationTypeValue is the value for the internal annotation type
	InternalAnnotationTypeValue = "Internal"
	// PoweringOnCondition is the condition type for powering on a server
	PoweringOnCondition = "PoweringOn"
)

const (
	// powerOpOn is the power on operation
	powerOpOn = "PowerOn"
	// powerOpOff is the power off operation
	powerOpOff = "PowerOff"
	// powerOpNoOP is the no operation
	powerOpNoOP = "NoOp"
)

// ServerReconciler reconciles a Server object
type ServerReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	Insecure                bool
	ManagerNamespace        string
	ProbeImage              string
	RegistryURL             string
	ProbeOSImage            string
	RegistryResyncInterval  time.Duration
	EnforceFirstBoot        bool
	EnforcePowerOff         bool
	ResyncInterval          time.Duration
	BMCOptions              bmc.Options
	DiscoveryTimeout        time.Duration
	MaxConcurrentReconciles int
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/finalizers,verbs=update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

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
	if r.shouldDelete(log, server) {
		return r.delete(ctx, log, server)
	}
	return r.reconcile(ctx, log, server)
}

func (r *ServerReconciler) shouldDelete(
	log logr.Logger,
	server *metalv1alpha1.Server,
) bool {
	if server.DeletionTimestamp.IsZero() {
		return false
	}

	if controllerutil.ContainsFinalizer(server, ServerFinalizer) &&
		server.Status.State == metalv1alpha1.ServerStateMaintenance {
		log.V(1).Info("postponing delete as server is in Maintenance state")
		return false
	}
	return true
}

func (r *ServerReconciler) delete(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(server, ServerFinalizer) {
		return ctrl.Result{}, nil
	}

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

	if server.Spec.BIOSSettingsRef != nil {
		if err := r.Delete(ctx, &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: server.Spec.BIOSSettingsRef.Name,
			}}); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to delete BIOS settings: %w", err)
		}
		log.V(1).Info("BIOS settings was deleted")
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

	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, r.Client, server, r.Insecure, r.BMCOptions)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client for server: %w", err)
	}
	defer bmcClient.Logout()

	if modified, err := r.patchServerURI(ctx, log, bmcClient, server); err != nil || modified {
		return ctrl.Result{}, err
	}

	if modified, err := r.handleAnnotionOperations(ctx, log, bmcClient, server); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Handled annotation operations")

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, server, ServerFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	if server.Spec.ServerMaintenanceRef != nil {
		if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateMaintenance); err != nil || modified {
			return ctrl.Result{}, err
		}
	} else {
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

	}

	if err := r.updateServerStatus(ctx, log, bmcClient, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update server status: %w", err)
	}
	log.V(1).Info("Updated Server status")

	if err := r.applyBootOrder(ctx, log, bmcClient, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update server bios boot order: %w", err)
	}
	log.V(1).Info("Updated Server BIOS boot order")

	requeue, err := r.ensureServerStateTransition(ctx, log, bmcClient, server)
	if requeue && err == nil {
		// we need to update the ServerStatus after state transition to make sure it reflects the changes done
		if err := r.updateServerStatus(ctx, log, bmcClient, server); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update server status: %w", err)
		}
		log.V(1).Info("Updated Server status after state transition")
		return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure server state transition: %w", err)
	}

	// we need to update the ServerStatus after state transition to make sure it reflects the changes done
	if err := r.updateServerStatus(ctx, log, bmcClient, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update server status: %w", err)
	}
	log.V(1).Info("Updated Server status after state transition")

	log.V(1).Info("Reconciled Server")
	return ctrl.Result{RequeueAfter: r.ResyncInterval}, nil
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
func (r *ServerReconciler) ensureServerStateTransition(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	switch server.Status.State {
	case metalv1alpha1.ServerStateInitial:
		return r.handleInitialState(ctx, log, bmcClient, server)
	case metalv1alpha1.ServerStateDiscovery:
		return r.handleDiscoveryState(ctx, log, bmcClient, server)
	case metalv1alpha1.ServerStateAvailable:
		return r.handleAvailableState(ctx, log, bmcClient, server)
	case metalv1alpha1.ServerStateReserved:
		return r.handleReservedState(ctx, log, bmcClient, server)
	case metalv1alpha1.ServerStateMaintenance:
		return r.handleMaintenanceState(ctx, log, bmcClient, server)
	default:
		return false, nil
	}
}

func (r *ServerReconciler) handleInitialState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	if requeue, err := r.ensureInitialConditions(ctx, log, bmcClient, server); err != nil || requeue {
		return requeue, err
	}
	log.V(1).Info("Initial conditions for Server met")

	if err := r.ensureServerPowerState(ctx, log, bmcClient, server); err != nil {
		return false, fmt.Errorf("failed to ensure server power state: %w", err)
	}
	log.V(1).Info("Ensured power state for Server")

	if err := r.updateServerStatusFromSystemInfo(ctx, log, bmcClient, server); err != nil {
		return false, fmt.Errorf("failed to update server status system info: %w", err)
	}
	log.V(1).Info("Updated Server status system info")

	if err := r.applyBootConfigurationAndIgnitionForDiscovery(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to apply server boot configuration: %w", err)
	}
	log.V(1).Info("Applied Server boot configuration")

	if err := r.pxeBootServer(ctx, log, bmcClient, server); err != nil {
		return false, fmt.Errorf("failed to set PXE boot for server: %w", err)
	}
	log.V(1).Info("Set PXE Boot for Server")

	if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateDiscovery); err != nil || modified {
		return false, err
	}
	return false, nil
}

func (r *ServerReconciler) handleDiscoveryState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
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

	if err := r.ensureServerPowerState(ctx, log, bmcClient, server); err != nil {
		return false, fmt.Errorf("failed to ensure server power state: %w", err)
	}
	log.V(1).Info("Server state set to power on")

	if err := r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return false, fmt.Errorf("failed to patch Server status: %w", err)
	}

	if r.checkLastStatusUpdateAfter(r.DiscoveryTimeout, server) {
		log.V(1).Info("Server did not post info to registry in time, back to initial state")
		if modified, err := r.patchServerState(ctx, server, metalv1alpha1.ServerStateInitial); err != nil || modified {
			return false, err
		}
	}

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

func (r *ServerReconciler) handleAvailableState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	serverBase := server.DeepCopy()
	if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
		server.Spec.Power = metalv1alpha1.PowerOff
		if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
			return false, fmt.Errorf("failed to update server power state: %w", err)
		}
		log.V(1).Info("Updated Server power state", "PowerState", metalv1alpha1.PowerOff)

		if err := r.ensureServerPowerState(ctx, log, bmcClient, server); err != nil {
			return false, fmt.Errorf("failed to ensure server power state: %w", err)
		}
		log.V(1).Info("Server state set to power off")
	}
	log.V(1).Info("ensureInitialBootConfigurationIsDeleted")
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

func (r *ServerReconciler) handleReservedState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	if ready, err := r.serverBootConfigurationIsReady(ctx, server); err != nil || !ready {
		log.V(1).Info("Server boot configuration is not ready. Retrying ...")
		return true, err
	}
	log.V(1).Info("Server boot configuration is ready")

	// TODO: fix properly, we need to free up the server if the claim does not exist anymore
	if server.Spec.ServerClaimRef != nil {
		claim := &metalv1alpha1.ServerClaim{}
		err := r.Get(ctx, client.ObjectKey{
			Name:      server.Spec.ServerClaimRef.Name,
			Namespace: server.Spec.ServerClaimRef.Namespace}, claim)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.V(1).Info(
					"ServerClaim not found, removing ServerClaimRef",
					"Server", server.Name,
					"ServerClaim", server.Spec.ServerClaimRef.Name)
				serverBase := server.DeepCopy()
				server.Spec.ServerClaimRef = nil
				if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
					return false, fmt.Errorf("failed to remove ServerClaimRef: %w", err)
				}
				return false, nil
			}
			return false, fmt.Errorf("failed to get ServerClaim: %w", err)
		}
	}

	//TODO: handle working Reserved Server that was suddenly powered off but needs to boot from disk
	if server.Status.PowerState == metalv1alpha1.ServerOffPowerState {
		if err := r.pxeBootServer(ctx, log, bmcClient, server); err != nil {
			return false, fmt.Errorf("failed to boot server: %w", err)
		}
		log.V(1).Info("Server is powered off, booting Server in PXE")
	}
	if err := r.ensureServerPowerState(ctx, log, bmcClient, server); err != nil {
		return false, fmt.Errorf("failed to ensure server power state: %w", err)
	}

	if err := r.ensureIndicatorLED(ctx, log, server); err != nil {
		return false, fmt.Errorf("failed to ensure server indicator led: %w", err)
	}
	log.V(1).Info("Reconciled reserved state")
	return true, nil
}

func (r *ServerReconciler) handleMaintenanceState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	if server.Spec.ServerMaintenanceRef == nil {
		log.V(1).Info("Server is in Maintenance state, but no ServerMaintenanceRef is set, transitioning back to previous state")
		// update system info in case the server was changed during Maintenance state (hardwere changes, biosVersion etc.)
		if err := r.updateServerStatusFromSystemInfo(ctx, log, bmcClient, server); err != nil {
			return false, fmt.Errorf("failed to update server status system info: %w", err)
		}
		if server.Spec.ServerClaimRef == nil {
			return r.patchServerState(ctx, server, metalv1alpha1.ServerStateInitial)
		}
		return r.patchServerState(ctx, server, metalv1alpha1.ServerStateReserved)
	}
	if err := r.ensureServerPowerState(ctx, log, bmcClient, server); err != nil {
		return false, fmt.Errorf("failed to ensure server power state: %w", err)
	}

	log.V(1).Info("Reconciled maintenance state")
	return false, nil
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
	return r.Patch(ctx, server, client.MergeFrom(serverBase))
}

// updates the Server status which can be changed via Spec
func (r *ServerReconciler) updateServerStatus(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		log.V(1).Info("Server has no BMC connection configured")
		return nil
	}
	systemInfo, err := bmcClient.GetSystemInfo(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get system info for Server: %w", err)
	}
	serverBase := server.DeepCopy()
	server.Status.PowerState = metalv1alpha1.ServerPowerState(systemInfo.PowerState)
	server.Status.IndicatorLED = metalv1alpha1.IndicatorLED(systemInfo.IndicatorLED)
	if err = r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch Server status: %w", err)
	}
	log.V(1).Info("Updated Server status", "Status", server.Status.State, "powerState", server.Status.PowerState)
	return nil
}

func (r *ServerReconciler) updateServerStatusFromSystemInfo(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
	serverBase := server.DeepCopy()
	systemInfo, err := bmcClient.GetSystemInfo(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get system info for Server: %w", err)
	}
	biosVersion, err := bmcClient.GetBiosVersion(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get BIOS version for Server: %w", err)
	}
	server.Status.BIOSVersion = biosVersion
	server.Status.PowerState = metalv1alpha1.ServerPowerState(systemInfo.PowerState)
	server.Status.SerialNumber = systemInfo.SerialNumber
	server.Status.SKU = systemInfo.SKU
	server.Status.Manufacturer = systemInfo.Manufacturer
	server.Status.Model = systemInfo.Model
	server.Status.TotalSystemMemory = &systemInfo.TotalSystemMemory

	processors, err := bmcClient.GetProcessors(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get processors for Server: %w", err)
	}
	server.Status.Processors = make([]metalv1alpha1.Processor, 0, len(processors))
	for _, processor := range processors {
		server.Status.Processors = append(server.Status.Processors, metalv1alpha1.Processor{
			ID:             processor.ID,
			Type:           processor.Type,
			Architecture:   processor.Architecture,
			InstructionSet: processor.InstructionSet,
			Manufacturer:   processor.Manufacturer,
			Model:          processor.Model,
			MaxSpeedMHz:    processor.MaxSpeedMHz,
			TotalCores:     processor.TotalCores,
			TotalThreads:   processor.TotalThreads,
		})
	}
	storages, err := bmcClient.GetStorages(ctx, server.Spec.SystemURI)
	if err != nil {
		return fmt.Errorf("failed to get storages for Server: %w", err)
	}
	server.Status.Storages = nil
	for _, storage := range storages {
		metalStorage := metalv1alpha1.Storage{
			Name:  storage.Name,
			State: metalv1alpha1.StorageState(storage.State),
		}
		for _, drive := range storage.Drives {
			metalStorage.Drives = append(metalStorage.Drives, metalv1alpha1.StorageDrive{
				Name:      drive.Name,
				Model:     drive.Model,
				Vendor:    drive.Vendor,
				Capacity:  resource.NewQuantity(drive.SizeBytes, resource.BinarySI),
				Type:      string(drive.Type),
				State:     metalv1alpha1.StorageState(drive.State),
				MediaType: drive.MediaType,
			})
		}
		metalStorage.Volumes = make([]metalv1alpha1.StorageVolume, 0, len(storage.Volumes))
		for _, volume := range storage.Volumes {
			metalStorage.Volumes = append(metalStorage.Volumes, metalv1alpha1.StorageVolume{
				Name:        volume.Name,
				Capacity:    resource.NewQuantity(volume.SizeBytes, resource.BinarySI),
				State:       metalv1alpha1.StorageState(volume.State),
				RAIDType:    string(volume.RAIDType),
				VolumeUsage: volume.VolumeUsage,
			})
		}
		server.Status.Storages = append(server.Status.Storages, metalStorage)
	}

	if err := r.Status().Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch Server status: %w", err)
	}
	log.V(1).Info("Updated Server status", "Status", server.Status.State, "powerState", server.Status.PowerState)
	return nil
}

func (r *ServerReconciler) applyBootConfigurationAndIgnitionForDiscovery(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	bootConfig := &metalv1alpha1.ServerBootConfiguration{}
	bootConfig.Name = server.Name
	bootConfig.Namespace = r.ManagerNamespace
	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, bootConfig, func() error {
		if bootConfig.Annotations == nil {
			bootConfig.Annotations = make(map[string]string)
		}
		bootConfig.Annotations[InternalAnnotationTypeKeyName] = InternalAnnotationTypeValue
		bootConfig.Annotations[IsDefaultServerBootConfigOSImageKeyName] = "true"
		bootConfig.Spec.ServerRef = v1.LocalObjectReference{Name: server.Name}
		bootConfig.Spec.IgnitionSecretRef = &v1.LocalObjectReference{Name: server.Name}
		bootConfig.Spec.Image = r.ProbeOSImage
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or patch ServerBootConfiguration: %w", err)
	}

	log.V(1).Info("Created or patched", "ServerBootConfiguration", bootConfig.Name, "Namespace", bootConfig.Namespace, "Operation", opResult)

	if err := r.ensureServerBootConfigRef(ctx, server, bootConfig); err != nil {
		return err
	}
	return r.applyDefaultIgnitionForServer(ctx, log, server, bootConfig, r.RegistryURL)
}

func (r *ServerReconciler) applyDefaultIgnitionForServer(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server, bootConfig *metalv1alpha1.ServerBootConfiguration, registryURL string) error {
	sshPrivateKey, sshPublicKey, password, err := generateSSHKeyPairAndPassword()
	if err != nil {
		return fmt.Errorf("failed to generate SSH keypair: %w", err)
	}

	sshSecret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.ManagerNamespace,
			Name:      fmt.Sprintf("%s-ssh", bootConfig.Name),
		},
		Data: map[string][]byte{
			SSHKeyPairSecretPublicKeyName:   sshPublicKey,
			SSHKeyPairSecretPrivateKeyName:  sshPrivateKey,
			SSHKeyPairSecretPasswordKeyName: password,
		},
	}
	if err := controllerutil.SetControllerReference(bootConfig, sshSecret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	if err := r.Patch(ctx, sshSecret, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to apply default SSH keypair: %w", err)
	}
	log.V(1).Info("Applied SSH keypair secret", "SSHKeyPair", client.ObjectKeyFromObject(sshSecret))

	probeFlags := fmt.Sprintf("--registry-url=%s --server-uuid=%s", registryURL, server.Spec.SystemUUID)
	ignitionData, err := r.generateDefaultIgnitionDataForServer(probeFlags, sshPublicKey, password)
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

	if err := controllerutil.SetControllerReference(bootConfig, ignitionSecret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := r.Patch(ctx, ignitionSecret, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to apply default ignition secret: %w", err)
	}
	log.V(1).Info("Applied Ignition Secret")

	return nil
}

func generateSSHKeyPairAndPassword() ([]byte, []byte, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	privateKeyBlock, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		return nil, nil, nil, err
	}
	privateKeyPem := pem.EncodeToMemory(privateKeyBlock)

	sshPubKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create SSH public key: %w", err)
	}
	publicKeyAuthorized := ssh.MarshalAuthorizedKey(sshPubKey)

	password, err := GenerateRandomPassword(20)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate password: %w", err)
	}

	return privateKeyPem, publicKeyAuthorized, password, nil
}

func (r *ServerReconciler) generateDefaultIgnitionDataForServer(flags string, sshPublicKey []byte, password []byte) ([]byte, error) {
	passwordHash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to generate password hash: %w", err)
	}

	ignitionData, err := ignition.GenerateDefaultIgnitionData(ignition.Config{
		Image:        r.ProbeImage,
		Flags:        flags,
		SSHPublicKey: string(sshPublicKey),
		PasswordHash: string(passwordHash),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate default ignition data: %w", err)
	}

	return ignitionData, nil
}

func (r *ServerReconciler) ensureInitialConditions(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	if server.Spec.Power == "" && server.Status.PowerState == metalv1alpha1.ServerOffPowerState {
		requeue, err := r.setAndPatchServerPowerState(ctx, log, bmcClient, server, metalv1alpha1.PowerOff)
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
		requeue, err := r.setAndPatchServerPowerState(ctx, log, bmcClient, server, metalv1alpha1.PowerOff)
		if err != nil {
			return false, fmt.Errorf("failed to set server power state: %w", err)
		}
		if requeue {
			return requeue, nil
		}
	}
	return false, nil
}

func (r *ServerReconciler) setAndPatchServerPowerState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server, powerState metalv1alpha1.Power) (bool, error) {
	op, err := controllerutil.CreateOrPatch(ctx, r.Client, server, func() error {
		server.Spec.Power = powerState
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to patch Server: %w", err)
	}
	if op == controllerutil.OperationResultUpdated {
		log.V(1).Info("Server updated to power off state.")
		if err := r.ensureServerPowerState(ctx, log, bmcClient, server); err != nil {
			log.V(1).Info("ensuring power state failed.")
		}
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

func (r *ServerReconciler) pxeBootServer(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
	if server == nil || server.Spec.BootConfigurationRef == nil {
		log.V(1).Info("Server not ready for netboot")
		return nil
	}

	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		return fmt.Errorf("can only PXE boot server with valid BMC ref or inline BMC configuration")
	}

	if err := bmcClient.SetPXEBootOnce(ctx, server.Spec.SystemURI); err != nil {
		return fmt.Errorf("failed to set PXE boot one for server: %w", err)
	}
	return nil
}

func (r *ServerReconciler) extractServerDetailsFromRegistry(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) (bool, error) {
	resp, err := http.Get(fmt.Sprintf("%s/systems/%s", r.RegistryURL, server.Spec.SystemUUID))
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
			Model:      s.Model,
			Speed:      s.Speed,
			Revision:   s.Revision,
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

func (r *ServerReconciler) patchServerURI(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	if len(server.Spec.SystemURI) != 0 {
		return false, nil
	}
	log.V(1).Info("Patching systemURI to the server resource")

	systems, err := bmcClient.GetSystems(ctx)
	if err != nil {
		return false, err
	}

	for _, system := range systems {
		if strings.EqualFold(system.UUID, server.Spec.SystemUUID) {
			serverBase := server.DeepCopy()
			server.Spec.SystemURI = system.URI
			if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
				return false, fmt.Errorf("failed to patch server URI: %w", err)
			}
		}
	}
	if len(server.Spec.SystemURI) == 0 {
		log.V(1).Info("Patching systemURI failed", "no system found for UUID", server.Spec.SystemUUID)
		return false, fmt.Errorf("unable to find system URI for UUID: %v", server.Spec.SystemUUID)
	}

	return true, nil
}

func (r *ServerReconciler) ensureServerPowerState(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
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
		log.V(1).Info("Server already in target power state", "powerState", server.Status.PowerState)
		return nil
	}

	switch powerOp {
	case powerOpOn:
		log.V(1).Info("Server Power On")
		if err := bmcClient.PowerOn(ctx, server.Spec.SystemURI); err != nil {
			return fmt.Errorf("failed to power on server: %w", err)
		}
		if err := bmcClient.WaitForServerPowerState(ctx, server.Spec.SystemURI, redfish.OnPowerState); err != nil {
			return fmt.Errorf("failed to wait for server power on server: %w", err)
		}
		if err := r.updatePowerOnCondition(ctx, server); err != nil {
			return fmt.Errorf("failed to update power on condition: %w", err)
		}
	case powerOpOff:
		log.V(1).Info("Server Power Off")
		powerOffType := bmcClient.PowerOff

		if err := powerOffType(ctx, server.Spec.SystemURI); err != nil {
			return fmt.Errorf("failed to power off server: %w", err)
		}
		if err := bmcClient.WaitForServerPowerState(ctx, server.Spec.SystemURI, redfish.OffPowerState); err != nil {
			if r.EnforcePowerOff {
				log.V(1).Info("Failed to wait for server graceful shutdown, retrying with force power off")
				powerOffType = bmcClient.ForcePowerOff
				if err := powerOffType(ctx, server.Spec.SystemURI); err != nil {
					return fmt.Errorf("failed to power off server: %w", err)
				}
				if err := bmcClient.WaitForServerPowerState(ctx, server.Spec.SystemURI, redfish.OffPowerState); err != nil {
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

func (r *ServerReconciler) updatePowerOnCondition(ctx context.Context, server *metalv1alpha1.Server) error {
	original := server.DeepCopy()
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	err := acc.UpdateSlice(
		&server.Status.Conditions,
		PoweringOnCondition,
		conditionutils.UpdateStatus(metav1.ConditionTrue),
		conditionutils.UpdateReason("ServerPowerOn"),
		conditionutils.UpdateMessage("Server is powering on"),
		conditionutils.UpdateObserved(server),
	)
	if err != nil {
		return fmt.Errorf("failed to update powering on condition: %w", err)
	}
	return r.Status().Patch(ctx, server, client.MergeFrom(original))
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
	url := fmt.Sprintf("%s/delete/%s", r.RegistryURL, server.Spec.SystemUUID)

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

func (r *ServerReconciler) applyBootOrder(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) error {
	if server.Spec.BMCRef == nil && server.Spec.BMC == nil {
		log.V(1).Info("Server has no BMC connection configured")
		return nil
	}

	order, err := bmcClient.GetBootOrder(ctx, server.Spec.SystemURI)
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
		return bmcClient.SetBootOrder(ctx, server.Spec.SystemURI, newOrder)
	}
	return nil
}

func (r *ServerReconciler) handleAnnotionOperations(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, server *metalv1alpha1.Server) (bool, error) {
	annotations := server.GetAnnotations()
	operation, ok := annotations[metalv1alpha1.OperationAnnotation]
	if !ok {
		return false, nil
	}

	log.V(1).Info("Handling operation", "Operation", operation)
	if err := bmcClient.Reset(ctx, server.Spec.SystemURI, redfish.ResetType(operation)); err != nil {
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

func (r *ServerReconciler) checkLastStatusUpdateAfter(duration time.Duration, server *metalv1alpha1.Server) bool {
	length := len(server.ManagedFields) - 1
	if server.ManagedFields[length].Operation == "Update" {
		if server.ManagedFields[length].Subresource == "status" {
			if server.ManagedFields[length].Time.Add(duration).Before(time.Now()) {
				return true
			}
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.MaxConcurrentReconciles,
		}).
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
