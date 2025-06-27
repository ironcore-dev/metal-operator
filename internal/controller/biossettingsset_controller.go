// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// BIOSSettingsSetReconciler reconciles a BIOSSettingsSet object
type BIOSSettingsSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const biosSettingsSetFinalizer = "metal.ironcore.dev/biossettingsSet"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BIOSSettingsSet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
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
	currentStatus := r.getOwnedBIOSSettingsStatus(ownedBiosSettings)

	if currentStatus.TotalSettings != (currentStatus.Completed + currentStatus.Failed) {
		err = r.updateStatus(ctx, log, currentStatus, biosSettingsSet)
		if err != nil {
			log.Error(err, "failed to update current Status")
			return ctrl.Result{}, err
		}
		log.Info("Waiting on the created BIOSSettings to reach terminal status")
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
	}

	if err := r.deleteBIOSSettings(ctx, log, &metalv1alpha1.ServerList{}, ownedBiosSettings); err != nil {
		log.Error(err, "failed to cleanup created resources")
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured cleaning up")

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

	if len(serverList.Items) > len(ownedBiosSettings.Items) {
		log.V(1).Info("new server found creating respective BIOSSettings")
		if err := r.createBIOSSettings(ctx, log, serverList, ownedBiosSettings, biosSettingsSet); err != nil {
			log.Error(err, "failed to create resources")
			return ctrl.Result{}, err
		}
		// wait for any updates from owned resources
		return ctrl.Result{}, nil
	} else if len(serverList.Items) < len(ownedBiosSettings.Items) {
		log.V(1).Info("servers deleted, deleting respective BIOSSettings")
		if err := r.deleteBIOSSettings(ctx, log, serverList, ownedBiosSettings); err != nil {
			log.Error(err, "failed to cleanup resources")
			return ctrl.Result{}, err
		}
		// wait for any updates from owned resources
		return ctrl.Result{}, nil
	}

	log.V(1).Info("updating the status of BIOSSettingsSet")
	currentStatus := r.getOwnedBIOSSettingsStatus(ownedBiosSettings)
	currentStatus.TotalServers = int32(len(serverList.Items))

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
	log.V(1).Info("to be implemented BIOSSettingsFlow")
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsSetReconciler) createBIOSSettings(
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
			newBiosSetting := &metalv1alpha1.BIOSSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-%s", biosSettingsSet.Name, server.Name),
				}}

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

func (r *BIOSSettingsSetReconciler) deleteBIOSSettings(
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
				log.V(1).Info("waiting for biosSettings to move out of InProgress state", "BIOSSettings", biosSettings.Name, "status", biosSettings.Status)
				continue
			}
			if err := r.Delete(ctx, &biosSettings); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

func (r *BIOSSettingsSetReconciler) getOwnedBIOSSettingsStatus(
	biosSettingsList *metalv1alpha1.BIOSSettingsList,
) *metalv1alpha1.BIOSSettingsSetStatus {
	currentStatus := &metalv1alpha1.BIOSSettingsSetStatus{}
	currentStatus.TotalSettings = int32(len(biosSettingsList.Items))
	for _, biosSettings := range biosSettingsList.Items {
		switch biosSettings.Status.State {
		case metalv1alpha1.BIOSSettingsStateApplied:
			currentStatus.Completed += 1
		case metalv1alpha1.BIOSSettingsStateFailed:
			currentStatus.Failed += 1
		case metalv1alpha1.BIOSSettingsStateInProgress:
			currentStatus.InProgress += 1
		case metalv1alpha1.BIOSSettingsStatePending, "":
			currentStatus.InProgress += 1
		}
	}
	return currentStatus
}

func (r *BIOSSettingsSetReconciler) getOwnedBIOSSettings(
	ctx context.Context,
	biosSettingsSet *metalv1alpha1.BIOSSettingsSet,
) (*metalv1alpha1.BIOSSettingsList, error) {
	BiosSettingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, biosSettingsSet, BiosSettingsList); err != nil {
		return nil, err
	}
	return BiosSettingsList, nil
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

	biosSettingsSet.Status.Completed = currentStatus.Completed
	biosSettingsSet.Status.TotalServers = currentStatus.TotalServers
	biosSettingsSet.Status.TotalSettings = currentStatus.TotalSettings
	biosSettingsSet.Status.Failed = currentStatus.Failed
	biosSettingsSet.Status.InProgress = currentStatus.InProgress
	biosSettingsSet.Status.Pending = currentStatus.Pending

	if err := r.Status().Patch(ctx, biosSettingsSet, client.MergeFrom(biosSettingsSetBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSSettingsSet status: %w", err)
	}

	log.V(1).Info("Updated biosSettingsSet state ", "new state", currentStatus)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BIOSSettingsSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSSettingsSet{}).
		Owns(&metalv1alpha1.BIOSSettings{}).
		Complete(r)
}
