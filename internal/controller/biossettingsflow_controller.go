// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

// BIOSSettingsFlowReconciler reconciles a BIOSSettingsFlow object
type BIOSSettingsFlowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const BIOSSettingsFlowFinalizer = "metal.ironcore.dev/biossettingsflow"

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingsflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingsflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettingsflows/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=biossettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *BIOSSettingsFlowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	biosSettingsFlow := &metalv1alpha1.BIOSSettingsFlow{}
	if err := r.Get(ctx, req.NamespacedName, biosSettingsFlow); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(1).Info("Reconciling biosSettingsFlow")

	return r.reconcileExists(ctx, log, biosSettingsFlow)
}

func (r *BIOSSettingsFlowReconciler) reconcileExists(
	ctx context.Context,
	log logr.Logger,
	biosSettingsFlow *metalv1alpha1.BIOSSettingsFlow,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if !biosSettingsFlow.DeletionTimestamp.IsZero() {
		// todo remove log
		log.V(1).Info("object is being deleted", "biosSettingsFlow", biosSettingsFlow, "biosSettingsFlow", biosSettingsFlow.Status.State)
		return r.delete(ctx, log, biosSettingsFlow)
	}

	return r.reconcile(ctx, log, biosSettingsFlow)
}

func (r *BIOSSettingsFlowReconciler) delete(
	ctx context.Context,
	log logr.Logger,
	biosSettingsFlow *metalv1alpha1.BIOSSettingsFlow,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(biosSettingsFlow, BIOSSettingsFlowFinalizer) {
		return ctrl.Result{}, nil
	}

	biosSettings := &metalv1alpha1.BIOSSettings{}
	var err error
	if err = r.Get(ctx, client.ObjectKey{Name: biosSettingsFlow.Name}, biosSettings); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get BIOSSettings %s: %w", biosSettingsFlow.Name, err)
		}
	}

	if biosSettingsFlow.Status.State == metalv1alpha1.BIOSSettingsFlowStateInProgress {
		log.V(1).Info("waiting for biosSettings to complete", "biosSettings Name", biosSettings.Name, "state", biosSettings.Status.State)
		return r.reconcile(ctx, log, biosSettingsFlow)
	}

	if err == nil && metav1.IsControlledBy(biosSettings, biosSettingsFlow) {
		log.V(1).Info("Waiting for biosSettings to be deleted", "biosSettings Name", biosSettings.Name, "state", biosSettings.Status.State)
		return ctrl.Result{}, err
	}

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, biosSettingsFlow, BIOSSettingsFlowFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	log.V(1).Info("biosSettingsFlow is deleted")
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsFlowReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	biosSettingsFlow *metalv1alpha1.BIOSSettingsFlow,
) (ctrl.Result, error) {
	if shouldIgnoreReconciliation(biosSettingsFlow) {
		log.V(1).Info("Skipped BIOSSettingFlow reconciliation")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, biosSettingsFlow, BIOSSettingsFlowFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	if len(biosSettingsFlow.Spec.SettingsFlow) == 0 {
		log.V(1).Info("Skipped BIOSSettingFlow as no settings found")
		err := r.updateBiosSettingsFlowStatus(ctx, log, biosSettingsFlow, metalv1alpha1.BIOSSettingsFlowStateApplied)
		return ctrl.Result{}, err
	}

	if modified, err := r.sortAndPatchSettingsFlow(ctx, log, biosSettingsFlow); err != nil || modified {
		return ctrl.Result{}, err
	}

	biosSettings := &metalv1alpha1.BIOSSettings{}
	if err := r.Get(ctx, client.ObjectKey{Name: biosSettingsFlow.Name}, biosSettings); err != nil {
		// make sure the server is Available.
		server := &metalv1alpha1.Server{}
		serverErr := r.Get(ctx, client.ObjectKey{Name: biosSettingsFlow.Spec.ServerRef.Name}, server)
		if apierrors.IsNotFound(err) && serverErr == nil {
			// create the biosSettings for the first time
			var err error
			if err = r.createOrPatchBiosSettings(ctx, log, biosSettingsFlow, biosSettings); err != nil {
				log.V(1).Error(err, "Failed to get biosSettings")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, errors.Join(err, fmt.Errorf("unable to fetch the referenced server: %v", serverErr))
	}

	log.V(1).Info("Check for BIOSSettings status",
		"current setting Priority", biosSettings.Spec.CurrentSettingPriority,
		"biosSettings status", biosSettings.Status.State)
	if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateFailed {
		err := r.updateBiosSettingsFlowStatus(ctx, log, biosSettingsFlow, metalv1alpha1.BIOSSettingsFlowStateFailed)
		return ctrl.Result{}, err
	}

	if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateInWaiting {
		// need to patch the next set of biosSettings, or mark all done
		log.V(1).Info("Patching bios state with next settings", "currentPriority", biosSettings.Spec.CurrentSettingPriority)
		return r.patchBiosSettings(ctx, log, biosSettingsFlow, biosSettings)
	}

	if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateApplied {
		err := r.updateBiosSettingsFlowStatus(ctx, log, biosSettingsFlow, metalv1alpha1.BIOSSettingsFlowStateApplied)
		return ctrl.Result{}, err
	}

	log.V(1).Info("Reconciled biosSettingsFlow")
	err := r.updateBiosSettingsFlowStatus(ctx, log, biosSettingsFlow, metalv1alpha1.BIOSSettingsFlowStateInProgress)
	return ctrl.Result{}, err
}

func (r *BIOSSettingsFlowReconciler) patchBiosSettings(
	ctx context.Context,
	log logr.Logger,
	biosSettingsFlow *metalv1alpha1.BIOSSettingsFlow,
	biosSettings *metalv1alpha1.BIOSSettings,
) (ctrl.Result, error) {
	// if all settings are applied, nothing to update
	if biosSettings.Status.AppliedSettingPriority == math.MaxInt32 {
		return ctrl.Result{}, nil
	}
	// find the next setting to apply
	currentSettings := map[string]string{}
	for idx, settings := range biosSettingsFlow.Spec.SettingsFlow {
		maps.Copy(currentSettings, settings.Settings)
		if settings.Priority == biosSettings.Status.AppliedSettingPriority {
			maps.Copy(currentSettings, biosSettingsFlow.Spec.SettingsFlow[idx+1].Settings)
			biosSettingsBase := biosSettings.DeepCopy()
			if idx+1 == len(biosSettingsFlow.Spec.SettingsFlow)-1 {
				// set the last update as the last update
				biosSettings.Spec.CurrentSettingPriority = math.MaxInt32
			} else {
				biosSettings.Spec.CurrentSettingPriority = biosSettingsFlow.Spec.SettingsFlow[idx+1].Priority
			}
			log.V(1).Info("patching bios state", "settings", currentSettings, "Priority", biosSettings.Spec.CurrentSettingPriority)
			biosSettings.Spec.SettingsMap = currentSettings
			return ctrl.Result{}, r.Patch(ctx, biosSettings, client.MergeFrom(biosSettingsBase))
		}
	}
	return ctrl.Result{}, nil
}

func (r *BIOSSettingsFlowReconciler) createOrPatchBiosSettings(
	ctx context.Context,
	log logr.Logger,
	biosSettingsFlow *metalv1alpha1.BIOSSettingsFlow,
	biosSettings *metalv1alpha1.BIOSSettings,
) error {
	biosSettings.Name = biosSettingsFlow.Name
	biosSettings.Namespace = biosSettingsFlow.Namespace

	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, biosSettings, func() error {
		if len(biosSettingsFlow.Spec.SettingsFlow) == 1 {
			biosSettings.Spec.CurrentSettingPriority = math.MaxInt32
		} else {
			biosSettings.Spec.CurrentSettingPriority = biosSettingsFlow.Spec.SettingsFlow[0].Priority
		}
		biosSettings.Spec.ServerMaintenancePolicy = biosSettingsFlow.Spec.ServerMaintenancePolicy
		biosSettings.Spec.Version = biosSettingsFlow.Spec.Version
		biosSettings.Spec.SettingsMap = biosSettingsFlow.Spec.SettingsFlow[0].Settings
		biosSettings.Spec.ServerRef = &corev1.LocalObjectReference{Name: biosSettingsFlow.Spec.ServerRef.Name}
		return controllerutil.SetControllerReference(biosSettingsFlow, biosSettings, r.Client.Scheme())
	})
	if err != nil {
		return fmt.Errorf("failed to create or patch biosSettings: %w", err)
	}
	log.V(1).Info("Created biosSettings", "biosSettings", biosSettings.Name, "Operation", opResult)
	return nil
}

func (r *BIOSSettingsFlowReconciler) sortAndPatchSettingsFlow(
	ctx context.Context,
	log logr.Logger,
	biosSettingsFlow *metalv1alpha1.BIOSSettingsFlow,
) (bool, error) {

	if len(biosSettingsFlow.Spec.SettingsFlow) < 2 {
		return false, nil
	}
	biosSettingsFlowBase := biosSettingsFlow.DeepCopy()
	sort.Slice(biosSettingsFlow.Spec.SettingsFlow, func(i, j int) bool {
		return biosSettingsFlow.Spec.SettingsFlow[i].Priority < biosSettingsFlow.Spec.SettingsFlow[j].Priority
	})

	if biosSettingsFlow.Spec.SettingsFlow[0].Priority <= 0 {
		return false, fmt.Errorf(
			"the lowest priority item in the settings flow must have a priority greater than 0"+
				" got %d", biosSettingsFlow.Spec.SettingsFlow[0].Priority)
	}

	changed := false

	for idx, settings := range biosSettingsFlowBase.Spec.SettingsFlow {
		if settings.Priority != biosSettingsFlow.Spec.SettingsFlow[idx].Priority {
			changed = true
			break
		}
	}

	if !changed {
		return false, nil
	}
	if err := r.Patch(ctx, biosSettingsFlow, client.MergeFrom(biosSettingsFlowBase)); err != nil {
		return true, fmt.Errorf("failed to patch BIOSSettingsFlow SettingsFlow: %w", err)
	}
	log.V(1).Info("Updated biosSettingsFlow SettingsFlow", "Updated SettingsFlow", biosSettingsFlow.Spec.SettingsFlow)
	return true, nil
}

func (r *BIOSSettingsFlowReconciler) updateBiosSettingsFlowStatus(
	ctx context.Context,
	log logr.Logger,
	biosSettingsFlow *metalv1alpha1.BIOSSettingsFlow,
	state metalv1alpha1.BIOSSettingsFlowState,
) error {
	if biosSettingsFlow.Status.State == state {
		return nil
	}

	biosSettingsFlowBase := biosSettingsFlow.DeepCopy()
	biosSettingsFlow.Status.State = state

	if err := r.Status().Patch(ctx, biosSettingsFlow, client.MergeFrom(biosSettingsFlowBase)); err != nil {
		return fmt.Errorf("failed to patch BIOSSettingsFlow status: %w", err)
	}

	log.V(1).Info("Updated biosSettingsFlow state ", "new state", state)

	return nil
}

func (r *BIOSSettingsFlowReconciler) enqueueBiosFlowByBiosSettings(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)
	BIOSSettings := obj.(*metalv1alpha1.BIOSSettings)
	// state we are not interested
	if BIOSSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress ||
		BIOSSettings.Status.State == metalv1alpha1.BIOSSettingsStatePending {
		return nil
	}

	BIOSSettingsFlowList := &metalv1alpha1.BIOSSettingsFlowList{}
	if err := r.List(ctx, BIOSSettingsFlowList); err != nil {
		log.Error(err, "failed to list BIOSSettingsFlows")
		return nil
	}

	for _, biosSettingsFlow := range BIOSSettingsFlowList.Items {
		if biosSettingsFlow.Spec.ServerRef.Name == BIOSSettings.Spec.ServerRef.Name {
			if BIOSSettings.Spec.CurrentSettingPriority > 0 {
				// we expect the BIOSSettings to have same namespace and name as the BIOSSettingsFlow
				return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: biosSettingsFlow.Namespace, Name: biosSettingsFlow.Name}}}
			}
			return nil
		}
	}
	return nil
}

func (r *BIOSSettingsFlowReconciler) enqueueByServer(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {

	log := ctrl.LoggerFrom(ctx)
	server := obj.(*metalv1alpha1.Server)

	BIOSSettingsFlowList := &metalv1alpha1.BIOSSettingsFlowList{}
	if err := r.List(ctx, BIOSSettingsFlowList); err != nil {
		log.Error(err, "failed to list BIOSSettingsFlows")
		return nil
	}

	for _, biosSettingsFlow := range BIOSSettingsFlowList.Items {
		if biosSettingsFlow.Spec.ServerRef.Name == server.Name {
			return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: biosSettingsFlow.Namespace, Name: biosSettingsFlow.Name}}}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BIOSSettingsFlowReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BIOSSettingsFlow{}).
		Watches(&metalv1alpha1.BIOSSettings{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBiosFlowByBiosSettings)).
		Watches(&metalv1alpha1.Server{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueByServer),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return true
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return false
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return true
				}, GenericFunc: func(e event.GenericEvent) bool {
					return false
				}})).
		Complete(r)
}
