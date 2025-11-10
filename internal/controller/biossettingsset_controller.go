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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// BIOSSettingsSetReconciler reconciles a BIOSSettingsSet object
type BIOSSettingsSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const biosSettingsSetFinalizer = "metal.ironcore.dev/biossettingsset"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch

func (r *BIOSSettingsSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	biosSettingsSet := &metalv1alpha1.BIOSSettingsSet{}
	if err := r.Get(ctx, req.NamespacedName, biosSettingsSet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling biosSettingsSet")

	return r.reconcileExists(ctx, log, biosSettingsSet)
}

func (r *BIOSSettingsSetReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if !biosSettingsSet.DeletionTimestamp.IsZero() {
		log.V(1).Info("Object is being deleted")
		return r.delete(ctx, log, biosSettingsSet)
	}

	return r.reconcile(ctx, log, biosSettingsSet)
}

func (r *BIOSSettingsSetReconciler) delete(
	ctx context.Context,
	log logr.Logger,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(biosSettingsSet, biosSettingsSetFinalizer) {
		return ctrl.Result{}, nil
	}

	// handle propogartion of ignore annotation to child when parent is being deleted
	// so that the deleted annotations can be passed to children before parent is deleted
	if err := r.handleIgnoreAnnotationPropagation(ctx, log, biosSettingsSet); err != nil {
		return ctrl.Result{}, err
	}
	ownedBiosSettings, err := r.getOwnedBIOSSettings(ctx, biosSettingsSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned BIOSSettings resources %w", err)
	}

	delatableBIOSSettings := map[string]struct{}{}
	for _, biosSettings := range ownedBiosSettings.Items {
		if biosSettings.Status.State != metalv1alpha1.BIOSSettingsStateInProgress {
			delatableBIOSSettings[biosSettings.Name] = struct{}{}
		} else if biosSettings.Spec.ServerMaintenanceRef == nil {
			delatableBIOSSettings[biosSettings.Name] = struct{}{}
		}
	}

	if len(ownedBiosSettings.Items) != len(delatableBIOSSettings) ||
		int32(len(ownedBiosSettings.Items)) != biosSettingsSet.Status.AvailableBIOSSettings {
		currentStatus := r.getOwnedBIOSSettingsSetStatus(ownedBiosSettings)
		err = r.updateStatus(ctx, log, currentStatus, biosSettingsSet)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update current BIOSSettingsSet Status %w", err)
		}
		log.Info("Waiting on the created BIOSSettings to reach terminal status")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosSettingsSet, biosSettingsSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("BIOSSettingsSet is deleted")
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsSetReconciler) handleIgnoreAnnotationPropagation(
	ctx context.Context,
	log logr.Logger,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) error {
	ownedBiosSettings, err := r.getOwnedBIOSSettings(ctx, biosSettingsSet)
	if err != nil {
		return err
	}
	if len(ownedBiosSettings.Items) == 0 {
		log.V(1).Info("No BIOSSettings found, skipping ignore annotation propagation")
		return nil
	}
	if !shouldChildIgnoreReconciliation(biosSettingsSet) {
		// if the Set object does not have the ignore annotation anymore,
		// we should remove the ignore annotation on the child
		var errs []error
		for _, biosSettings := range ownedBiosSettings.Items {
			// if the Child object is ignored through sets, we should remove the ignore annotation on the child
			// ad the Set object does not have the ignore annotation anymore
			opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &biosSettings, func() error {
				if isChildIgnoredThroughSets(&biosSettings) {
					annotations := biosSettings.GetAnnotations()
					log.V(1).Info("Ignore operation deleted on child object", "BIOSSettings", biosSettings.Name)
					delete(annotations, metalv1alpha1.OperationAnnotation)
					delete(annotations, metalv1alpha1.PropagatedOperationAnnotation)
					biosSettings.SetAnnotations(annotations)
				}
				return nil
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to patch BIOSSettings annotations Propogartion removal: %w for: %v", err, biosSettings.Name))
			}
			if opResult != controllerutil.OperationResultNone {
				log.V(1).Info("Patched BIOSSettings's annotations to remove Ignore", "BIOSSettings", biosSettings.Name, "Operation", opResult)
			}
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		return nil
	}
	var errs []error
	for _, biosSettings := range ownedBiosSettings.Items {
		// should not overwrite the already ignored annotation on child
		// should not overwrite if the annotation already present on the child
		if !isChildIgnoredThroughSets(&biosSettings) && !shouldIgnoreReconciliation(&biosSettings) {
			biosSettingsBase := biosSettings.DeepCopy()
			annotations := biosSettings.GetAnnotations()
			annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.OperationAnnotationIgnore
			annotations[metalv1alpha1.PropagatedOperationAnnotation] = metalv1alpha1.OperationAnnotationIgnoreChild
			if err := r.Patch(ctx, &biosSettings, client.MergeFrom(biosSettingsBase)); err != nil {
				errs = append(errs, fmt.Errorf("failed to patch BIOSSettings annotations: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSSettingsSetReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) (ctrl.Result, error) {
	if err := r.handleIgnoreAnnotationPropagation(ctx, log, biosSettingsSet); err != nil {
		return ctrl.Result{}, err
	}

	if shouldIgnoreReconciliation(biosSettingsSet) {
		log.V(1).Info("Skipped BIOSSettingsSet reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, biosSettingsSet, biosSettingsSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	serverList, err := r.getServersBySelector(ctx, biosSettingsSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get Servers through label selector %w", err)
	}
	return r.handleBiosSettings(ctx, log, serverList, biosSettingsSet)
}

func (r *BIOSSettingsSetReconciler) handleBiosSettings(
	ctx context.Context,
	log logr.Logger,
	serverList *metalv1alpha1.ServerList,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) (ctrl.Result, error) {
	ownedBiosSettings, err := r.getOwnedBIOSSettings(ctx, biosSettingsSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.createMissingBIOSSettings(ctx, log, serverList, ownedBiosSettings, biosSettingsSet); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create missing BIOSSettings resources %w", err)
	}

	log.V(1).Info("Summary of servers and BIOSSettings", "Server count", len(serverList.Items),
		"BIOSVersion count", len(ownedBiosSettings.Items))

	if err := r.deleteOrphanBIOSSettings(ctx, log, serverList, ownedBiosSettings); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete orphaned BIOSSettings resources %w", err)
	}

	if err := r.patchBIOSSettingsfromTemplate(ctx, log, &biosSettingsSet.Spec.BIOSSettingsTemplate, ownedBiosSettings); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch BIOSSettings spec from template %w", err)
	}

	log.V(1).Info("Updating the status of BIOSSettingsSet")
	currentStatus := r.getOwnedBIOSSettingsSetStatus(ownedBiosSettings)
	currentStatus.FullyLabeledServers = int32(len(serverList.Items))

	err = r.updateStatus(ctx, log, currentStatus, biosSettingsSet)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update current BIOSSettingsSet Status %w", err)
	}
	// wait for any updates from owned resources
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsSetReconciler) createMissingBIOSSettings(
	ctx context.Context,
	log logr.Logger,
	serverList *metalv1alpha1.ServerList,
	biosSettingsList *metalv1alpha1.BIOSSettingsList,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) error {

	serverWithSettings := make(map[string]struct{})
	for _, biosSettings := range biosSettingsList.Items {
		serverWithSettings[biosSettings.Spec.ServerRef.Name] = struct{}{}
	}

	var errs []error
	for _, server := range serverList.Items {
		if _, ok := serverWithSettings[server.Name]; !ok {
			if server.Spec.BIOSSettingsRef != nil {
				// this is the case where the server already has a different BIOSSettingsRef, and we should not create a new one for this server
				log.V(1).Info("Server already has different BIOSSettingsRef, skipping creation", "server", server.Name, "BIOSSettingsRef", server.Spec.BIOSSettingsRef)
				continue
			}
			newBiosSettingsName := fmt.Sprintf("%s-%s", biosSettingsSet.Name, server.Name)
			var newBiosSetting *metalv1alpha1.BIOSSettings
			if len(newBiosSettingsName) > utilvalidation.DNS1123SubdomainMaxLength {
				log.V(1).Info("BiosSettings name is too long, it will be shortened using random string", "name", newBiosSettingsName)
				newBiosSetting = &metalv1alpha1.BIOSSettings{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: newBiosSettingsName[:utilvalidation.DNS1123SubdomainMaxLength-10] + "-",
					}}
			} else {
				newBiosSetting = &metalv1alpha1.BIOSSettings{
					ObjectMeta: metav1.ObjectMeta{
						Name: newBiosSettingsName,
					}}
			}

			opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, newBiosSetting, func() error {
				newBiosSetting.Spec.BIOSSettingsTemplate = *biosSettingsSet.Spec.BIOSSettingsTemplate.DeepCopy()
				newBiosSetting.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
				return controllerutil.SetControllerReference(biosSettingsSet, newBiosSetting, r.Client.Scheme())
			})
			if err != nil {
				errs = append(errs, err)
			}
			log.V(1).Info("Created biosSettings", "BIOSSettings", newBiosSetting.Name, "server ref", server.Name, "Operation", opResult)
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSSettingsSetReconciler) deleteOrphanBIOSSettings(
	ctx context.Context,
	log logr.Logger,
	serverList *metalv1alpha1.ServerList,
	biosSettingsList *metalv1alpha1.BIOSSettingsList,
) error {
	serverMap := make(map[string]bool)
	for _, server := range serverList.Items {
		serverMap[server.Name] = true
	}

	var errs []error
	for _, biosSettings := range biosSettingsList.Items {
		if _, ok := serverMap[biosSettings.Spec.ServerRef.Name]; !ok {
			if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
				log.V(1).Info("Waiting for BIOSSettings to move out of InProgress state", "BIOSSettings", biosSettings.Name, "status", biosSettings.Status)
				continue
			}
			if err := r.Delete(ctx, &biosSettings); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

func (r *BIOSSettingsSetReconciler) patchBIOSSettingsfromTemplate(
	ctx context.Context,
	log logr.Logger,
	biosSettingsTemplate *metalv1alpha1.BIOSSettingsTemplate,
	biosSettingsList *metalv1alpha1.BIOSSettingsList,
) error {
	if len(biosSettingsList.Items) == 0 {
		log.V(1).Info("No BIOSSettings found, skipping spec template update")
		return nil
	}

	var errs []error
	for _, biosSettings := range biosSettingsList.Items {
		if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
			continue
		}
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &biosSettings, func() error {
			// serverMaintenanceRef might not be part of the patching template, so we do not patch if not provided
			if biosSettingsTemplate.ServerMaintenanceRef != nil {
				biosSettings.Spec.BIOSSettingsTemplate = *biosSettingsTemplate.DeepCopy()
			} else {
				serverMaintenanceRef := biosSettings.Spec.ServerMaintenanceRef
				biosSettings.Spec.BIOSSettingsTemplate = *biosSettingsTemplate.DeepCopy()
				biosSettings.Spec.ServerMaintenanceRef = serverMaintenanceRef
			}
			return nil
		}) //nolint:errcheck
		if err != nil {
			errs = append(errs, err)
		}
		if opResult != controllerutil.OperationResultNone {
			log.V(1).Info("Patched biosSettings with updated spec", "BIOSSettings", biosSettings.Name, "Operation", opResult)
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSSettingsSetReconciler) getOwnedBIOSSettingsSetStatus(
	biosSettingsList *metalv1alpha1.BIOSSettingsList,
) *metalv1alpha1.BIOSSettingsSetStatus {
	currentStatus := &metalv1alpha1.BIOSSettingsSetStatus{}
	currentStatus.AvailableBIOSSettings = int32(len(biosSettingsList.Items))
	for _, biosSettings := range biosSettingsList.Items {
		switch biosSettings.Status.State {
		case metalv1alpha1.BIOSSettingsStateApplied:
			currentStatus.CompletedBIOSSettings += 1
		case metalv1alpha1.BIOSSettingsStateFailed:
			currentStatus.FailedBIOSSettings += 1
		case metalv1alpha1.BIOSSettingsStateInProgress:
			currentStatus.InProgressBIOSSettings += 1
		case metalv1alpha1.BIOSSettingsStatePending, "":
			currentStatus.PendingBIOSSettings += 1
		}
	}
	return currentStatus
}

func (r *BIOSSettingsSetReconciler) getOwnedBIOSSettings(
	ctx context.Context,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) (*metalv1alpha1.BIOSSettingsList, error) {
	biosSettingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, biosSettingsSet, biosSettingsList); err != nil {
		return nil, err
	}
	return biosSettingsList, nil
}

func (r *BIOSSettingsSetReconciler) getServersBySelector(
	ctx context.Context,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) (*metalv1alpha1.ServerList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&biosSettingsSet.Spec.ServerSelector)
	if err != nil {
		return nil, err
	}
	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}

	return serverList, nil
}

func (r *BIOSSettingsSetReconciler) updateStatus(
	ctx context.Context,
	log logr.Logger,
	currentStatus *metalv1alpha1.BIOSSettingsSetStatus,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) error {

	biosSettingsSetBase := biosSettingsSet.DeepCopy()

	biosSettingsSet.Status = *currentStatus

	if err := r.Status().Patch(ctx, biosSettingsSet, client.MergeFrom(biosSettingsSetBase)); err != nil {
		return err
	}

	log.V(1).Info("Updated biosSettingsSet state ", "new state", currentStatus)

	return nil
}

func (r *BIOSSettingsSetReconciler) enqueueByServer(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	host := obj.(*metalv1alpha1.Server)

	biosSettingsSetList := &metalv1alpha1.BIOSSettingsSetList{}
	if err := r.List(ctx, biosSettingsSetList); err != nil {
		log.V(1).Error(err, "failed to list BIOSVersionSet")
		return nil
	}
	reqs := make([]ctrl.Request, 0)
	for _, biosSettingsSet := range biosSettingsSetList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&biosSettingsSet.Spec.ServerSelector)
		if err != nil {
			log.V(1).Error(err, "failed to convert label selector")
			return nil
		}
		// if the host label matches the selector, enqueue the request
		if selector.Matches(labels.Set(host.GetLabels())) {
			reqs = append(reqs, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name:      biosSettingsSet.Name,
					Namespace: biosSettingsSet.Namespace,
				},
			})
		} else { // if the label has been removed
			ownedBiosVersions, err := r.getOwnedBIOSSettings(ctx, &biosSettingsSet)
			if err != nil {
				log.V(1).Error(err, "failed to get owned BIOSVersion resources")
				return nil
			}
			for _, biosVersion := range ownedBiosVersions.Items {
				if biosVersion.Spec.ServerRef.Name == host.Name {
					reqs = append(reqs, ctrl.Request{
						NamespacedName: client.ObjectKey{
							Name:      biosSettingsSet.Name,
							Namespace: biosSettingsSet.Namespace,
						},
					})
				}
			}
		}
	}
	return reqs
}

// SetupWithManager sets up the controller with the Manager.
func (r *BIOSSettingsSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSSettingsSet{}).
		Watches(
			&metalv1alpha1.BIOSSettings{},
			handler.EnqueueRequestForOwner(r.Scheme, r.RESTMapper(), &metalv1alpha1.BIOSSettingsSet{}),
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
		Watches(
			&metalv1alpha1.Server{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueByServer),
			builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Complete(r)
}
