// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// ServerMaintenanceSetFinalizer is the finalizer for ServerMaintenanceSet
const ServerMaintenanceSetFinalizer = "metal.ironcore.dev/servermaintenanceset"

// ServerMaintenanceSetReconciler reconciles a ServerMaintenanceSet object
type ServerMaintenanceSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenancesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenancesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenancesets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServerMaintenanceSet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *ServerMaintenanceSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	replica := &metalv1alpha1.ServerMaintenanceSet{}

	if err := r.Get(ctx, req.NamespacedName, replica); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, replica)
}

func (r *ServerMaintenanceSetReconciler) reconcileExists(ctx context.Context, replica *metalv1alpha1.ServerMaintenanceSet) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	if !replica.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, replica)
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, replica, ServerMaintenanceSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	return r.reconcile(ctx, log, replica)
}

func (r *ServerMaintenanceSetReconciler) delete(ctx context.Context, log logr.Logger, replica *metalv1alpha1.ServerMaintenanceSet) (ctrl.Result, error) {
	log.V(1).Info("Deleting ServerMaintenanceSet")

	maintenancelist, err := r.getOwnedMaintenances(ctx, replica)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, maintenance := range maintenancelist.Items {
		if maintenance.Status.State == metalv1alpha1.ServerMaintenanceStateInMaintenance ||
			maintenance.Status.State == metalv1alpha1.ServerMaintenanceStateCompleted {
			log.V(1).Info("Owned Maintenance is still inMaintenance or completed", "maintenance", maintenance.Name)
			continue
		}
		if err := r.Delete(ctx, &maintenance); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, replica, ServerMaintenanceSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Deleted ServerMaintenanceSet")
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceSetReconciler) reconcile(ctx context.Context, log logr.Logger, replica *metalv1alpha1.ServerMaintenanceSet) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerMaintenanceSet", "Name", replica.Name)
	servers, err := r.getServersBySelector(ctx, replica)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(servers.Items) == 0 {
		log.V(1).Info("No servers found")
		return ctrl.Result{}, nil
	}

	maintenancelist, err := r.getOwnedMaintenances(ctx, replica)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If all servers have a maintenance object, only update the status
	if len(servers.Items) == len(maintenancelist.Items) {
		if modified, err := r.patchStatus(ctx, replica, calculateStatus(maintenancelist.Items)); err != nil || modified {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	var errs []error
out:
	for i, server := range servers.Items {
		log.V(1).Info("Reconciling server", "server", server.Name)
		// Check if the server already got a maintenance
		for _, maintenance := range maintenancelist.Items {
			if maintenance.Spec.ServerRef.Name == server.Name {
				log.V(1).Info("Server is already in maintenance", "server", server.Name)
				continue out
			}
		}
		maintenance := metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", replica.Name, i),
				Namespace: replica.Namespace,
			},
		}
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &maintenance, func() error {
			metautils.SetLabels(&maintenance, map[string]string{ServerMaintenanceSetFinalizer: replica.Name})
			maintenance.Spec = metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:                       &v1.LocalObjectReference{Name: server.Name},
				Policy:                          replica.Spec.Template.Policy,
				ServerPower:                     replica.Spec.Template.ServerPower,
				ServerBootConfigurationTemplate: replica.Spec.Template.ServerBootConfigurationTemplate,
			}
			return controllerutil.SetControllerReference(replica, &maintenance, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create or patch serverMaintenance %s: %w", maintenance.Name, err))
			continue
		}
		log.V(1).Info("Created or patched ServerMaintenance", "ServerMaintenance", maintenance.Name, "Operation", opResult)
	}
	if len(errs) > 0 {
		return ctrl.Result{}, fmt.Errorf("errors occurred during servermaintenances creation: %v", errs)
	}

	// Fetch the list of maintenances again to get the updated status
	maintenancelist, err = r.getOwnedMaintenances(ctx, replica)
	if err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Fetched ServerMaintenances", "Count", len(maintenancelist.Items))
	if modified, err := r.patchStatus(ctx, replica, calculateStatus(maintenancelist.Items)); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Reconciled ServerMaintenanceSet", "Name", replica.Name)

	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceSetReconciler) patchStatus(ctx context.Context, replica *metalv1alpha1.ServerMaintenanceSet, status metalv1alpha1.ServerMaintenanceSetStatus) (bool, error) {
	if replica.Status == status {
		return false, nil
	}
	base := replica.DeepCopy()
	replica.Status = status
	if err := r.Status().Patch(ctx, replica, client.MergeFrom(base)); err != nil {
		return false, err
	}

	return true, nil
}

func (r *ServerMaintenanceSetReconciler) getOwnedMaintenances(ctx context.Context, replica *metalv1alpha1.ServerMaintenanceSet) (*metalv1alpha1.ServerMaintenanceList, error) {
	maintenancelist := &metalv1alpha1.ServerMaintenanceList{}
	opts := []client.ListOption{
		client.InNamespace(replica.Namespace),
		client.MatchingLabels{ServerMaintenanceSetFinalizer: replica.Name},
	}
	if err := r.List(ctx, maintenancelist, opts...); err != nil {
		return nil, err
	}
	log := log.FromContext(ctx)
	for i := range maintenancelist.Items {
		log.V(1).Info("Found ServerMaintenance", "Status", maintenancelist.Items[i].Status)
	}
	return maintenancelist, nil
}

func (r *ServerMaintenanceSetReconciler) getServersBySelector(ctx context.Context, replica *metalv1alpha1.ServerMaintenanceSet) (*metalv1alpha1.ServerList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&replica.Spec.ServerSelector)
	if err != nil {
		return nil, err
	}
	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}

	return serverList, nil
}

func calculateStatus(maintenances []metalv1alpha1.ServerMaintenance) metalv1alpha1.ServerMaintenanceSetStatus {
	var newStatus metalv1alpha1.ServerMaintenanceSetStatus
	pendingCount := 0
	maintenanceCount := 0
	completedCount := 0
	failedCount := 0
	for _, m := range maintenances {
		switch m.Status.State {
		case metalv1alpha1.ServerMaintenanceStatePending:
			pendingCount++
		case metalv1alpha1.ServerMaintenanceStateInMaintenance:
			maintenanceCount++
		case metalv1alpha1.ServerMaintenanceStateCompleted:
			completedCount++
		case metalv1alpha1.ServerMaintenanceStateFailed:
			failedCount++
		}
	}
	newStatus.Maintenances = int32(len(maintenances))
	newStatus.Pending = int32(pendingCount)
	newStatus.Completed = int32(completedCount)
	newStatus.Failed = int32(failedCount)
	newStatus.InMaintenance = int32(maintenanceCount)
	return newStatus
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerMaintenanceSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerMaintenanceSet{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Named("servermaintenanceset").
		Complete(r)
}
