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

// BIOSSettingsSetReconciler reconciles a BIOSSettingsSet object
type BIOSSettingsSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const biosSettingsSetFinalizer = "metal.ironcore.dev/biosSettingsSet"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings,verbs=get;list;watch;create;update;patch;delete

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
		log.V(1).Info("object is being deleted")
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

	ownedBiosSettings, err := r.getOwnedBIOSSettings(ctx, biosSettingsSet)
	if err != nil {
		return ctrl.Result{}, err
	}
	currentStatus := r.getOwnedBIOSSettingsSetStatus(ownedBiosSettings)

	if currentStatus.AvailableBIOSSettings != (currentStatus.CompletedBIOSSettings+currentStatus.FailedBIOSSettings) ||
		currentStatus.AvailableBIOSSettings != biosSettingsSet.Status.AvailableBIOSSettings {
		err = r.updateStatus(ctx, log, currentStatus, biosSettingsSet)
		if err != nil {
			log.Error(err, "failed to update current Status")
			return ctrl.Result{}, err
		}
		log.Info("Waiting on the created BIOSSettings to reach terminal status")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosSettingsSet, biosSettingsSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("biosSettingsSet is deleted")
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsSetReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(biosSettingsSet) {
		log.V(1).Info("Skipped BIOSSettingsSet reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, biosSettingsSet, biosSettingsSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	serverList, err := r.getServersBySelector(ctx, biosSettingsSet)
	if err != nil {
		return ctrl.Result{}, err
	}

	switch len(biosSettingsSet.Spec.SettingsFlow) {
	case 0:
		log.V(1).Info("BIOSSettingsSet reconciliation ended")
		return ctrl.Result{}, nil
	case 1:
		return r.handleBiosSettings(ctx, log, serverList, biosSettingsSet)
	default:
		return r.handleBiosSettingsFlow(ctx, log, serverList, biosSettingsSet)
	}
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

	if err := r.createMissingBIOSVersions(ctx, log, serverList, ownedBiosSettings, biosSettingsSet); err != nil {
		log.Error(err, "failed to create resources")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Summary of servers and BIOSSettings", "Server count", len(serverList.Items),
		"BIOSVersion count", len(ownedBiosSettings.Items))

	if err := r.deleteOrphanBIOSVersions(ctx, log, serverList, ownedBiosSettings); err != nil {
		log.Error(err, "failed to cleanup resources")
		return ctrl.Result{}, err
	}

	log.V(1).Info("updating the status of BIOSSettingsSet")
	currentStatus := r.getOwnedBIOSSettingsSetStatus(ownedBiosSettings)
	currentStatus.FullyLabeledServers = int32(len(serverList.Items))

	err = r.updateStatus(ctx, log, currentStatus, biosSettingsSet)
	if err != nil {
		log.Error(err, "failed to update current Status")
		return ctrl.Result{}, err
	}
	// wait for any updates from owned resources
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsSetReconciler) handleBiosSettingsFlow(
	ctx context.Context,
	log logr.Logger,
	serverList *metalv1alpha1.ServerList,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) (ctrl.Result, error) {
	log.V(1).Info("to be implemented with BIOSSettingsFlow")
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsSetReconciler) createMissingBIOSVersions(
	ctx context.Context,
	log logr.Logger,
	serverList *metalv1alpha1.ServerList,
	biosSettingsList *metalv1alpha1.BIOSSettingsList,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) error {

	serverWithSettings := make(map[string]bool)
	for _, biosSettings := range biosSettingsList.Items {
		serverWithSettings[biosSettings.Spec.ServerRef.Name] = true
	}

	var errs []error
	for _, server := range serverList.Items {
		if !serverWithSettings[server.Name] {

			newBiosSettingsName := fmt.Sprintf("%s-%s", biosSettingsSet.Name, server.Name)
			var newBiosSetting *metalv1alpha1.BIOSSettings
			if len(newBiosSettingsName) > utilvalidation.DNS1123SubdomainMaxLength {
				log.V(1).Info("BiosSettings name is too long, it will be shortened using randam string", "name", newBiosSettingsName)
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
				newBiosSetting.Spec.ServerMaintenancePolicy = biosSettingsSet.Spec.ServerMaintenancePolicy
				newBiosSetting.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
				newBiosSetting.Spec.Version = biosSettingsSet.Spec.Version
				newBiosSetting.Spec.SettingsMap = biosSettingsSet.Spec.SettingsFlow[0].Settings
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

func (r *BIOSSettingsSetReconciler) deleteOrphanBIOSVersions(
	ctx context.Context,
	log logr.Logger,
	serverList *metalv1alpha1.ServerList,
	biosSettingsList *metalv1alpha1.BIOSSettingsList,
) error {

	serverWithSettings := make(map[string]bool)
	for _, server := range serverList.Items {
		serverWithSettings[server.Spec.BIOSSettingsRef.Name] = true
	}

	var errs []error
	for _, biosSettings := range biosSettingsList.Items {
		if !serverWithSettings[biosSettings.Name] {
			if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
				log.V(1).Info("waiting for BIOSSettings to move out of InProgress state", "BIOSSettings", biosSettings.Name, "status", biosSettings.Status)
				continue
			}
			if err := r.Delete(ctx, &biosSettings); err != nil {
				errs = append(errs, err)
			}
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
		return fmt.Errorf("failed to patch BIOSSettingsSet status: %w", err)
	}

	log.V(1).Info("Updated biosSettingsSet state ", "new state", currentStatus)

	return nil
}

func (r *BIOSSettingsSetReconciler) enqueueByServer(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	log.Info("server created/deleted/updated label")

	host := obj.(*metalv1alpha1.Server)

	biosSettingsSetList := &metalv1alpha1.BIOSSettingsSetList{}
	if err := r.List(ctx, biosSettingsSetList); err != nil {
		log.Error(err, "failed to list BIOSVersionSet")
		return nil
	}
	reqs := make([]ctrl.Request, 0)
	for _, biosSettingsSet := range biosSettingsSetList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&biosSettingsSet.Spec.ServerSelector)
		if err != nil {
			log.Error(err, "failed to convert label selector")
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
		Owns(&metalv1alpha1.BIOSSettings{}).
		Watches(
			&metalv1alpha1.Server{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueByServer),
			builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Complete(r)
}
