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

// BMCVersionSetReconciler reconciles a BMCVersionSet object
type BMCVersionSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const BMCVersionSetFinalizer = "metal.ironcore.dev/bmcversionset"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcversionsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcversionsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcversionsets/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcversions,verbs=get;list;watch;create;update;patch;delete

func (r *BMCVersionSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bmcVersionSet := &metalv1alpha1.BMCVersionSet{}
	if err := r.Get(ctx, req.NamespacedName, bmcVersionSet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling BMCVersionSet")

	return r.reconcileExists(ctx, log, bmcVersionSet)
}

func (r *BMCVersionSetReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if !bmcVersionSet.DeletionTimestamp.IsZero() {
		log.V(1).Info("object is being deleted")
		return r.delete(ctx, log, bmcVersionSet)
	}

	return r.reconcile(ctx, log, bmcVersionSet)
}

func (r *BMCVersionSetReconciler) delete(
	ctx context.Context,
	log logr.Logger,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(bmcVersionSet, BMCVersionSetFinalizer) {
		return ctrl.Result{}, nil
	}

	ownedBMCVersions, err := r.getOwnedBMCVersions(ctx, bmcVersionSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	currentStatus := r.getOwnedBMCVersionSetStatus(ownedBMCVersions)

	if currentStatus.AvailableBMCVersion != (currentStatus.CompletedBMCVersion+currentStatus.FailedBMCVersion) ||
		bmcVersionSet.Status.AvailableBMCVersion != currentStatus.AvailableBMCVersion {
		err = r.updateStatus(ctx, log, currentStatus, bmcVersionSet)
		if err != nil {
			log.Error(err, "failed to update current Status")
			return ctrl.Result{}, err
		}
		log.Info("Waiting on the created BMCVersion to reach terminal status")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, bmcVersionSet, BMCVersionSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("BMCVersionSet is deleted")
	return ctrl.Result{}, nil
}

func (r *BMCVersionSetReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(bmcVersionSet) {
		log.V(1).Info("Skipped BMCVersionSet reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, bmcVersionSet, BMCVersionSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	bmcList, err := r.getBMCBySelector(ctx, bmcVersionSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	ownedBMCVersions, err := r.getOwnedBMCVersions(ctx, bmcVersionSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Summary of BMC and BMCVersions", "BMCs count", len(bmcList.Items),
		"BMCVersion count", len(ownedBMCVersions.Items))

	// create BMCVersion for BMCs selected, if it does not exist
	if err := r.createMissingBMCVersions(ctx, log, bmcList, ownedBMCVersions, bmcVersionSet); err != nil {
		log.Error(err, "failed to create resources")
		return ctrl.Result{}, err
	}

	// delete BMCVersion for BMCs which do not exist anymore
	if _, err := r.deleteOrphanBMCVersions(ctx, log, bmcList, ownedBMCVersions); err != nil {
		log.Error(err, "failed to cleanup resources")
		return ctrl.Result{}, err
	}

	if err := r.patchOrCreateBMCVersionfromTemplate(ctx, log, &bmcVersionSet.Spec.BMCVersionTemplate, ownedBMCVersions); err != nil {
		log.Error(err, "failed to update biosSettings specs")
		return ctrl.Result{}, err
	}

	log.V(1).Info("updating the status of BMCVersionSet")
	currentStatus := r.getOwnedBMCVersionSetStatus(ownedBMCVersions)
	currentStatus.FullyLabeledBMCs = int32(len(bmcList.Items))

	err = r.updateStatus(ctx, log, currentStatus, bmcVersionSet)
	if err != nil {
		log.Error(err, "failed to update current Status")
		return ctrl.Result{}, err
	}
	// wait for any updates from owned resources
	return ctrl.Result{}, nil
}

func (r *BMCVersionSetReconciler) createMissingBMCVersions(
	ctx context.Context,
	log logr.Logger,
	bmcList *metalv1alpha1.BMCList,
	bmcVersionList *metalv1alpha1.BMCVersionList,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) error {

	BMCWithBMCVersion := make(map[string]struct{})
	for _, bmcVersion := range bmcVersionList.Items {
		BMCWithBMCVersion[bmcVersion.Spec.BMCRef.Name] = struct{}{}
	}

	var errs []error
	for _, bmc := range bmcList.Items {
		if _, ok := BMCWithBMCVersion[bmc.Name]; !ok {
			newBMCVersion := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "bmc-version-set-",
				}}

			opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, newBMCVersion, func() error {
				newBMCVersion.Spec.BMCVersionTemplate = *bmcVersionSet.Spec.BMCVersionTemplate.DeepCopy()
				newBMCVersion.Spec.BMCRef = &corev1.LocalObjectReference{Name: bmc.Name}
				return controllerutil.SetControllerReference(bmcVersionSet, newBMCVersion, r.Client.Scheme())
			})
			if err != nil {
				errs = append(errs, err)
			}
			log.V(1).Info("Created BMCVersion", "BMCVersion", newBMCVersion.Name, "BMC ref", bmc.Name, "Operation", opResult)
		}
	}
	return errors.Join(errs...)
}

func (r *BMCVersionSetReconciler) deleteOrphanBMCVersions(
	ctx context.Context,
	log logr.Logger,
	bmcList *metalv1alpha1.BMCList,
	bmcVersionList *metalv1alpha1.BMCVersionList,
) ([]string, error) {

	bmcMap := make(map[string]struct{})
	for _, bmc := range bmcList.Items {
		bmcMap[bmc.Name] = struct{}{}
	}

	var errs []error
	var warnings []string
	for _, bmcVersion := range bmcVersionList.Items {
		if _, ok := bmcMap[bmcVersion.Spec.BMCRef.Name]; !ok {
			if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateInProgress {
				log.V(1).Info("waiting for BMCVersion to move out of InProgress state", "BMCVersion", bmcVersion.Name, "status", bmcVersion.Status)
				warnings = append(warnings, fmt.Sprintf("BMCVersion %s is still in progress, skipping deletion", bmcVersion.Name))
				continue
			}
			if err := r.Delete(ctx, &bmcVersion); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return warnings, errors.Join(errs...)
}

func (r *BMCVersionSetReconciler) patchOrCreateBMCVersionfromTemplate(
	ctx context.Context,
	log logr.Logger,
	bmcVersionTemplate *metalv1alpha1.BMCVersionTemplate,
	bmcVersionList *metalv1alpha1.BMCVersionList,
) error {
	if len(bmcVersionList.Items) == 0 {
		log.V(1).Info("No BMCVersion found, skipping spec template update")
		return nil
	}

	var errs []error
	for _, bmcVersion := range bmcVersionList.Items {
		if bmcVersion.Status.State != metalv1alpha1.BMCVersionStateInProgress {
			opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &bmcVersion, func() error {
				bmcVersion.Spec.BMCVersionTemplate = *bmcVersionTemplate.DeepCopy()
				return nil
			}) //nolint:errcheck
			if err != nil {
				errs = append(errs, err)
			}
			if opResult != controllerutil.OperationResultNone {
				log.V(1).Info("Patched BMCVersion with updated spec", "BMCVersions", bmcVersion.Name, "Operation", opResult)
			}
		}
	}
	return errors.Join(errs...)
}

func (r *BMCVersionSetReconciler) getOwnedBMCVersionSetStatus(
	bmcVersionList *metalv1alpha1.BMCVersionList,
) *metalv1alpha1.BMCVersionSetStatus {
	currentStatus := &metalv1alpha1.BMCVersionSetStatus{}
	currentStatus.AvailableBMCVersion = int32(len(bmcVersionList.Items))
	for _, bmcVersion := range bmcVersionList.Items {
		switch bmcVersion.Status.State {
		case metalv1alpha1.BMCVersionStateCompleted:
			currentStatus.CompletedBMCVersion += 1
		case metalv1alpha1.BMCVersionStateFailed:
			currentStatus.FailedBMCVersion += 1
		case metalv1alpha1.BMCVersionStateInProgress:
			currentStatus.InProgressBMCVersion += 1
		case metalv1alpha1.BMCVersionStatePending, "":
			currentStatus.PendingBMCVersion += 1
		}
	}
	return currentStatus
}

func (r *BMCVersionSetReconciler) getOwnedBMCVersions(
	ctx context.Context,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) (*metalv1alpha1.BMCVersionList, error) {
	bmcVersionList := &metalv1alpha1.BMCVersionList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, bmcVersionSet, bmcVersionList); err != nil {
		return nil, err
	}
	return bmcVersionList, nil
}

func (r *BMCVersionSetReconciler) getBMCBySelector(
	ctx context.Context,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) (*metalv1alpha1.BMCList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&bmcVersionSet.Spec.BMCSelector)
	if err != nil {
		return nil, err
	}
	bmcList := &metalv1alpha1.BMCList{}
	if err := r.List(ctx, bmcList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}

	return bmcList, nil
}

