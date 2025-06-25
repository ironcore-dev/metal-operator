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
	BMCPollingOptions bmc.Options
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

	if bmcObj.Spec.BMCSettingRef != nil {
		bmcSettings := &metalv1alpha1.BMCSettings{}
		if err := r.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.BMCSettingRef.Name}, bmcSettings); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get BMCSettings for BMC: %w", err)
		}
		if err := r.Delete(ctx, bmcSettings); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to deleted referred BMCSettings. %w", err)
		}
	}

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

	manager, err := bmcClient.GetManager(bmcObj.Spec.BMCUUID)
	if err != nil {
		return fmt.Errorf("failed to get manager details for BMC %s: %w", bmcObj.Name, err)
	}

	if manager != nil {
		bmcBase := bmcObj.DeepCopy()
		bmcObj.Status.Manufacturer = manager.Manufacturer
		bmcObj.Status.State = metalv1alpha1.BMCState(string(manager.Status.State))
		bmcObj.Status.PowerState = metalv1alpha1.BMCPowerState(string(manager.PowerState))
		bmcObj.Status.FirmwareVersion = manager.FirmwareVersion
		bmcObj.Status.SerialNumber = manager.SerialNumber
		bmcObj.Status.SKU = manager.PartNumber
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
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred during server discovery: %v", errs)
	}

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

// SetupWithManager sets up the controller with the Manager.
func (r *BMCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMC{}).
		Owns(&metalv1alpha1.Server{}).
		Watches(&metalv1alpha1.Endpoint{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByEndpoint)).
		Watches(&metalv1alpha1.BMCSecret{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByBMCSecret)).
		Complete(r)
}
