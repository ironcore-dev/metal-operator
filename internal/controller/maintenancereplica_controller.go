// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const (
	ServerMaintenanceReplicaFinalizer = "metal.ironcore.dev/ServerMaintenanceReplica"
)

// ServerMaintenanceReplicaReconciler reconciles a ServerMaintenanceReplica object
type ServerMaintenanceReplicaReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=ServerMaintenanceReplicas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=ServerMaintenanceReplicas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=ServerMaintenanceReplicas/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServerMaintenanceReplica object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *ServerMaintenanceReplicaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	replica := &metalv1alpha1.ServerMaintenanceReplica{}

	if err := r.Get(ctx, req.NamespacedName, replica); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, replica)
}

func (r *ServerMaintenanceReplicaReconciler) reconcileExists(ctx context.Context, replica *metalv1alpha1.ServerMaintenanceReplica) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	if !replica.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, replica)
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, replica, ServerMaintenanceReplicaFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	return r.reconcile(ctx, log, replica)
}

func (r *ServerMaintenanceReplicaReconciler) delete(ctx context.Context, log logr.Logger, replica *metalv1alpha1.ServerMaintenanceReplica) (ctrl.Result, error) {
	log.V(1).Info("Deleting ServerMaintenanceReplica")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, replica, ServerMaintenanceReplicaFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Deleted ServerMaintenanceReplica")
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReplicaReconciler) reconcile(ctx context.Context, log logr.Logger, replica *metalv1alpha1.ServerMaintenanceReplica) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerMaintenanceReplica")
	servers, err := r.getServersBySelector(ctx, replica)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(servers.Items) == 0 {
		log.V(1).Info("No servers found")
		return ctrl.Result{}, nil
	}

	maintenancelist := &metalv1alpha1.ServerMaintenanceList{}
	if err = r.List(ctx, maintenancelist, client.MatchingLabels{
		ServerMaintenanceReplicaFinalizer: replica.Name,
	}); err != nil {
		return ctrl.Result{}, err
	}

	// If all servers have a maintenance object, only update the status
	if len(servers.Items) == len(maintenancelist.Items) {
		if modified, err := r.patchStatus(ctx, replica, calculateStatus(replica, maintenancelist.Items)); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

out:
	for _, server := range servers.Items {
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
				Name:      replica.Name,
				Namespace: replica.Namespace,
				Labels: map[string]string{
					ServerMaintenanceReplicaFinalizer: replica.Name,
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:                       v1.LocalObjectReference{Name: server.Name},
				Policy:                          replica.Spec.Template.Policy,
				ServerPower:                     replica.Spec.Template.ServerPower,
				ServerBootConfigurationTemplate: replica.Spec.Template.ServerBootConfigurationTemplate,
			},
		}
		if err := r.Create(ctx, &maintenance); err != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("Created ServerMaintenance", "Name", maintenance.Name)

		if err := controllerutil.SetControllerReference(replica, &maintenance, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Fetch the list of maintenances again to get the updated status
	opts := []client.ListOption{
		client.InNamespace(replica.Namespace),
		client.MatchingLabels{ServerMaintenanceReplicaFinalizer: replica.Name},
	}
	if err = r.List(ctx, maintenancelist, opts...); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Fetched ServerMaintenances", "Count", len(maintenancelist.Items))
	if modified, err := r.patchStatus(ctx, replica, calculateStatus(replica, maintenancelist.Items)); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Reconciled ServerMaintenanceReplica", "Name", replica.Name)

	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReplicaReconciler) patchStatus(ctx context.Context, replica *metalv1alpha1.ServerMaintenanceReplica, status metalv1alpha1.ServerMaintenanceReplicaStatus) (bool, error) {
	if replica.Status == status {
		return false, nil
	}
	replica.Status = status
	if err := r.Status().Update(ctx, replica); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ServerMaintenanceReplicaReconciler) getServersBySelector(ctx context.Context, replica *metalv1alpha1.ServerMaintenanceReplica) (*metalv1alpha1.ServerList, error) {
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

func calculateStatus(replica *metalv1alpha1.ServerMaintenanceReplica, maintenances []metalv1alpha1.ServerMaintenance) metalv1alpha1.ServerMaintenanceReplicaStatus {
	newStatus := replica.Status
	pendingReplicasCount := 0
	ServerMaintenanceReplicasCount := 0
	completedReplicasCount := 0
	failedReplicasCpunt := 0
	for _, m := range maintenances {
		if m.Status.State == metalv1alpha1.ServerMaintenanceStateInMaintenance {
			ServerMaintenanceReplicasCount++
		}
		if m.Status.State == metalv1alpha1.ServerMaintenanceStatePending {
			pendingReplicasCount++
		}
		if m.Status.State == metalv1alpha1.ServerMaintenanceStateCompleted {
			completedReplicasCount++
		}
		if m.Status.State == metalv1alpha1.ServerMaintenanceStateFailed {
			failedReplicasCpunt++
		}
	}
	newStatus.Replicas = int32(len(maintenances))
	newStatus.ServerMaintenanceReplicas = int32(ServerMaintenanceReplicasCount)
	newStatus.PendingReplicas = int32(pendingReplicasCount)
	newStatus.CompletedReplicas = int32(completedReplicasCount)
	newStatus.FailedReplicas = int32(failedReplicasCpunt)
	return newStatus
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerMaintenanceReplicaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerMaintenanceReplica{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Named("ServerMaintenanceReplica").
		Complete(r)
}
