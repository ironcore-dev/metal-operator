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
	"sigs.k8s.io/controller-runtime/pkg/event"
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

	if err := r.handleIgnoreAnnotationPropagation(ctx, log, bmcVersionSet); err != nil {
		return ctrl.Result{}, err
	}

	ownedBMCVersions, err := r.getOwnedBMCVersions(ctx, bmcVersionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned BMCVersion resources %w", err)
	}

	currentStatus := r.getOwnedBMCVersionSetStatus(ownedBMCVersions)

	if currentStatus.AvailableBMCVersion != (currentStatus.CompletedBMCVersion+currentStatus.FailedBMCVersion) ||
		bmcVersionSet.Status.AvailableBMCVersion != currentStatus.AvailableBMCVersion {
		err = r.updateStatus(ctx, log, currentStatus, bmcVersionSet)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update current BMCVersionSet Status %w", err)
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

func (r *BMCVersionSetReconciler) handleIgnoreAnnotationPropagation(
	ctx context.Context,
	log logr.Logger,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) error {
	ownedBMCVersions, err := r.getOwnedBMCVersions(ctx, bmcVersionSet)
	if err != nil {
		return err
	}
	if len(ownedBMCVersions.Items) == 0 {
		log.V(1).Info("No BMCVersion found, skipping ignore annotation propagation")
		return nil
	}
	if !shouldChildIgnoreReconciliation(bmcVersionSet) {
		// if the Set object does not have the ignore annotation anymore,
		// we should remove the ignore annotation on the child
		var errs []error
		for _, bmcVersion := range ownedBMCVersions.Items {
			// if the Child object is ignored through sets, we should remove the ignore annotation on the child
			// ad the Set object does not have the ignore annotation anymore
			opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &bmcVersion, func() error {
				if isChildIgnoredThroughSets(&bmcVersion) {
					annotations := bmcVersion.GetAnnotations()
					log.V(1).Info("Ignore operation deleted on child object", "BMCVersion", bmcVersion.Name)
					delete(annotations, metalv1alpha1.OperationAnnotation)
					delete(annotations, metalv1alpha1.PropagatedOperationAnnotation)
					bmcVersion.SetAnnotations(annotations)
				}
				return nil
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to patch BMCVersion annotations Propogartion removal: %w for: %v", err, bmcVersion.Name))
			}
			if opResult != controllerutil.OperationResultNone {
				log.V(1).Info("Patched BMCVersion's annotations to remove Ignore", "BMCVersion", bmcVersion.Name, "Operation", opResult)
			}
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		return nil
	}

	var errs []error
	for _, bmcVersion := range ownedBMCVersions.Items {
		// should not overwrite the ignored annotation written by others on child
		// should not overwrite if the annotation is already written on the child
		if !isChildIgnoredThroughSets(&bmcVersion) && !shouldIgnoreReconciliation(&bmcVersion) {
			bmcVersionBase := bmcVersion.DeepCopy()
			annotations := bmcVersion.GetAnnotations()
			annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.OperationAnnotationIgnore
			annotations[metalv1alpha1.PropagatedOperationAnnotation] = metalv1alpha1.OperationAnnotationIgnoreChild
			if err := r.Patch(ctx, &bmcVersion, client.MergeFrom(bmcVersionBase)); err != nil {
				errs = append(errs, fmt.Errorf("failed to patch BMCVersion annotations: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}

func (r *BMCVersionSetReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) (ctrl.Result, error) {
	if err := r.handleIgnoreAnnotationPropagation(ctx, log, bmcVersionSet); err != nil {
		return ctrl.Result{}, err
	}

	if shouldIgnoreReconciliation(bmcVersionSet) {
		log.V(1).Info("Skipped BMCVersionSet reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, bmcVersionSet, BMCVersionSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	bmcList, err := r.getBMCBySelector(ctx, bmcVersionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC resource through label selector %w", err)
	}

	ownedBMCVersions, err := r.getOwnedBMCVersions(ctx, bmcVersionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned BMCVersion resources %w", err)
	}

	log.V(1).Info("Summary of BMC and BMCVersions", "BMCs count", len(bmcList.Items),
		"BMCVersion count", len(ownedBMCVersions.Items))

	// create BMCVersion for BMCs selected, if it does not exist
	if err := r.createMissingBMCVersions(ctx, log, bmcList, ownedBMCVersions, bmcVersionSet); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMCVersion resources %w", err)
	}

	// delete BMCVersion for BMCs which do not exist anymore
	if _, err := r.deleteOrphanBMCVersions(ctx, log, bmcList, ownedBMCVersions); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete orphaned BMCVersion resources %w", err)
	}

	if err := r.patchBMCVersionfromTemplate(ctx, log, bmcList, bmcVersionSet, ownedBMCVersions); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch BMCVersion spec from template %w", err)
	}

	log.V(1).Info("updating the status of BMCVersionSet")
	currentStatus := r.getOwnedBMCVersionSetStatus(ownedBMCVersions)
	currentStatus.FullyLabeledBMCs = int32(len(bmcList.Items))

	err = r.updateStatus(ctx, log, currentStatus, bmcVersionSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update current BMCVersionSet Status %w", err)
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
			serverMaintenanceRefsProvided, err := r.getProvidedServerMaintenanceRefs(ctx, &bmc, bmcVersionSet)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get provided ServerMaintenanceRefs for BMC %s: %w", bmc.Name, err))
				continue
			}
			opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, newBMCVersion, func() error {
				newBMCVersion.Spec.BMCVersionTemplate = *bmcVersionSet.Spec.BMCVersionTemplate.DeepCopy()
				// patch ServerMaintenance referenced by BMC's Servers
				newBMCVersion.Spec.ServerMaintenanceRefs = serverMaintenanceRefsProvided

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

func (r *BMCVersionSetReconciler) getProvidedServerMaintenanceRefs(
	ctx context.Context,
	bmc *metalv1alpha1.BMC,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
) ([]metalv1alpha1.ServerMaintenanceRefItem, error) {
	serverMaintenanceRefMap := make(map[string]metalv1alpha1.ServerMaintenanceRefItem)
	for _, ref := range bmcVersionSet.Spec.BMCVersionTemplate.ServerMaintenanceRefs {
		if ref.ServerMaintenanceRef != nil {
			serverMaintenanceRefMap[ref.ServerMaintenanceRef.Name] = ref
		}
	}
	// get servers for this BMC
	serverList, err := r.getBMCOwnedServers(ctx, bmc)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers owned by BMC %s: %w", bmc.Name, err)
	}
	// get map of servers owned by this BMC
	serverMap := make(map[string]struct{})
	for _, server := range serverList.Items {
		serverMap[server.Name] = struct{}{}
	}
	// filter the serverMaintenances which are for servers owned by this BMC
	serverMaintenancesList := &metalv1alpha1.ServerMaintenanceList{}
	err = clientutils.ListAndFilter(ctx, r.Client, serverMaintenancesList, func(object client.Object) (bool, error) {
		serverMaintenance := object.(*metalv1alpha1.ServerMaintenance)
		if serverMaintenance.Spec.ServerRef == nil {
			return false, nil
		}
		if _, exists := serverMap[serverMaintenance.Spec.ServerRef.Name]; !exists {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list ServerMaintenance for BMC's %s: Servers. Error %w", bmc.Name, err)
	}

	serverMaintenanceRefs := []metalv1alpha1.ServerMaintenanceRefItem{}
	for _, serverMaintenance := range serverMaintenancesList.Items {
		if ref, exists := serverMaintenanceRefMap[serverMaintenance.Name]; exists {
			serverMaintenanceRefs = append(serverMaintenanceRefs, ref)
		}
	}
	return serverMaintenanceRefs, nil
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

func (r *BMCVersionSetReconciler) patchBMCVersionfromTemplate(
	ctx context.Context,
	log logr.Logger,
	bmcList *metalv1alpha1.BMCList,
	bmcVersionSet *metalv1alpha1.BMCVersionSet,
	bmcVersionList *metalv1alpha1.BMCVersionList,
) error {
	if len(bmcVersionList.Items) == 0 {
		log.V(1).Info("No BMCVersion found, skipping spec template update")
		return nil
	}
	bmcNameMap := make(map[string]metalv1alpha1.BMC)
	for _, bmc := range bmcList.Items {
		bmcNameMap[bmc.Name] = bmc
	}

	var errs []error
	for _, bmcVersion := range bmcVersionList.Items {
		if bmcVersion.Status.State == metalv1alpha1.BMCVersionStateInProgress && bmcVersion.Status.UpgradeTask != nil {
			log.V(1).Info("Skipping BMCVersion spec patching as it is in InProgress state with an active UpgradeTask")
			continue
		}
		bmc, exists := bmcNameMap[bmcVersion.Spec.BMCRef.Name]
		if !exists {
			errs = append(errs, fmt.Errorf("BMC %s not found for BMCVersion %s", bmcVersion.Spec.BMCRef.Name, bmcVersion.Name))
			continue
		}
		serverMaintenanceRefsProvided, err := r.getProvidedServerMaintenanceRefs(ctx, &bmc, bmcVersionSet)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get ServerMaintenanceRefs for BMC %s: %w", bmc.Name, err))
			continue
		}

		serverMaintenancesRefsCreated, err := r.getCreatedServerMaintenanceRefs(ctx, &bmcVersion)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get owned ServerMaintenance for BMCVersion %s: %w", bmcVersion.Name, err))
			continue
		}

		var serverMaintenanceRefsMerged []metalv1alpha1.ServerMaintenanceRefItem
		if len(serverMaintenancesRefsCreated) > 0 && len(serverMaintenanceRefsProvided) > 0 {
			// merge provided and created serverMaintenanceRefs
			serverMaintenanceRefMap := make(map[string]metalv1alpha1.ServerMaintenanceRefItem)
			for _, ref := range serverMaintenanceRefsProvided {
				if ref.ServerMaintenanceRef != nil {
					serverMaintenanceRefMap[ref.ServerMaintenanceRef.Name] = ref
				}
			}
			for _, ref := range serverMaintenancesRefsCreated {
				if ref.ServerMaintenanceRef != nil {
					serverMaintenanceRefMap[ref.ServerMaintenanceRef.Name] = ref
				}
			}
			for _, ref := range serverMaintenanceRefMap {
				serverMaintenanceRefsMerged = append(serverMaintenanceRefsMerged, ref)
			}

			// check if the length is as expected
			server, err := r.getBMCOwnedServers(ctx, &bmc)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get servers owned by BMC %s to verify serverMaintenanceRef: %w", bmc.Name, err))
				continue
			}
			if len(serverMaintenanceRefsMerged) > len(server.Items) {
				errs = append(errs, fmt.Errorf("number of ServerMaintenanceRefs %d exceeds number of Servers %d for BMC %s",
					len(serverMaintenanceRefsMerged), len(server.Items), bmc.Name))
				continue
			}
		} else if len(serverMaintenanceRefsProvided) > 0 {
			serverMaintenanceRefsMerged = serverMaintenanceRefsProvided
		} else {
			serverMaintenanceRefsMerged = serverMaintenancesRefsCreated
		}

		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &bmcVersion, func() error {
			bmcVersion.Spec.BMCVersionTemplate = *bmcVersionSet.Spec.BMCVersionTemplate.DeepCopy()
			bmcVersion.Spec.ServerMaintenanceRefs = serverMaintenanceRefsMerged
			return nil
		}) //nolint:errcheck
		if err != nil {
			errs = append(errs, err)
		}
		if opResult != controllerutil.OperationResultNone {
			log.V(1).Info("Patched BMCVersion with updated spec", "BMCVersions", bmcVersion.Name, "Operation", opResult)
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

func (r *BMCVersionSetReconciler) getBMCOwnedServers(
	ctx context.Context,
	bmc *metalv1alpha1.BMC,
) (*metalv1alpha1.ServerList, error) {
	serverList := &metalv1alpha1.ServerList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, bmc, serverList); err != nil {
		return nil, err
	}
	return serverList, nil
}

func (r *BMCVersionSetReconciler) getCreatedServerMaintenanceRefs(
	ctx context.Context,
	bmcVersion *metalv1alpha1.BMCVersion,
) ([]metalv1alpha1.ServerMaintenanceRefItem, error) {
	serverMaintenanceRefMap := make(map[string]metalv1alpha1.ServerMaintenanceRefItem)
	for _, ref := range bmcVersion.Spec.ServerMaintenanceRefs {
		if ref.ServerMaintenanceRef != nil {
			serverMaintenanceRefMap[ref.ServerMaintenanceRef.Name] = ref
		}
	}

	serverMaintenanceList := &metalv1alpha1.ServerMaintenanceList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, bmcVersion, serverMaintenanceList); err != nil {
		return nil, err
	}

	bmcVersion.Spec.ServerMaintenanceRefs = []metalv1alpha1.ServerMaintenanceRefItem{}

	serverMaintenanceRefs := []metalv1alpha1.ServerMaintenanceRefItem{}
	for _, serverMaintenance := range serverMaintenanceList.Items {
		if ref, exists := serverMaintenanceRefMap[serverMaintenance.Name]; exists {
			serverMaintenanceRefs = append(serverMaintenanceRefs, ref)
		}
	}
	return serverMaintenanceRefs, nil
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
		return err
	}

	log.V(1).Info("Updated BMCVersionSet state ", "new state", currentStatus)

	return nil
}

func (r *BMCVersionSetReconciler) enqueueByBMC(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)

	host := obj.(*metalv1alpha1.BMC)

	bmcVersionSetList := &metalv1alpha1.BMCVersionSetList{}
	if err := r.List(ctx, bmcVersionSetList); err != nil {
		log.V(1).Error(err, "failed to list BMCVersionSet")
		return nil
	}
	reqs := make([]ctrl.Request, 0)
	for _, bmcVersionSet := range bmcVersionSetList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&bmcVersionSet.Spec.BMCSelector)
		if err != nil {
			log.V(1).Error(err, "failed to convert label selector")
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
				log.V(1).Error(err, "failed to get owned BMCVersion resources")
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
		Watches(
			&metalv1alpha1.BMCVersion{},
			handler.EnqueueRequestForOwner(r.Scheme, r.RESTMapper(), &metalv1alpha1.BMCVersionSet{}),
			builder.WithPredicates(
				predicate.Funcs{
					CreateFunc: func(e event.CreateEvent) bool {
						return true
					},
					UpdateFunc: func(e event.UpdateEvent) bool {
						return enqueFromChildObjUpdatesExceptAnnotation(e)
					},
					DeleteFunc: func(e event.DeleteEvent) bool {
						return true
					}, GenericFunc: func(e event.GenericEvent) bool {
						return false
					},
				},
			),
		).
		Watches(&metalv1alpha1.BMC{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueByBMC),
			builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Complete(r)
}
