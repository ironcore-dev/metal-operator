// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const BMCFinalizer = "metal.ironcore.dev/bmc"

// BMCReconciler reconciles a BMC object
type BMCReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Insecure          bool
	BMCPollingOptions bmc.BMCOptions
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BMCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, req.NamespacedName, bmcObj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, bmcObj)
}

func (r *BMCReconciler) reconcileExists(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	if !bmcObj.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, bmcObj)
	}
	return r.reconcile(ctx, log, bmcObj)
}

func (r *BMCReconciler) delete(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	log.V(1).Info("Deleting BMC")
	if _, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, bmcObj, BMCFinalizer); err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Deleted BMC")
	return ctrl.Result{}, nil
}

func (r *BMCReconciler) reconcile(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	log.V(1).Info("Reconciling BMC")
	if shouldIgnoreReconciliation(bmcObj) {
		log.V(1).Info("Skipped BMC reconciliation")
		return ctrl.Result{}, nil
	}

	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCPollingOptions)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	if err := r.updateBMCStatusDetails(ctx, log, bmcClient, bmcObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update BMC status: %w", err)
	}
	log.V(1).Info("Updated BMC status")

	if err := r.discoverServers(ctx, log, bmcClient, bmcObj); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to discover servers: %w", err)
	}
	log.V(1).Info("Discovered servers")

	log.V(1).Info("Reconciled BMC")
	return ctrl.Result{}, nil
}

func (r *BMCReconciler) updateBMCStatusDetails(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, bmcObj *metalv1alpha1.BMC) error {
	var (
		ip         metalv1alpha1.IP
		macAddress string
	)

	if bmcObj.Spec.EndpointRef != nil {
		endpoint := &metalv1alpha1.Endpoint{}
		if err := r.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.EndpointRef.Name}, endpoint); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get Endpoints for BMC: %w", err)
		}
		ip = endpoint.Spec.IP
		macAddress = endpoint.Spec.MACAddress
		log.V(1).Info("Got Endpoints for BMC", "Endpoints", endpoint.Name)
	}

	if bmcObj.Spec.Endpoint != nil {
		ip = bmcObj.Spec.Endpoint.IP
		macAddress = bmcObj.Spec.Endpoint.MACAddress
	}

	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.IP = ip
	bmcObj.Status.MACAddress = macAddress
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to patch IP and MAC address status: %w", err)
	}

	// TODO: Secret rotation/User management

	manager, err := bmcClient.GetManager()
	if err != nil {
		return fmt.Errorf("failed to get manager details for BMC %s: %w", bmcObj.Name, err)
	}
	if manager != nil {
		bmcBase := bmcObj.DeepCopy()
		bmcObj.Status.Manufacturer = manager.Manufacturer
		bmcObj.Status.State = metalv1alpha1.BMCState(manager.State)
		bmcObj.Status.PowerState = metalv1alpha1.BMCPowerState(manager.PowerState)
		bmcObj.Status.FirmwareVersion = manager.FirmwareVersion
		bmcObj.Status.SerialNumber = manager.SerialNumber
		bmcObj.Status.SKU = manager.SKU
		bmcObj.Status.Model = manager.Model
		if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
			return fmt.Errorf("failed to patch manager details for BMC %s: %w", bmcObj.Name, err)
		}
	} else {
		log.V(1).Info("Manager details not available for BMC", "BMC", bmcObj.Name)
	}

	return nil
}

func (r *BMCReconciler) discoverServers(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, bmcObj *metalv1alpha1.BMC) error {
	servers, err := bmcClient.GetSystems(ctx)
	if err != nil {
		return fmt.Errorf("failed to get servers from BMC %s: %w", bmcObj.Name, err)
	}

	var errs []error
	for i, s := range servers {
		server := &metalv1alpha1.Server{}
		server.Name = bmcutils.GetServerNameFromBMCandIndex(i, bmcObj)

		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, server, func() error {
			metautils.SetLabels(server, bmcObj.Labels)
			server.Spec.UUID = strings.ToLower(s.UUID)
			server.Spec.SystemUUID = strings.ToLower(s.UUID)
			server.Spec.BMCRef = &v1.LocalObjectReference{Name: bmcObj.Name}
			return controllerutil.SetControllerReference(bmcObj, server, r.Scheme)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create or patch server %s: %w", server.Name, err))
			continue
		}
		log.V(1).Info("Created or patched Server", "Server", server.Name, "Operation", opResult)
		if err := r.updateServerDetails(ctx, log, server, bmcClient); err != nil {
			errs = append(errs, fmt.Errorf("failed to write server details for %s: %w", server.Name, err))
			continue
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred during server discovery: %v", errs)
	}

	return nil
}

func (r *BMCReconciler) updateServerDetails(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server, bmcClient bmc.BMC) error {
	serverBase := server.DeepCopy()
	systemInfo, err := bmcClient.GetSystemInfo(ctx, server.Spec.SystemUUID)
	if err != nil {
		return fmt.Errorf("failed to get system info for Server: %w", err)
	}
	server.Status.SerialNumber = systemInfo.SerialNumber
	server.Status.SKU = systemInfo.SKU
	server.Status.Manufacturer = systemInfo.Manufacturer
	server.Status.Model = systemInfo.Model
	server.Status.IndicatorLED = metalv1alpha1.IndicatorLED(systemInfo.IndicatorLED)
	server.Status.TotalSystemMemory = &systemInfo.TotalSystemMemory

	processors, err := bmcClient.GetProcessors(ctx, server.Spec.SystemUUID)
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
	storages, err := bmcClient.GetStorages(ctx, server.Spec.SystemUUID)
	if err != nil {
		return fmt.Errorf("failed to get storages for Server: %w", err)
	}
	server.Status.Storages = nil
	for _, storage := range storages {
		metalStorage := metalv1alpha1.Storage{
			Name:  storage.Name,
			State: metalv1alpha1.StorageState(storage.State),
		}
		metalStorage.Drives = make([]metalv1alpha1.StorageDrive, 0, len(storage.Drives))
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
		return fmt.Errorf("failed to patch server status: %w", err)
	}
	log.V(1).Info("Updated server status", "Server", server.Name)
	return nil
}

func (r *BMCReconciler) enqueueBMCByEndpoint(ctx context.Context, obj client.Object) []ctrl.Request {
	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{Name: obj.(*metalv1alpha1.Endpoint).Name},
		},
	}
}

func (r *BMCReconciler) enqueueBMCByBMCSecret(ctx context.Context, obj client.Object) []ctrl.Request {
	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{Name: obj.(*metalv1alpha1.BMCSecret).Name},
		},
	}
}

func (r *BMCReconciler) enqueueBMCByServer(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	server := obj.(*metalv1alpha1.Server)
	if server.Spec.BMCRef != nil {
		// Check if the server is being deleted or if the operation is to update server details
		if server.Annotations[metalv1alpha1.OperationAnnotation] == metalv1alpha1.OperationAnnotationUpdateServerDetails ||
			server.DeletionTimestamp != nil {
			log.V(1).Info("Enqueueing BMC by server", "Server", server.Name)
			return []ctrl.Request{
				{
					NamespacedName: types.NamespacedName{Name: server.Spec.BMCRef.Name},
				},
			}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMC{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByServer)).
		Watches(&metalv1alpha1.Endpoint{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByEndpoint)).
		Watches(&metalv1alpha1.BMCSecret{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByBMCSecret)).
		Complete(r)
}
