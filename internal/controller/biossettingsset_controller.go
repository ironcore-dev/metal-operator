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

const biosSettingsSetFinalizer = "metal.ironcore.dev/biossettingsset"

// BIOSSettingsSetReconciler reconciles a BIOSSettingsSet object
type BIOSSettingsSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingssets/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch

func (r *BIOSSettingsSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	biosSettingsSet := &metalv1alpha1.BIOSSettingsSet{}
	if err := r.Get(ctx, req.NamespacedName, biosSettingsSet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, biosSettingsSet)
}

func (r *BIOSSettingsSetReconciler) reconcileExists(ctx context.Context, set *metalv1alpha1.BIOSSettingsSet) (ctrl.Result, error) {
	if !set.DeletionTimestamp.IsZero() {
		return r.delete(ctx, set)
	}
	return r.reconcile(ctx, set)
}

func (r *BIOSSettingsSetReconciler) delete(ctx context.Context, set *metalv1alpha1.BIOSSettingsSet) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Deleting BIOSSettingsSet")

	if !controllerutil.ContainsFinalizer(set, biosSettingsSetFinalizer) {
		return ctrl.Result{}, nil
	}

	// Handle propagation of ignore annotation to child resources when parent is being deleted.
	// That way the deleted annotations can be passed to children before parent is deleted.
	if err := r.handleIgnoreAnnotationPropagation(ctx, set); err != nil {
		return ctrl.Result{}, err
	}

	ownedBIOSSettings, err := r.getOwnedBIOSSettings(ctx, set)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned BIOSSettings resources %w", err)
	}

	deletableBIOSSettings := map[string]struct{}{}
	for _, biosSettings := range ownedBIOSSettings.Items {
		if biosSettings.Status.State != metalv1alpha1.BIOSSettingsStateInProgress {
			deletableBIOSSettings[biosSettings.Name] = struct{}{}
		} else if biosSettings.Spec.ServerMaintenanceRef == nil {
			deletableBIOSSettings[biosSettings.Name] = struct{}{}
		}
	}

	if len(ownedBIOSSettings.Items) != len(deletableBIOSSettings) ||
		int32(len(ownedBIOSSettings.Items)) != set.Status.AvailableBIOSSettings {
		status := r.getOwnedBIOSSettingsSetStatus(ownedBIOSSettings)

		if err = r.patchStatus(ctx, status, set); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update BIOSSettingsSet status %w", err)
		}
		log.V(1).Info("Updated BIOSSettingsSet state", "Status", status)

		// Handle propagation of retry annotation to child when parent is being deleted.
		// That way the deleted annotations can be passed to children before parent is deleted.
		if err := r.handleRetryAnnotationPropagation(ctx, set); err != nil {
			return ctrl.Result{}, err
		}

		log.Info("Waiting for the created BIOSSettings to reach terminal status")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, set, biosSettingsSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured that the finalizer is removed")

	log.V(1).Info("Deleted BIOSSettingsSet")
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsSetReconciler) handleIgnoreAnnotationPropagation(ctx context.Context, set *metalv1alpha1.BIOSSettingsSet) error {
	log := ctrl.LoggerFrom(ctx)
	ownedBiosSettings, err := r.getOwnedBIOSSettings(ctx, set)
	if err != nil {
		return err
	}

	if len(ownedBiosSettings.Items) == 0 {
		log.V(1).Info("No BIOSSettings found, skipping ignore annotation propagation")
		return nil
	}

	return handleIgnoreAnnotationPropagation(ctx, r.Client, set, ownedBiosSettings)
}

func (r *BIOSSettingsSetReconciler) handleRetryAnnotationPropagation(ctx context.Context, set *metalv1alpha1.BIOSSettingsSet) error {
	log := ctrl.LoggerFrom(ctx)
	ownedBiosSettings, err := r.getOwnedBIOSSettings(ctx, set)
	if err != nil {
		return err
	}

	if len(ownedBiosSettings.Items) == 0 {
		log.V(1).Info("No BIOSSettings found, skipping retry annotation propagation")
		return nil
	}

	log.V(1).Info("Propagating retry annotation to owned BIOSSettings resources")
	return handleRetryAnnotationPropagation(ctx, r.Client, set, ownedBiosSettings)
}

func (r *BIOSSettingsSetReconciler) reconcile(ctx context.Context, set *metalv1alpha1.BIOSSettingsSet) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling BIOSSettingsSet")

	if err := r.handleIgnoreAnnotationPropagation(ctx, set); err != nil {
		return ctrl.Result{}, err
	}

	if shouldIgnoreReconciliation(set) {
		log.V(1).Info("Ignoring BIOSSettingsSet reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, set, biosSettingsSetFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	serverList, err := r.getServersBySelector(ctx, set)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get Servers for label selector %w", err)
	}
	return r.handleBIOSSettings(ctx, serverList, set)
}