func (r *BMCVersionSetReconciler) updateStatus(
	ctx context.Context,
	log logr.Logger,
	currentStatus *metalv1alpha1.BMCVersionSetStatus,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) error {

	bmcVersionSetBase := bmcVersionSet.DeepCopy()

	bmcVersionSet.Status = *currentStatus

	if err := r.Status().Patch(ctx, bmcVersionSet, client.MergeFrom(bmcVersionSetBase)); err != nil {
		return fmt.Errorf("failed to patch BMCVersionSet status: %w", err)
	}

	log.V(1).Info("Updated BMCVersionSet state ", "new state", currentStatus)

	return nil
}

func (r *BMCVersionSetReconciler) enqueueByBMC(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)

	host := obj.(*metalv1alpha1.BMC)

	bmcVersionSetList := &metalv1alpha1.BMCVersionSetList{}
	if err := r.List(ctx, bmcVersionSetList); err != nil {
		log.Error(err, "failed to list BMCVersionSet")
		return nil
	}
	reqs := make([]ctrl.Request, 0)
	for _, bmcVersionSet := range bmcVersionSetList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&bmcVersionSet.Spec.BMCSelector)
		if err != nil {
			log.Error(err, "failed to convert label selector")
			return nil
		}
		// if the host label matches the selector, enqueue the request
		if selector.Matches(labels.Set(host.GetLabels())) {
			reqs = append(reqs, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name:      bmcVersionSet.Name,
					Namespace: bmcVersionSet.Namespace,
				},
			})
		} else { // if the label has been removed
			ownedBMCVersions, err := r.getOwnedBMCVersions(ctx, &bmcVersionSet)
			if err != nil {
				return nil
			}
			for _, bmcVersion := range ownedBMCVersions.Items {
				if bmcVersion.Spec.BMCRef.Name == host.Name {
					reqs = append(reqs, ctrl.Request{
						NamespacedName: client.ObjectKey{
							Name:      bmcVersionSet.Name,
							Namespace: bmcVersionSet.Namespace,
						},
					})
				}
			}
		}
	}
	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCVersionSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMCVersionSet{}).
		Owns(&metalv1alpha1.BMCVersion{}).
		Watches(&metalv1alpha1.BMC{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueByBMC),
			builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Complete(r)
}
