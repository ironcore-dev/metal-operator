// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerMaintenanceSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	maintenanceSet := &metalv1alpha1.ServerMaintenanceSet{}
	if err := r.Get(ctx, req.NamespacedName, maintenanceSet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, maintenanceSet)
}

func (r *ServerMaintenanceSetReconciler) reconcileExists(ctx context.Context, maintenanceSet *metalv1alpha1.ServerMaintenanceSet) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	if !maintenanceSet.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, maintenanceSet)
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, maintenanceSet, ServerMaintenanceSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	return r.reconcile(ctx, log, maintenanceSet)
}

func (r *ServerMaintenanceSetReconciler) delete(ctx context.Context, log logr.Logger, maintenanceSet *metalv1alpha1.ServerMaintenanceSet) (ctrl.Result, error) {
	log.V(1).Info("Deleting ServerMaintenanceSet")
	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, maintenanceSet, ServerMaintenanceSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Deleted ServerMaintenanceSet")
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceSetReconciler) reconcile(ctx context.Context, log logr.Logger, maintenanceSet *metalv1alpha1.ServerMaintenanceSet) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerMaintenanceSet", "Name", maintenanceSet.Name)
	if shouldIgnoreReconciliation(maintenanceSet) {
		log.V(1).Info("Skipped reconciliation for ServerMaintenanceSet", "Name", maintenanceSet.Name)
		return ctrl.Result{}, nil
	}
	servers, err := r.getServersBySelector(ctx, maintenanceSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(servers.Items) == 0 {
		log.V(0).Info("No servers found")
		return ctrl.Result{}, nil
	}

	maintenancelist, err := r.getOwnedMaintenances(ctx, maintenanceSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Fetched ServerMaintenances", "Count", len(maintenancelist.Items))
	// If there are no existing maintenances, create them
	if err = r.createMaintenances(ctx, log, maintenanceSet, maintenancelist, servers); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create maintenances: %w", err)
	}
	// If there are existing maintenances, check if any are orphaned and delete them
	if err = r.deleteOrphanedMaintenances(ctx, log, maintenancelist, servers); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete orphaned maintenances: %w", err)
	}

	// Fetch the list of maintenances again to get the updated status
	maintenancelist, err = r.getOwnedMaintenances(ctx, maintenanceSet)
	if err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Fetched ServerMaintenances", "Count", len(maintenancelist.Items))
	if modified, err := r.patchStatus(ctx, maintenanceSet, calculateStatus(maintenancelist.Items)); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Reconciled ServerMaintenanceSet", "Name", maintenanceSet.Name)

	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceSetReconciler) createMaintenances(
	ctx context.Context,
	log logr.Logger,
	maintenanceSet *metalv1alpha1.ServerMaintenanceSet,
	maintenancelist *metalv1alpha1.ServerMaintenanceList,
	serverList *metalv1alpha1.ServerList,
) error {
	var errs error
	var createdMaintenances []string
	for _, maintenance := range maintenancelist.Items {
		if maintenance.Spec.ServerRef != nil {
			createdMaintenances = append(createdMaintenances, maintenance.Spec.ServerRef.Name)
		}
	}
	// Iterate through the servers that should be managed by this set.
	for _, server := range serverList.Items {
		log.V(1).Info("Reconciling server", "server", server.Name)

		if !server.DeletionTimestamp.IsZero() {
			log.V(1).Info("Server is marked for deletion, skipping", "server", server.Name)
			continue
		}

		// Check the map to see if maintenance already exists.
		if slices.Contains(createdMaintenances, server.Name) {
			log.V(1).Info("maintenance already created, skipping", "server", server.Name)
			continue
		}
		maintenanceName := truncateString(fmt.Sprintf("%s-%s-", maintenanceSet.Name, server.Name), utilvalidation.DNS1123SubdomainMaxLength-5)
		maintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: maintenanceName,
				Namespace:    maintenanceSet.Namespace,
			},
		}
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, maintenance, func() error {
			metautils.SetLabels(maintenance, map[string]string{ServerMaintenanceSetFinalizer: maintenanceSet.Name})
			maintenance.Spec = metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:                 &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenanceTemplate: maintenanceSet.Spec.ServerMaintenanceTemplate,
			}
			return controllerutil.SetControllerReference(maintenanceSet, maintenance, r.Client.Scheme())
		})
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to create or patch serverMaintenance %s: %w", maintenance.Name, err))
			continue
		}
		log.V(1).Info("Created or patched ServerMaintenance", "ServerMaintenance", maintenance.Name, "Operation", opResult)
	}
	return errs
}

