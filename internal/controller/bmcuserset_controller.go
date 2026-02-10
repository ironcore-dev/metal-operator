// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
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
)

type BMCUserSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const BMCUserSetFinalizer = "metal.ironcore.dev/bmcuserset"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusersets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusersets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusersets/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch

func (r *BMCUserSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bmcUserSet := &metalv1alpha1.BMCUserSet{}
	if err := r.Get(ctx, req.NamespacedName, bmcUserSet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling BMCUserSet")
	return r.reconcileExists(ctx, log, bmcUserSet)
}

func (r *BMCUserSetReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	bmcUserSet *metalv1alpha1.BMCUserSet,
) (ctrl.Result, error) {
	if !bmcUserSet.DeletionTimestamp.IsZero() {
		log.V(1).Info("Object is being deleted")
		return r.delete(ctx, log, bmcUserSet)
	}
	log.V(1).Info("Object exists and is not being deleted")
	return r.reconcile(ctx, log, bmcUserSet)
}

func (r *BMCUserSetReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	bmcUserSet *metalv1alpha1.BMCUserSet,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(bmcUserSet) {
		log.V(1).Info("Skipped BMCUserSet reconciliation")
		return ctrl.Result{}, nil
	}
	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, bmcUserSet, BMCUserSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	bmcList, err := r.getBMCsBySelector(ctx, bmcUserSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMCs through label selector: %w", err)
	}

	ownedBMCUsers, err := r.getOwnedBMCUsers(ctx, bmcUserSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list owned BMCUsers: %w", err)
	}

	if err := r.createMissingBMCUsers(ctx, log, bmcList, ownedBMCUsers, bmcUserSet); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create missing BMCUsers: %w", err)
	}

	if err := r.deleteOrphanedBMCUsers(ctx, log, bmcList, ownedBMCUsers); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete orphaned BMCUsers: %w", err)
	}

	if err := r.patchBMCUsersFromTemplate(ctx, log, bmcUserSet, ownedBMCUsers); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch BMCUsers from template: %w", err)
	}

	log.V(1).Info("Updating BMCUserSet status")
	currentStatus := r.getOwnedBMCUserSetStatus(ownedBMCUsers)
	currentStatus.FullyLabeledBMCs = int32(len(bmcList.Items))
	if err := r.updateStatus(ctx, log, currentStatus, bmcUserSet); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update current BMCUserSet status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *BMCUserSetReconciler) delete(
	ctx context.Context,
	log logr.Logger,
	bmcUserSet *metalv1alpha1.BMCUserSet,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(bmcUserSet, BMCUserSetFinalizer) {
		return ctrl.Result{}, nil
	}

	ownedBMCUsers, err := r.getOwnedBMCUsers(ctx, bmcUserSet)
	if err != nil {
		log.Error(err, "Failed to list owned BMCUsers")
		return ctrl.Result{}, fmt.Errorf("failed to get owned BMCUsers: %w", err)
	}

	var errs []error
	for i := range ownedBMCUsers.Items {
		if err := r.Delete(ctx, &ownedBMCUsers.Items[i]); err != nil {
			errs = append(errs, err)
		}
	}

	if len(ownedBMCUsers.Items) > 0 {
		currentStatus := r.getOwnedBMCUserSetStatus(ownedBMCUsers)
		if err := r.updateStatus(ctx, log, currentStatus, bmcUserSet); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update current BMCUserSet status: %w", err)
		}
		log.V(1).Info("Waiting on the created BMCUsers to be deleted")
		return ctrl.Result{}, errors.Join(errs...)
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, bmcUserSet, BMCUserSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("BMCUserSet is deleted")
	return ctrl.Result{}, errors.Join(errs...)
}

func (r *BMCUserSetReconciler) getOwnedBMCUsers(
	ctx context.Context,
	bmcUserSet *metalv1alpha1.BMCUserSet,
) (*metalv1alpha1.BMCUserList, error) {
	bmcUserList := &metalv1alpha1.BMCUserList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, bmcUserSet, bmcUserList); err != nil {
		return nil, err
	}
	return bmcUserList, nil
}

func (r *BMCUserSetReconciler) getOwnedBMCUserSetStatus(
	bmcUserList *metalv1alpha1.BMCUserList,
) *metalv1alpha1.BMCUserSetStatus {
	currentStatus := &metalv1alpha1.BMCUserSetStatus{}
	currentStatus.AvailableBMCUsers = int32(len(bmcUserList.Items))
	return currentStatus
}

