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
	versionSet := &metalv1alpha1.BIOSVersionSet{}
	if err := r.Get(ctx, req.NamespacedName, versionSet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, versionSet)
}

func (r *BIOSVersionSetReconciler) reconcileExists(ctx context.Context, versionSet *metalv1alpha1.BIOSVersionSet) (ctrl.Result, error) {
	if !versionSet.DeletionTimestamp.IsZero() {
		return r.delete(ctx, versionSet)
	}
	return r.reconcile(ctx, versionSet)
}

func (r *BIOSVersionSetReconciler) delete(ctx context.Context, versionSet *metalv1alpha1.BIOSVersionSet) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Deleting BIOSVersionSet")
	if !controllerutil.ContainsFinalizer(versionSet, BIOSVersionSetFinalizer) {
		return ctrl.Result{}, nil
	}

	if err := r.handleIgnoreAnnotationPropagation(ctx, versionSet); err != nil {
		return ctrl.Result{}, err
	}

	versions, err := r.getOwnedBIOSVersions(ctx, versionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned BIOSVersions: %w", err)
	}

	status := r.getOwnedBIOSVersionSetStatus(versions)
	if status.AvailableBIOSVersion != (status.CompletedBIOSVersion+status.FailedBIOSVersion) ||
		versionSet.Status.AvailableBIOSVersion != status.AvailableBIOSVersion {
		if err = r.patchStatus(ctx, status, versionSet); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch BIOSVersionSet status: %w", err)
		}
		log.V(1).Info("BIOSVersionSet status patched", "Status", status)

		if err := r.handleRetryAnnotationPropagation(ctx, versionSet); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Waiting on the created BIOSVersion to reach terminal status")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, versionSet, BIOSVersionSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Deleted BIOSVersionSet")
	return ctrl.Result{}, nil
}

func (r *BIOSVersionSetReconciler) handleIgnoreAnnotationPropagation(ctx context.Context, versionSet *metalv1alpha1.BIOSVersionSet) error {
	log := ctrl.LoggerFrom(ctx)
	versions, err := r.getOwnedBIOSVersions(ctx, versionSet)
	if err != nil {
		return err
	}

	if len(versions.Items) == 0 {
		log.V(1).Info("No BIOSVersions found, skipping ignore annotation propagation")
		return nil
	}
	return handleIgnoreAnnotationPropagation(ctx, r.Client, versionSet, versions)
}

func (r *BIOSVersionSetReconciler) handleRetryAnnotationPropagation(ctx context.Context, versionSet *metalv1alpha1.BIOSVersionSet) error {
	log := ctrl.LoggerFrom(ctx)
	versions, err := r.getOwnedBIOSVersions(ctx, versionSet)
	if err != nil {
		return err
	}

	if len(versions.Items) == 0 {
		log.V(1).Info("No BIOSVersion found, skipping retry annotation propagation")
		return nil
	}
	return handleRetryAnnotationPropagation(ctx, r.Client, versionSet, versions)
}

func (r *BIOSVersionSetReconciler) reconcile(ctx context.Context, versionSet *metalv1alpha1.BIOSVersionSet) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling BIOSVersionSet")
	if err := r.handleIgnoreAnnotationPropagation(ctx, versionSet); err != nil {
		return ctrl.Result{}, err
	}

	if shouldIgnoreReconciliation(versionSet) {
		log.V(1).Info("Skipped BIOSVersionSet reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, versionSet, BIOSVersionSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	servers, err := r.getServersBySelector(ctx, versionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get servers by selector: %w", err)
	}

	versions, err := r.getOwnedBIOSVersions(ctx, versionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned BIOSVersions: %w", err)
	}

	log.V(1).Info("Summary of Servers and BIOSVersions", "ServerCount", len(servers.Items), "BIOSVersionCount", len(versions.Items))

	// Create BIOSVersion for servers which do not have one yet
	if err := r.ensureBIOSVersionsForServers(ctx, servers, versions, versionSet); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BIOSVersions: %w", err)
	}

	// Delete BIOSVersions which no longer have a matching server
	if err := r.deleteOrphanBIOSVersions(ctx, servers, versions); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete orphaned BIOSVersions: %w", err)
	}

	if err := r.patchBIOSVersionFromTemplate(ctx, &versionSet.Spec.BIOSVersionTemplate, versions); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch BIOSVersion spec from template: %w", err)
	}

	log.V(1).Info("Updating the status of BIOSVersionSet")
	status := r.getOwnedBIOSVersionSetStatus(versions)
	status.FullyLabeledServers = int32(len(servers.Items))

	if err := r.patchStatus(ctx, status, versionSet); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update BIOSVersionSet status: %w", err)
	}
	log.V(1).Info("Patched BIOSVersionSet status", "Status", status)

	if err := r.handleRetryAnnotationPropagation(ctx, versionSet); err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Reconciled BIOSVersionSet")
	return ctrl.Result{}, nil
}