func (r *ServerMaintenanceSetReconciler) deleteOrphanedMaintenances(
	ctx context.Context,
	log logr.Logger,
	maintenancelist *metalv1alpha1.ServerMaintenanceList,
	serverList *metalv1alpha1.ServerList,
) error {
	var errs error
	var serverNames []string
	for _, server := range serverList.Items {
		if server.DeletionTimestamp.IsZero() {
			serverNames = append(serverNames, server.Name)
		}
	}
	for _, maintenance := range maintenancelist.Items {
		log.V(1).Info("Checking for orphaned maintenance", "maintenance", maintenance.Name)
		// Check if the maintenance is part of the server maintenance set
		if !slices.Contains(serverNames, maintenance.Spec.ServerRef.Name) {
			log.V(1).Info("Maintenance is orphaned, deleting", "maintenance", maintenance.Name)
			if err := r.Delete(ctx, &maintenance); err != nil {
				errs = errors.Join(errs, fmt.Errorf("failed to delete orphaned maintenance %s: %w", maintenance.Name, err))
			} else {
				log.V(1).Info("Deleted orphaned maintenance", "maintenance", maintenance.Name)
			}
			continue
		}
	}
	return errs
}

func (r *ServerMaintenanceSetReconciler) patchStatus(ctx context.Context, maintenanceSet *metalv1alpha1.ServerMaintenanceSet, status metalv1alpha1.ServerMaintenanceSetStatus) (bool, error) {
	if maintenanceSet.Status == status {
		return false, nil
	}
	base := maintenanceSet.DeepCopy()
	maintenanceSet.Status = status
	if err := r.Status().Patch(ctx, maintenanceSet, client.MergeFrom(base)); err != nil {
		return false, err
	}

	return true, nil
}

func (r *ServerMaintenanceSetReconciler) getOwnedMaintenances(ctx context.Context, maintenanceSet *metalv1alpha1.ServerMaintenanceSet) (*metalv1alpha1.ServerMaintenanceList, error) {
	maintenancelist := &metalv1alpha1.ServerMaintenanceList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, maintenanceSet, maintenancelist); err != nil {
		return nil, err
	}
	return maintenancelist, nil
}

func (r *ServerMaintenanceSetReconciler) getServersBySelector(ctx context.Context, maintenanceSet *metalv1alpha1.ServerMaintenanceSet) (*metalv1alpha1.ServerList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&maintenanceSet.Spec.ServerSelector)
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
	failedCount := 0
	completed := 0

	for _, m := range maintenances {
		switch m.Status.State {
		case metalv1alpha1.ServerMaintenanceStatePending:
			pendingCount++
		case metalv1alpha1.ServerMaintenanceStateInMaintenance:
			maintenanceCount++
		case metalv1alpha1.ServerMaintenanceStateFailed:
			failedCount++
		case metalv1alpha1.ServerMaintenanceStateCompleted:
			completed++
		}
	}
	newStatus.Maintenances = int32(len(maintenances))
	newStatus.Pending = int32(pendingCount)
	newStatus.Failed = int32(failedCount)
	newStatus.InMaintenance = int32(maintenanceCount)
	newStatus.Completed = int32(completed)
	return newStatus
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerMaintenanceSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerMaintenanceSet{}).
		Owns(&metalv1alpha1.ServerMaintenance{}).
		Watches(&metalv1alpha1.Server{}, r.enqueueMaintenanceSetByServers()).
		Named("servermaintenanceset").
		Complete(r)
}

func (r *ServerMaintenanceSetReconciler) enqueueMaintenanceSetByServers() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)
		server := object.(*metalv1alpha1.Server)
		var req []reconcile.Request

		maintenanceSetList := &metalv1alpha1.ServerMaintenanceSetList{}
		if err := r.List(ctx, maintenanceSetList); err != nil {
			log.Error(err, "failed to list host serverMaintenances")
			return nil
		}
		for _, ms := range maintenanceSetList.Items {
			selector, err := metav1.LabelSelectorAsSelector(&ms.Spec.ServerSelector)
			if err != nil {
				log.Error(err, "failed to convert label selector", "selector", ms.Spec.ServerSelector)
				continue
			}
			if selector.Matches(labels.Set(server.Labels)) {
				log.V(1).Info("Found ServerMaintenanceSet matching the server", "Server", server.Name, "MaintenanceSet", ms.Name)
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: ms.Namespace, Name: ms.Name},
				})
				continue
			} else {
				maintenances, err := r.getOwnedMaintenances(ctx, &ms)
				if err != nil {
					log.Error(err, "failed to get owned maintenances for ServerMaintenanceSet", "ServerMaintenanceSet", ms.Name)
					continue
				}
				for _, maintenance := range maintenances.Items {
					if maintenance.Spec.ServerRef != nil && maintenance.Spec.ServerRef.Name == server.Name {
						req = append(req, reconcile.Request{
							NamespacedName: types.NamespacedName{Namespace: ms.Namespace, Name: ms.Name},
						})
					}
				}
			}
		}
		return req
	})
}
