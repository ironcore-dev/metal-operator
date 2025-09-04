// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const BIOSVersionSetFinalizer = "metal.ironcore.dev/biosversionset"

// BIOSVersionSetReconciler reconciles a BIOSVersionSet object
type BIOSVersionSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversionsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversionsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversionsets/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biosversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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
	if !controllerutil.ContainsFinalizer(biosVersionSet, BIOSVersionSetFinalizer) {
		return ctrl.Result{}, nil
	}

	ownedBiosVersions, err := r.getOwnedBIOSVersions(ctx, biosVersionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned BIOSVersions: %w", err)
	}

	currentStatus := r.getOwnedBIOSVersionSetStatus(ownedBiosVersions)

	if currentStatus.AvailableBIOSVersion != (currentStatus.CompletedBIOSVersion+currentStatus.FailedBIOSVersion) ||
		biosVersionSet.Status.AvailableBIOSVersion != currentStatus.AvailableBIOSVersion {
		err = r.updateStatus(ctx, log, currentStatus, biosVersionSet)
		if err != nil {
			log.Error(err, "failed to update current Status")
			return ctrl.Result{}, fmt.Errorf("failed to update current BIOSVersionSet Status: %w", err)
		}
		log.Info("Waiting on the created BIOSVersion to reach terminal status")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosVersionSet, BIOSVersionSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("biosVersionSet is deleted")
	return ctrl.Result{}, nil
}