func (r *BMCUserSetReconciler) updateStatus(
	ctx context.Context,
	log logr.Logger,
	currentStatus *metalv1alpha1.BMCUserSetStatus,
	bmcUserSet *metalv1alpha1.BMCUserSet,
) error {
	bmcUserSetBase := bmcUserSet.DeepCopy()
	bmcUserSet.Status = *currentStatus
	if err := r.Status().Patch(ctx, bmcUserSet, client.MergeFrom(bmcUserSetBase)); err != nil {
		return err
	}
	log.V(1).Info("Updated BMCUserSet status", "new status", currentStatus)
	return nil
}

func (r *BMCUserSetReconciler) getBMCsBySelector(
	ctx context.Context,
	bmcUserSet *metalv1alpha1.BMCUserSet,
) (*metalv1alpha1.BMCList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&bmcUserSet.Spec.BMCSelector)
	if err != nil {
		return nil, err
	}

	bmcList := &metalv1alpha1.BMCList{}
	if err := r.List(ctx, bmcList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}
	return bmcList, nil
}

func (r *BMCUserSetReconciler) createMissingBMCUsers(
	ctx context.Context,
	log logr.Logger,
	bmcList *metalv1alpha1.BMCList,
	bmcUserList *metalv1alpha1.BMCUserList,
	bmcUserSet *metalv1alpha1.BMCUserSet,
) error {
	bmcWithUser := make(map[string]struct{})
	for _, bmcUser := range bmcUserList.Items {
		if bmcUser.Spec.BMCRef == nil {
			continue
		}
		bmcWithUser[bmcUser.Spec.BMCRef.Name] = struct{}{}
	}

	var errs []error
	for _, bmc := range bmcList.Items {
		if _, ok := bmcWithUser[bmc.Name]; ok {
			continue
		}

		newBMCUserName := fmt.Sprintf("%s-%s", bmcUserSet.Name, bmc.Name)
		var newBMCUser *metalv1alpha1.BMCUser
		if len(newBMCUserName) > utilvalidation.DNS1123SubdomainMaxLength {
			log.V(1).Info("BMCUser name is too long, it will be shortened using random string",
				"name", newBMCUserName)
			newBMCUser = &metalv1alpha1.BMCUser{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: newBMCUserName[:utilvalidation.DNS1123SubdomainMaxLength-10] + "-",
				},
			}
		} else {
			newBMCUser = &metalv1alpha1.BMCUser{
				ObjectMeta: metav1.ObjectMeta{
					Name: newBMCUserName,
				},
			}
		}

		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, newBMCUser, func() error {
			newBMCUser.Spec.UserName = bmcUserSet.Spec.BMCUserTemplate.UserName
			newBMCUser.Spec.RoleID = bmcUserSet.Spec.BMCUserTemplate.RoleID
			newBMCUser.Spec.Description = bmcUserSet.Spec.BMCUserTemplate.Description
			newBMCUser.Spec.RotationPeriod = nil
			if bmcUserSet.Spec.BMCUserTemplate.RotationPeriod != nil {
				newBMCUser.Spec.RotationPeriod = bmcUserSet.Spec.BMCUserTemplate.RotationPeriod
			}
			newBMCUser.Spec.BMCSecretRef = nil
			if bmcUserSet.Spec.BMCUserTemplate.BMCSecretRef != nil {
				newBMCUser.Spec.BMCSecretRef = bmcUserSet.Spec.BMCUserTemplate.BMCSecretRef
			}
			newBMCUser.Spec.BMCRef = &corev1.LocalObjectReference{Name: bmc.Name}
			return controllerutil.SetControllerReference(bmcUserSet, newBMCUser, r.Scheme)
		})
		if err != nil {
			errs = append(errs, err)
		}
		log.V(1).Info("Created BMCUser", "BMCUser", newBMCUser.Name, "bmc ref", bmc.Name, "operation", opResult)
	}
	return errors.Join(errs...)
}