func (r *BIOSVersionSetReconciler) ensureBIOSVersionsForServers(ctx context.Context, servers *metalv1alpha1.ServerList, versions *metalv1alpha1.BIOSVersionList, versionSet *metalv1alpha1.BIOSVersionSet) error {
	log := ctrl.LoggerFrom(ctx)
	withBiosVersion := make(map[string]bool)
	for _, version := range versions.Items {
		withBiosVersion[version.Spec.ServerRef.Name] = true
	}

	var errs []error
	for _, server := range servers.Items {
		if !withBiosVersion[server.Name] {
			var newBiosVersion *metalv1alpha1.BIOSVersion
			newBiosVersionName := fmt.Sprintf("%s-%s", versionSet.Name, server.Name)
			if len(newBiosVersionName) > utilvalidation.DNS1123SubdomainMaxLength {
				log.V(1).Info("BIOSVersion name is too long, it will be shortened using random string", "BIOSVersionName", newBiosVersionName)
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
				newBiosVersion.Spec.BIOSVersionTemplate = *versionSet.Spec.BIOSVersionTemplate.DeepCopy()
				newBiosVersion.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
				return controllerutil.SetControllerReference(versionSet, newBiosVersion, r.Client.Scheme())
			})
			if err != nil {
				errs = append(errs, err)
			}
			log.V(1).Info("Created BIOSVersion", "BIOSVersion", newBiosVersion.Name, "Server", server.Name, "Operation", opResult)
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSVersionSetReconciler) deleteOrphanBIOSVersions(ctx context.Context, servers *metalv1alpha1.ServerList, versions *metalv1alpha1.BIOSVersionList) error {
	log := ctrl.LoggerFrom(ctx)
	serverMap := make(map[string]bool)
	for _, server := range servers.Items {
		serverMap[server.Name] = true
	}

	var errs []error
	for _, biosVersion := range versions.Items {
		if !serverMap[biosVersion.Spec.ServerRef.Name] {
			if biosVersion.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
				log.V(1).Info("Waiting for BIOSVersion to move out of InProgress state", "BIOSVersion", biosVersion.Name, "Status", biosVersion.Status)
				continue
			}
			if err := r.Delete(ctx, &biosVersion); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSVersionSetReconciler) patchBIOSVersionFromTemplate(ctx context.Context, template *metalv1alpha1.BIOSVersionTemplate, versions *metalv1alpha1.BIOSVersionList) error {
	log := ctrl.LoggerFrom(ctx)
	if len(versions.Items) == 0 {
		log.V(1).Info("No BIOSVersion found, skipping spec template update")
		return nil
	}

	var errs []error
	for _, version := range versions.Items {
		if version.Status.State == metalv1alpha1.BIOSVersionStateInProgress {
			continue
		}
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &version, func() error {
			version.Spec.BIOSVersionTemplate = *template.DeepCopy()
			return nil
		})
		if err != nil {
			errs = append(errs, err)
		}
		if opResult != controllerutil.OperationResultNone {
			log.V(1).Info("Patched BIOSVersion with updated spec", "BIOSVersion", version.Name, "Operation", opResult)
			opResult, err = controllerutil.CreateOrPatch(ctx, r.Client, &version, func() error {
				version.Status.AutoRetryCountRemaining = version.Spec.BIOSVersionTemplate.FailedAutoRetryCount
				return nil
			})
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSVersionSetReconciler) getOwnedBIOSVersionSetStatus(versionList *metalv1alpha1.BIOSVersionList) *metalv1alpha1.BIOSVersionSetStatus {
	status := &metalv1alpha1.BIOSVersionSetStatus{}
	status.AvailableBIOSVersion = int32(len(versionList.Items))
	for _, biosVersion := range versionList.Items {
		switch biosVersion.Status.State {
		case metalv1alpha1.BIOSVersionStateCompleted:
			status.CompletedBIOSVersion += 1
		case metalv1alpha1.BIOSVersionStateFailed:
			status.FailedBIOSVersion += 1
		case metalv1alpha1.BIOSVersionStateInProgress:
			status.InProgressBIOSVersion += 1
		case metalv1alpha1.BIOSVersionStatePending, "":
			status.PendingBIOSVersion += 1
		}
	}
	return status
}

func (r *BIOSVersionSetReconciler) getOwnedBIOSVersions(ctx context.Context, versionSet *metalv1alpha1.BIOSVersionSet) (*metalv1alpha1.BIOSVersionList, error) {
	biosVersionList := &metalv1alpha1.BIOSVersionList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, versionSet, biosVersionList); err != nil {
		return nil, err
	}
	return biosVersionList, nil
}

func (r *BIOSVersionSetReconciler) getServersBySelector(ctx context.Context, set *metalv1alpha1.BIOSVersionSet) (*metalv1alpha1.ServerList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&set.Spec.ServerSelector)
	if err != nil {
		return nil, err
	}
	servers := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, servers, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}
	return servers, nil
}

func (r *BIOSVersionSetReconciler) patchStatus(ctx context.Context, status *metalv1alpha1.BIOSVersionSetStatus, versionSet *metalv1alpha1.BIOSVersionSet) error {
	versionSetBase := versionSet.DeepCopy()
	versionSet.Status = *status

	if err := r.Status().Patch(ctx, versionSet, client.MergeFrom(versionSetBase)); err != nil {
		return err
	}
	return nil
}

func (r *BIOSVersionSetReconciler) enqueueByServer(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	server := obj.(*metalv1alpha1.Server)

	setList := &metalv1alpha1.BIOSVersionSetList{}
	if err := r.List(ctx, setList); err != nil {
		log.Error(err, "failed to list BIOSVersionSet")
		return nil
	}
	reqs := make([]ctrl.Request, 0)
	for _, set := range setList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&set.Spec.ServerSelector)
		if err != nil {
			log.Error(err, "failed to convert label selector")
			return nil
		}
		// If the Server label matches the selector, enqueue the request
		if selector.Matches(labels.Set(server.GetLabels())) {
			reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKey{Name: set.Name}})
		} else { // if the label has been removed
			versions, err := r.getOwnedBIOSVersions(ctx, &set)
			if err != nil {
				log.Error(err, "failed to get owned BIOSVersions")
				return nil
			}
			for _, version := range versions.Items {
				if version.Spec.ServerRef.Name == server.Name {
					reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKey{Name: set.Name}})
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
