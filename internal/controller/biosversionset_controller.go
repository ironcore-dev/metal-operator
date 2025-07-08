// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const biosVersionSetFinalizer = "metal.ironcore.dev/biosVersionSet"

// BIOSVersionSetReconciler reconciles a BIOSVersionSet object
type BIOSVersionSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversionsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversionsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversionsets/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversion,verbs=get;list;watch;create;update;patch;delete
func (r *BIOSVersionSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	biosVersionSet := &metalv1alpha1.BIOSVersionSet{}
	if err := r.Get(ctx, req.NamespacedName, biosVersionSet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling biosVersionSet")

	return r.reconcileExists(ctx, log, biosVersionSet)
}

func (r *BIOSVersionSetReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	biosVersionSet *metalv1alpha1.BIOSVersionSet,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if !biosVersionSet.DeletionTimestamp.IsZero() {
		log.V(1).Info("object is being deleted")
		return r.delete(ctx, log, biosVersionSet)
	}

	return r.reconcile(ctx, log, biosVersionSet)
}

func (r *BIOSVersionSetReconciler) delete(
	ctx context.Context,
	log logr.Logger,
	biosVersionSet *metalv1alpha1.BIOSVersionSet,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(biosVersionSet, biosVersionSetFinalizer) {
		return ctrl.Result{}, nil
	}

	ownedBiosVersions, err := r.getOwnedBIOSVersions(ctx, biosVersionSet)
	if err != nil {
		return ctrl.Result{}, err
	}
	currentStatus := r.getOwnedBIOSVersionStatus(ownedBiosVersions)

	if currentStatus.TotalVersionResource != (currentStatus.Completed + currentStatus.Failed) {
		err = r.updateStatus(ctx, log, currentStatus, biosVersionSet)
		if err != nil {
			log.Error(err, "failed to update current Status")
			return ctrl.Result{}, err
		}
		log.Info("Waiting on the created BIOSVersion to reach terminal status")
		return ctrl.Result{}, nil
	}

	if err := r.deleteBIOSVersions(ctx, log, &metalv1alpha1.ServerList{}, ownedBiosVersions); err != nil {
		log.Error(err, "failed to cleanup created resources")
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured cleaning up")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosVersionSet, biosVersionSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("biosVersionSet is deleted")
	return ctrl.Result{}, nil
}

func (r *BIOSVersionSetReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	biosVersionSet *metalv1alpha1.BIOSVersionSet,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(biosVersionSet) {
		log.V(1).Info("Skipped BIOSVersionSet reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, biosVersionSet, biosVersionSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	serverList, err := r.getServersBySelector(ctx, biosVersionSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	ownedBiosVersions, err := r.getOwnedBIOSVersions(ctx, biosVersionSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(serverList.Items) > len(ownedBiosVersions.Items) {
		log.V(1).Info("new server found creating respective BIOSVersion")
		if err := r.createBIOSVersions(ctx, log, serverList, ownedBiosVersions, biosVersionSet); err != nil {
			log.Error(err, "failed to create resources")
			return ctrl.Result{}, err
		}
		// wait for any updates from owned resources
		return ctrl.Result{}, nil
	} else if len(serverList.Items) < len(ownedBiosVersions.Items) {
		log.V(1).Info("servers deleted, deleting respective BIOSVersion")
		if err := r.deleteBIOSVersions(ctx, log, serverList, ownedBiosVersions); err != nil {
			log.Error(err, "failed to cleanup resources")
			return ctrl.Result{}, err
		}
		// wait for any updates from owned resources
		return ctrl.Result{}, nil
	}

	if len(serverList.Items) == 0 {
		log.V(1).Info("No matching Servers found reconciliation")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("updating the status of BIOSVersionSet")
	currentStatus := r.getOwnedBIOSVersionStatus(ownedBiosVersions)
	currentStatus.TotalServers = int32(len(serverList.Items))

	err = r.updateStatus(ctx, log, currentStatus, biosVersionSet)
	if err != nil {
		log.Error(err, "failed to update current Status")
		return ctrl.Result{}, err
	}
	// wait for any updates from owned resources
	return ctrl.Result{}, nil
}

func (r *BIOSVersionSetReconciler) createBIOSVersions(
	ctx context.Context,
	log logr.Logger,
	serverList *metalv1alpha1.ServerList,
	biosVersionList *metalv1alpha1.BIOSVersionList,
	biosVersionSet *metalv1alpha1.BIOSVersionSet,
) error {

	serverWithBiosVersion := make(map[string]bool)
	for _, biosVersion := range biosVersionList.Items {
		serverWithBiosVersion[biosVersion.Spec.ServerRef.Name] = true
	}

	var errs []error
	for _, server := range serverList.Items {
		if !serverWithBiosVersion[server.Name] {
			newBiosVersion := &metalv1alpha1.BIOSVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-%s", biosVersionSet.Name, server.Name),
				}}

			opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, newBiosVersion, func() error {
				newBiosVersion.Spec.ServerMaintenancePolicy = biosVersionSet.Spec.ServerMaintenancePolicy
				newBiosVersion.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
				newBiosVersion.Spec.Version = biosVersionSet.Spec.Version
				newBiosVersion.Spec.Image = biosVersionSet.Spec.Image
				newBiosVersion.Spec.UpdatePolicy = biosVersionSet.Spec.UpdatePolicy
				return controllerutil.SetControllerReference(biosVersionSet, newBiosVersion, r.Client.Scheme())
			})
			if err != nil {
				errs = append(errs, err)
			}
			log.V(1).Info("Created biosVersion", "BIOSVersion", newBiosVersion.Name, "server ref", server.Name, "Operation", opResult)
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSVersionSetReconciler) deleteBIOSVersions(
	ctx context.Context,
	log logr.Logger,
	serverList *metalv1alpha1.ServerList,
	biosVersionList *metalv1alpha1.BIOSVersionList,
) error {

	serverMap := make(map[string]bool)
	for _, server := range serverList.Items {
		serverMap[server.Name] = true
	}

	var errs []error
	for _, biosVersion := range biosVersionList.Items {
		if !serverMap[biosVersion.Spec.ServerRef.Name] {
			if biosVersion.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
				log.V(1).Info("waiting for BIOSVersion to move out of InProgress state", "BIOSVersion", biosVersion.Name, "status", biosVersion.Status)
				continue
			}
			if err := r.Delete(ctx, &biosVersion); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

func (r *BIOSVersionSetReconciler) getOwnedBIOSVersionStatus(
	biosVersionList *metalv1alpha1.BIOSVersionList,
) *metalv1alpha1.BIOSVersionSetStatus {
	currentStatus := &metalv1alpha1.BIOSVersionSetStatus{}
	currentStatus.TotalVersionResource = int32(len(biosVersionList.Items))
	for _, biosVersion := range biosVersionList.Items {
		switch biosVersion.Status.State {
		case metalv1alpha1.BIOSVersionStateCompleted:
			currentStatus.Completed += 1
		case metalv1alpha1.BIOSVersionStateFailed:
			currentStatus.Failed += 1
		case metalv1alpha1.BIOSVersionStateInProgress:
			currentStatus.InProgress += 1
		case metalv1alpha1.BIOSVersionStatePending, "":
			currentStatus.InProgress += 1
		}
	}
	return currentStatus
}

func (r *BIOSVersionSetReconciler) getOwnedBIOSVersions(
	ctx context.Context,
	biosVersionSet *metalv1alpha1.BIOSVersionSet,
) (*metalv1alpha1.BIOSVersionList, error) {
	biosVersionList := &metalv1alpha1.BIOSVersionList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, biosVersionSet, biosVersionList); err != nil {
		return nil, err
	}
	return biosVersionList, nil
}

func (r *BIOSVersionSetReconciler) getServersBySelector(
	ctx context.Context,
	biosVersionSet *metalv1alpha1.BIOSVersionSet,
) (*metalv1alpha1.ServerList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&biosVersionSet.Spec.ServerSelector)
	if err != nil {
		return nil, err
	}
	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}

	return serverList, nil
}

func (r *BIOSVersionSetReconciler) updateStatus(
	ctx context.Context,
	log logr.Logger,
	currentStatus *metalv1alpha1.BIOSVersionSetStatus,
	biosVersionSet *metalv1alpha1.BIOSVersionSet,
) error {

	biosVersionSetBase := biosVersionSet.DeepCopy()

	biosVersionSet.Status.Completed = currentStatus.Completed
	biosVersionSet.Status.TotalServers = currentStatus.TotalServers
	biosVersionSet.Status.TotalVersionResource = currentStatus.TotalVersionResource
	biosVersionSet.Status.Failed = currentStatus.Failed
	biosVersionSet.Status.InProgress = currentStatus.InProgress
	biosVersionSet.Status.Pending = currentStatus.Pending

	if err := r.Status().Patch(ctx, biosVersionSet, client.MergeFrom(biosVersionSetBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSVersionSet status: %w", err)
	}

	log.V(1).Info("Updated biosVersionSet state ", "new state", currentStatus)

	return nil
}

func (r *BIOSVersionSetReconciler) enqueueBiosVersionSetByServersListChanges(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)

	// there is no change in server list (as its not deleted or created)
	if host.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(host, ServerFinalizer) {
		return nil
	}

	biosVersionSetList := &metalv1alpha1.BIOSVersionSetList{}
	if err := r.List(ctx, biosVersionSetList); err != nil {
		log.Error(err, "failed to list BIOSVersionSet")
		return nil
	}

	for _, biosVersionSet := range biosVersionSetList.Items {
		serverList, err := r.getServersBySelector(ctx, &biosVersionSet)
		if err != nil {
			log.Error(err, "failed to list BIOSVersionSet servers by selector")
			return nil
		}
		for _, server := range serverList.Items {
			if server.Name == host.Name {
				ownedBiosVersion, err := r.getOwnedBIOSVersions(ctx, &biosVersionSet)
				if err != nil {
					log.Error(err, "failed to list owned BIOSVersions")
					return nil
				}
				if len(ownedBiosVersion.Items) != len(serverList.Items) {
					return []ctrl.Request{{
						NamespacedName: client.ObjectKey{
							Name:      biosVersionSet.Name,
							Namespace: biosVersionSet.Namespace,
						}}}
				}
			}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BIOSVersionSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSVersionSet{}).
		Owns(&metalv1alpha1.BIOSVersion{}).
		Watches(&metalv1alpha1.Server{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosVersionSetByServersListChanges)).
		Complete(r)
}