func (r *BMCUserSetReconciler) deleteOrphanedBMCUsers(
	ctx context.Context,
	log logr.Logger,
	bmcList *metalv1alpha1.BMCList,
	bmcUserList *metalv1alpha1.BMCUserList,
) error {
	bmcMap := make(map[string]struct{})
	for _, bmc := range bmcList.Items {
		bmcMap[bmc.Name] = struct{}{}
	}

	var errs []error
	for i := range bmcUserList.Items {
		bmcUser := &bmcUserList.Items[i]
		if bmcUser.Spec.BMCRef == nil {
			continue
		}
		if _, ok := bmcMap[bmcUser.Spec.BMCRef.Name]; !ok {
			if err := r.Delete(ctx, bmcUser); err != nil {
				errs = append(errs, err)
			}
			log.V(1).Info("Deleted orphaned BMCUser", "BMCUser", bmcUser.Name)
		}
	}

	return errors.Join(errs...)
}

func (r *BMCUserSetReconciler) patchBMCUsersFromTemplate(
	ctx context.Context,
	log logr.Logger,
	bmcUserSet *metalv1alpha1.BMCUserSet,
	bmcUserList *metalv1alpha1.BMCUserList,
) error {
	if len(bmcUserList.Items) == 0 {
		log.V(1).Info("No BMCUsers found, skipping spec template update")
		return nil
	}

	var errs []error
	for i := range bmcUserList.Items {
		bmcUser := &bmcUserList.Items[i]
		if !bmcUser.DeletionTimestamp.IsZero() {
			continue
		}
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, bmcUser, func() error {
			bmcUser.Spec.UserName = bmcUserSet.Spec.BMCUserTemplate.UserName
			bmcUser.Spec.RoleID = bmcUserSet.Spec.BMCUserTemplate.RoleID
			bmcUser.Spec.Description = bmcUserSet.Spec.BMCUserTemplate.Description
			bmcUser.Spec.RotationPeriod = nil
			if bmcUserSet.Spec.BMCUserTemplate.RotationPeriod != nil {
				bmcUser.Spec.RotationPeriod = &metav1.Duration{Duration: bmcUserSet.Spec.BMCUserTemplate.RotationPeriod.Duration}
			}
			if bmcUserSet.Spec.BMCUserTemplate.BMCSecretRef != nil {
				bmcUser.Spec.BMCSecretRef = &corev1.LocalObjectReference{Name: bmcUserSet.Spec.BMCUserTemplate.BMCSecretRef.Name}
			}
			return nil
		})
		if err != nil {
			errs = append(errs, err)
		}
		if opResult != controllerutil.OperationResultNone {
			log.V(1).Info("Patched BMCUser with updated spec", "BMCUser", bmcUser.Name, "operation", opResult)
		}
	}
	return errors.Join(errs...)
}

func (r *BMCUserSetReconciler) enqueueByBMC(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)

	bmc := obj.(*metalv1alpha1.BMC)
	bmcUserSetList := &metalv1alpha1.BMCUserSetList{}

	if err := r.List(ctx, bmcUserSetList); err != nil {
		log.Error(err, "Failed to list BMCUserSet")
		return nil
	}
	var reqs []ctrl.Request
	for _, bmcUserSet := range bmcUserSetList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&bmcUserSet.Spec.BMCSelector)
		if err != nil {
			log.Error(err, "Failed to parse BMCSelector", "BMCUserSet", bmcUserSet.Name)
			return nil
		}
		if selector.Matches(labels.Set(bmc.GetLabels())) {
			reqs = append(reqs, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name:      bmcUserSet.Name,
					Namespace: bmcUserSet.Namespace,
				},
			})
		} else {
			ownedBMCUsers, err := r.getOwnedBMCUsers(ctx, &bmcUserSet)
			if err != nil {
				log.Error(err, "Failed to list owned BMCUsers")
				return nil
			}
			for _, bmcUser := range ownedBMCUsers.Items {
				if bmcUser.Spec.BMCRef != nil && bmcUser.Spec.BMCRef.Name == bmc.Name {
					reqs = append(reqs, ctrl.Request{
						NamespacedName: client.ObjectKey{
							Name:      bmcUserSet.Name,
							Namespace: bmcUserSet.Namespace,
						},
					})
				}
			}
		}
	}
	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCUserSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMCUserSet{}).
		Owns(&metalv1alpha1.BMCUser{}).
		Watches(
			&metalv1alpha1.BMC{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueByBMC),
			builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Named("bmcuserset").
		Complete(r)
}
