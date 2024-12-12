// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	if err := r.updateBMCStatusDetails(ctx, log, bmcObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC details: %w", err)
	}
	log.V(1).Info("Updated BMC status")

	if err := r.discoverServers(ctx, log, bmcObj); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to discover servers: %w", err)
	}
	log.V(1).Info("Discovered servers")

	log.V(1).Info("Reconciled BMC")
	return ctrl.Result{}, nil
}

func (r *BMCReconciler) updateBMCStatusDetails(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) error {
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

	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCPollingOptions)
	if err != nil {
		return fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	// TODO: Secret rotation/User management

	manager, err := bmcClient.GetManager()
	if err != nil {
		return fmt.Errorf("failed to get manager details: %w", err)
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
			return err
		}
	}

	return nil
}

func (r *BMCReconciler) discoverServers(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) error {
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCPollingOptions)
	if err != nil {
		return fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	servers, err := bmcClient.GetSystems(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Servers from BMC: %w", err)
	}
	for i, s := range servers {
		server := &metalv1alpha1.Server{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "metal.ironcore.dev/v1alpha1",
				Kind:       "Server",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(i, bmcObj),
			},
			Spec: metalv1alpha1.ServerSpec{
				UUID:       strings.ToLower(s.UUID), // always use lower-case uuids
				SystemUUID: strings.ToLower(s.UUID), // always use lower-case uuids
				BMCRef:     &v1.LocalObjectReference{Name: bmcObj.Name},
			},
		}

		if err := controllerutil.SetControllerReference(bmcObj, server, r.Scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on Server: %w", err)
		}

		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, server, nil)
		if err != nil {
			return fmt.Errorf("failed to create or patch Server: %w", err)
		}
		log.V(1).Info("Created or patched Server", "Server", server.Name, "Operation", opResult)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMC{}).
		Owns(&metalv1alpha1.Server{}).
		// TODO: add watches for Endpoints and BMCSecrets
		Complete(r)
}