func (r *BIOSVersionSetReconciler) handleIgnoreAnnotationPropagation(
	ctx context.Context,
	log logr.Logger,
	biosVersionSet *metalv1alpha1.BIOSVersionSet,
) error {
	ownedBiosVersions, err := r.getOwnedBIOSVersions(ctx, biosVersionSet)
	if err != nil {
		return err
	}
	if len(ownedBiosVersions.Items) == 0 {
		log.V(1).Info("No BIOSVersion found, skipping ignore annotation propagation")
		return nil
	}
	var errs []error
	for _, biosVersion := range ownedBiosVersions.Items {
		// should not overwrite the already ignored annotation on child
		// should not overwrite if the annotation already present on the child
		if !isChildIgnoredThroughSets(&biosVersion) && !shouldIgnoreReconciliation(&biosVersion) {
			biosVersionBase := biosVersion.DeepCopy()
			annotations := biosVersion.GetAnnotations()
			annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.OperationAnnotationIgnore
			annotations[metalv1alpha1.PropogatedOperationAnnotation] = metalv1alpha1.PropogatedOperationAnnotationIgnored
			if err := r.Patch(ctx, &biosVersion, client.MergeFrom(biosVersionBase)); err != nil {
				errs = append(errs, fmt.Errorf("failed to patch BIOSVersion annotations: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSVersionSetReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	biosVersionSet *metalv1alpha1.BIOSVersionSet,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(biosVersionSet) {
		log.V(1).Info("Skipped BIOSVersionSet reconciliation")
		err := r.handleIgnoreAnnotationPropagation(ctx, log, biosVersionSet)
		return ctrl.Result{}, err
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, biosVersionSet, BIOSVersionSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	serverList, err := r.getServersBySelector(ctx, biosVersionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get servers by selector: %w", err)
	}

	ownedBiosVersions, err := r.getOwnedBIOSVersions(ctx, biosVersionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned BIOSVersions: %w", err)
	}

	log.V(1).Info("Summary of servers and BIOSVersions", "Server count", len(serverList.Items),
		"BIOSVersion count", len(ownedBiosVersions.Items))

	// create BIOSVersion for servers selected, if it does not exist
	if err := r.createMissingBIOSVersions(ctx, log, serverList, ownedBiosVersions, biosVersionSet); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create missing BIOSVersions: %w", err)
	}

	// delete BIOSVersion for servers which do not exist anymore
	if _, err := r.deleteOrphanBIOSVersions(ctx, log, serverList, ownedBiosVersions); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete orphaned BIOSVersions: %w", err)
	}

	if err := r.patchBIOSVersionfromTemplate(ctx, log, &biosVersionSet.Spec.BIOSVersionTemplate, ownedBiosVersions); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch BIOSVersion spec from template: %w", err)
	}

	log.V(1).Info("updating the status of BIOSVersionSet")
	currentStatus := r.getOwnedBIOSVersionSetStatus(ownedBiosVersions)
	currentStatus.FullyLabeledServers = int32(len(serverList.Items))

	err = r.updateStatus(ctx, log, currentStatus, biosVersionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update current BIOSVersionSet Status: %w", err)
	}
	// wait for any updates from owned resources
	return ctrl.Result{}, nil
}

func (r *BIOSVersionSetReconciler) createMissingBIOSVersions(
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
			var newBiosVersion *metalv1alpha1.BIOSVersion
			newBiosVersionName := fmt.Sprintf("%s-%s", biosVersionSet.Name, server.Name)
			if len(newBiosVersionName) > utilvalidation.DNS1123SubdomainMaxLength {
				log.V(1).Info("BiosVersion name is too long, it will be shortened using randam string", "name", newBiosVersionName)
				newBiosVersion = &metalv1alpha1.BIOSVersion{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: newBiosVersionName[:utilvalidation.DNS1123SubdomainMaxLength-10] + "-",
					}}
			} else {
				newBiosVersion = &metalv1alpha1.BIOSVersion{
					ObjectMeta: metav1.ObjectMeta{
						Name: newBiosVersionName,
					}}
			}

			opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, newBiosVersion, func() error {
				newBiosVersion.Spec.BIOSVersionTemplate = *biosVersionSet.Spec.BIOSVersionTemplate.DeepCopy()
				newBiosVersion.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
				return controllerutil.SetControllerReference(biosVersionSet, newBiosVersion, r.Client.Scheme())
			})
			if err != nil {
				errs = append(errs, err)
			}
			log.V(1).Info("Created BIOSVersion", "BIOSVersion", newBiosVersion.Name, "server ref", server.Name, "Operation", opResult)
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSVersionSetReconciler) deleteOrphanBIOSVersions(
	ctx context.Context,
	log logr.Logger,
	serverList *metalv1alpha1.ServerList,
	biosVersionList *metalv1alpha1.BIOSVersionList,
) ([]string, error) {

	serverMap := make(map[string]bool)
	for _, server := range serverList.Items {
		serverMap[server.Name] = true
	}

	var errs []error
	var warnings []string
	for _, biosVersion := range biosVersionList.Items {
		if !serverMap[biosVersion.Spec.ServerRef.Name] {
			if biosVersion.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
				log.V(1).Info("waiting for BIOSVersion to move out of InProgress state", "BIOSVersion", biosVersion.Name, "status", biosVersion.Status)
				warnings = append(warnings, fmt.Sprintf("BIOSVersion %s is still in progress, skipping deletion", biosVersion.Name))
				continue
			}
			if err := r.Delete(ctx, &biosVersion); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return warnings, errors.Join(errs...)
}

func (r *BIOSVersionSetReconciler) patchBIOSVersionfromTemplate(
	ctx context.Context,
	log logr.Logger,
	biosVersionTemplate *metalv1alpha1.BIOSVersionTemplate,
	biosVersionList *metalv1alpha1.BIOSVersionList,
) error {
	if len(biosVersionList.Items) == 0 {
		log.V(1).Info("No BIOSVersion found, skipping spec template update")
		return nil
	}

	var errs []error
	for _, biosVersion := range biosVersionList.Items {
		if biosVersion.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
			continue
		}
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &biosVersion, func() error {
			// serverMaintenanceRef might not be part of the patching template, so we do not patch if not provided
			if biosVersionTemplate.ServerMaintenanceRef != nil {
				biosVersion.Spec.BIOSVersionTemplate = *biosVersionTemplate.DeepCopy()
			} else {
				serverMaintenanceRef := biosVersion.Spec.ServerMaintenanceRef
				biosVersion.Spec.BIOSVersionTemplate = *biosVersionTemplate.DeepCopy()
				biosVersion.Spec.ServerMaintenanceRef = serverMaintenanceRef
			}
			return nil
		}) //nolint:errcheck
		if err != nil {
			errs = append(errs, err)
		}
		if opResult != controllerutil.OperationResultNone {
			log.V(1).Info("Patched BIOSVersion with updated spec", "BIOSVersions", biosVersion.Name, "Operation", opResult)
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSVersionSetReconciler) getOwnedBIOSVersionSetStatus(
	biosVersionList *metalv1alpha1.BIOSVersionList,
) *metalv1alpha1.BIOSVersionSetStatus {
	currentStatus := &metalv1alpha1.BIOSVersionSetStatus{}
	currentStatus.AvailableBIOSVersion = int32(len(biosVersionList.Items))
	for _, biosVersion := range biosVersionList.Items {
		switch biosVersion.Status.State {
		case metalv1alpha1.BIOSVersionStateCompleted:
			currentStatus.CompletedBIOSVersion += 1
		case metalv1alpha1.BIOSVersionStateFailed:
			currentStatus.FailedBIOSVersion += 1
		case metalv1alpha1.BIOSVersionStateInProgress:
			currentStatus.InProgressBIOSVersion += 1
		case metalv1alpha1.BIOSVersionStatePending, "":
			currentStatus.PendingBIOSVersion += 1
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

	biosVersionSet.Status = *currentStatus

	if err := r.Status().Patch(ctx, biosVersionSet, client.MergeFrom(biosVersionSetBase)); err != nil {
		return err
	}

	log.V(1).Info("Updated biosVersionSet state ", "new state", currentStatus)

	return nil
}

func (r *BIOSVersionSetReconciler) enqueueByServer(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	log.Info("server created/deleted/updated label")

	host := obj.(*metalv1alpha1.Server)

	biosVersionSetList := &metalv1alpha1.BIOSVersionSetList{}
	if err := r.List(ctx, biosVersionSetList); err != nil {
		log.V(1).Error(err, "failed to list BIOSVersionSet")
		return nil
	}
	reqs := make([]ctrl.Request, 0)
	for _, biosVersionSet := range biosVersionSetList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&biosVersionSet.Spec.ServerSelector)
		if err != nil {
			log.V(1).Error(err, "failed to convert label selector")
			return nil
		}
		// if the host label matches the selector, enqueue the request
		if selector.Matches(labels.Set(host.GetLabels())) {
			reqs = append(reqs, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name:      biosVersionSet.Name,
					Namespace: biosVersionSet.Namespace,
				},
			})
		} else { // if the label has been removed
			ownedBiosVersions, err := r.getOwnedBIOSVersions(ctx, &biosVersionSet)
			if err != nil {
				log.V(1).Error(err, "failed to get owned BIOSVersions")
				return nil
			}
			for _, biosVersion := range ownedBiosVersions.Items {
				if biosVersion.Spec.ServerRef.Name == host.Name {
					reqs = append(reqs, ctrl.Request{
						NamespacedName: client.ObjectKey{
							Name:      biosVersionSet.Name,
							Namespace: biosVersionSet.Namespace,
						},
					})
				}
			}
		}
	}
	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *BIOSVersionSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSVersionSet{}).
		Owns(&metalv1alpha1.BIOSVersion{}).
		Watches(&metalv1alpha1.Server{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueByServer),
			builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Complete(r)
}