func (r *BIOSSettingsSetReconciler) handleBIOSSettings(ctx context.Context, servers *metalv1alpha1.ServerList, set *metalv1alpha1.BIOSSettingsSet) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	ownedBiosSettings, err := r.getOwnedBIOSSettings(ctx, set)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.createMissingBIOSSettings(ctx, servers, ownedBiosSettings, set); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create missing BIOSSettings: %w", err)
	}

	log.V(1).Info("Summary of Servers and BIOSSettings", "ServerCount", len(servers.Items),
		"BIOSSettingsCount", len(ownedBiosSettings.Items))

	if err := r.deleteOrphanBIOSSettings(ctx, servers, ownedBiosSettings); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete orphaned BIOSSettings: %w", err)
	}

	if err := r.patchBIOSSettingsFromTemplate(ctx, &set.Spec.BIOSSettingsTemplate, ownedBiosSettings); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch BIOSSettings spec from template: %w", err)
	}

	log.V(1).Info("Updating the status of BIOSSettingsSet")
	status := r.getOwnedBIOSSettingsSetStatus(ownedBiosSettings)
	status.FullyLabeledServers = int32(len(servers.Items))

	if err = r.patchStatus(ctx, status, set); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update BIOSSettingsSet status %w", err)
	}
	log.V(1).Info("Updated BIOSSettingsSet state", "Status", status)

	// handle retry annotation - remove the annotation after retrying reconciliation
	if err := r.handleRetryAnnotationPropagation(ctx, set); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BIOSSettingsSetReconciler) createMissingBIOSSettings(ctx context.Context, servers *metalv1alpha1.ServerList, settings *metalv1alpha1.BIOSSettingsList, set *metalv1alpha1.BIOSSettingsSet) error {
	log := ctrl.LoggerFrom(ctx)
	serverWithSettings := make(map[string]struct{})
	for _, settings := range settings.Items {
		serverWithSettings[settings.Spec.ServerRef.Name] = struct{}{}
	}

	var errs []error
	for _, server := range servers.Items {
		if _, ok := serverWithSettings[server.Name]; !ok {
			if server.Spec.BIOSSettingsRef != nil {
				// this is the case where the server already has a different BIOSSettingsRef, and we should not create a new one for this server
				log.V(1).Info("Server already has a BIOSSettings", "Server", server.Name, "BIOSSettings", server.Spec.BIOSSettingsRef.Name)
				continue
			}
			newBiosSettingsName := fmt.Sprintf("%s-%s", set.Name, server.Name)
			var newBiosSetting *metalv1alpha1.BIOSSettings
			if len(newBiosSettingsName) > utilvalidation.DNS1123SubdomainMaxLength {
				log.V(1).Info("BIOSSettings name is too long and will be shortened using a random string", "Name", newBiosSettingsName)
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
				newBiosSetting.Spec.BIOSSettingsTemplate = *set.Spec.BIOSSettingsTemplate.DeepCopy()
				newBiosSetting.Spec.ServerRef = &corev1.LocalObjectReference{Name: server.Name}
				return controllerutil.SetControllerReference(set, newBiosSetting, r.Client.Scheme())
			})
			if err != nil {
				errs = append(errs, err)
			}
			log.V(1).Info("Created BIOSSettings", "BIOSSettings", newBiosSetting.Name, "Server", server.Name, "Operation", opResult)
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSSettingsSetReconciler) deleteOrphanBIOSSettings(ctx context.Context, servers *metalv1alpha1.ServerList, settings *metalv1alpha1.BIOSSettingsList) error {
	log := ctrl.LoggerFrom(ctx)
	serverMap := make(map[string]bool)
	for _, server := range servers.Items {
		serverMap[server.Name] = true
	}

	var errs []error
	for _, biosSettings := range settings.Items {
		if _, ok := serverMap[biosSettings.Spec.ServerRef.Name]; !ok {
			if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
				log.V(1).Info("Waiting for BIOSSettings to move out of InProgress state", "BIOSSettings", biosSettings.Name, "Status", biosSettings.Status)
				continue
			}
			if err := r.Delete(ctx, &biosSettings); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSSettingsSetReconciler) patchBIOSSettingsFromTemplate(ctx context.Context, template *metalv1alpha1.BIOSSettingsTemplate, settingsList *metalv1alpha1.BIOSSettingsList) error {
	log := ctrl.LoggerFrom(ctx)
	if len(settingsList.Items) == 0 {
		log.V(1).Info("No BIOSSettings found, skipping template update")
		return nil
	}

	var errs []error
	for _, settings := range settingsList.Items {
		if settings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
			continue
		}
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &settings, func() error {
			settings.Spec.BIOSSettingsTemplate = *template.DeepCopy()
			return nil
		})
		if err != nil {
			errs = append(errs, err)
		}
		if opResult != controllerutil.OperationResultNone {
			log.V(1).Info("Patched BIOSSettings with updated spec", "BIOSSettings", settings.Name, "Operation", opResult)
			settingsBase := settings.DeepCopy()
			settings.Status.AutoRetryCountRemaining = settings.Spec.FailedAutoRetryCount
			if err = r.Status().Patch(ctx, &settings, client.MergeFrom(settingsBase)); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (r *BIOSSettingsSetReconciler) getOwnedBIOSSettingsSetStatus(settingsList *metalv1alpha1.BIOSSettingsList) *metalv1alpha1.BIOSSettingsSetStatus {
	status := &metalv1alpha1.BIOSSettingsSetStatus{}
	status.AvailableBIOSSettings = int32(len(settingsList.Items))
	for _, settings := range settingsList.Items {
		switch settings.Status.State {
		case metalv1alpha1.BIOSSettingsStateApplied:
			status.CompletedBIOSSettings += 1
		case metalv1alpha1.BIOSSettingsStateFailed:
			status.FailedBIOSSettings += 1
		case metalv1alpha1.BIOSSettingsStateInProgress:
			status.InProgressBIOSSettings += 1
		case metalv1alpha1.BIOSSettingsStatePending, "":
			status.PendingBIOSSettings += 1
		}
	}
	return status
}

func (r *BIOSSettingsSetReconciler) getOwnedBIOSSettings(ctx context.Context, set *metalv1alpha1.BIOSSettingsSet) (*metalv1alpha1.BIOSSettingsList, error) {
	settingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := clientutils.ListAndFilterControlledBy(ctx, r.Client, set, settingsList); err != nil {
		return nil, err
	}
	return settingsList, nil
}

func (r *BIOSSettingsSetReconciler) getServersBySelector(ctx context.Context, set *metalv1alpha1.BIOSSettingsSet) (*metalv1alpha1.ServerList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&set.Spec.ServerSelector)
	if err != nil {
		return nil, err
	}
	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}

	return serverList, nil
}

func (r *BIOSSettingsSetReconciler) patchStatus(ctx context.Context, status *metalv1alpha1.BIOSSettingsSetStatus, set *metalv1alpha1.BIOSSettingsSet) error {
	setBase := set.DeepCopy()
	set.Status = *status

	if err := r.Status().Patch(ctx, set, client.MergeFrom(setBase)); err != nil {
		return err
	}
	return nil
}

func (r *BIOSSettingsSetReconciler) enqueueByServer(ctx context.Context, obj client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	server := obj.(*metalv1alpha1.Server)
	reqs := make(map[client.ObjectKey]bool)

	// Get all BIOSSettingsSets and check if the Server matches their selectors
	setList := &metalv1alpha1.BIOSSettingsSetList{}
	if err := r.List(ctx, setList); err == nil {
		for _, set := range setList.Items {
			sel, _ := metav1.LabelSelectorAsSelector(&set.Spec.ServerSelector)
			if sel.Matches(labels.Set(server.GetLabels())) {
				reqs[client.ObjectKeyFromObject(&set)] = true
			}
		}
	}

	// Additionally, check if the Server has a BIOSSettingsRef and enqueue its owner BIOSSettingsSet
	if server.Spec.BIOSSettingsRef != nil {
		settings := &metalv1alpha1.BIOSSettings{}
		if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.BIOSSettingsRef.Name}, settings); err != nil {
			log.Error(err, "failed to get BIOSSettings referenced by Server", "Server", server.Name, "BIOSSettings", server.Spec.BIOSSettingsRef.Name)
			return nil
		}
		owner := metav1.GetControllerOf(settings)
		if owner != nil && owner.Kind == "BIOSSettingsSet" {
			reqs[client.ObjectKey{Namespace: settings.Namespace, Name: owner.Name}] = true
		}
	}

	result := make([]ctrl.Request, 0, len(reqs))
	for k := range reqs {
		result = append(result, ctrl.Request{NamespacedName: k})
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *BIOSSettingsSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSSettingsSet{}).
		Owns(&metalv1alpha1.BIOSSettings{}).
		Watches(&metalv1alpha1.Server{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueByServer),
			builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Complete(r)
}
