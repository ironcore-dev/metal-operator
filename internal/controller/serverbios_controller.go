// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/fmi"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const serverBIOSFinalizer = "metal.ironcore.dev/serverbios"

// ServerBIOSReconciler reconciles a ServerBIOS object
type ServerBIOSReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	TaskRunnerClient fmi.TaskRunnerClient
	RequeueInterval  time.Duration
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbioses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbioses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbioses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerBIOSReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	serverBIOS := &metalv1alpha1.ServerBIOS{}
	if err := r.Get(ctx, req.NamespacedName, serverBIOS); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconciliationRequired(ctx, log, serverBIOS)
}

// Determine whether reconciliation is required. It's not required if:
// - object is being deleted;
// - object does not contain reference to server;
// - object contains reference to server, but server references to another object;
func (r *ServerBIOSReconciler) reconciliationRequired(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	// if object is being deleted - reconcile deletion
	if !serverBIOS.DeletionTimestamp.IsZero() {
		log.V(1).Info("object is being deleted")
		return r.reconcileDeletion(ctx, log, serverBIOS)
	}

	// if object does not refer to server object - stop reconciliation
	if serverBIOS.Spec.ServerRef == (corev1.LocalObjectReference{}) {
		log.V(1).Info("object does not refer to server object")
		return ctrl.Result{}, nil
	}

	// if referred server contains reference to different ServerBIOS object - stop reconciliation
	server, err := r.getReferredServer(ctx, log, serverBIOS.Spec.ServerRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	if server.Spec.BIOSSettingsRef != (corev1.LocalObjectReference{}) &&
		server.Spec.BIOSSettingsRef.Name != serverBIOS.Name {
		log.V(1).Info("referred server contains reference to different ServerBIOS object")
		return ctrl.Result{}, nil
	}

	// patch server with serverbios reference
	if server.Spec.BIOSSettingsRef == (corev1.LocalObjectReference{}) {
		reference := corev1.LocalObjectReference{Name: serverBIOS.Name}
		if err := r.patchBIOSSettingsRef(ctx, log, &server, reference); err != nil {
			return ctrl.Result{}, err
		}
	}
	log.V(1).Info("ensured mutual reference", "server", server.Name)

	return r.reconcile(ctx, log, serverBIOS)
}

func (r *ServerBIOSReconciler) reconcileDeletion(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(serverBIOS, serverBIOSFinalizer) {
		return ctrl.Result{}, nil
	}
	if err := r.cleanupReferences(ctx, log, serverBIOS); err != nil {
		log.Error(err, "failed to cleanup references")
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured references were cleaned up")

	_, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, serverBIOS, serverBIOSFinalizer)
	return ctrl.Result{}, err
}

func (r *ServerBIOSReconciler) cleanupReferences(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) error {
	if serverBIOS.Spec.ServerRef == (corev1.LocalObjectReference{}) {
		return nil
	}
	server, err := r.getReferredServer(ctx, log, serverBIOS.Spec.ServerRef.Name)
	if err != nil {
		return err
	}
	if server.Spec.BIOSSettingsRef == (corev1.LocalObjectReference{}) {
		return nil
	}
	if server.Spec.BIOSSettingsRef.Name != serverBIOS.Name {
		return nil
	}
	return r.patchBIOSSettingsRef(ctx, log, &server, corev1.LocalObjectReference{})
}

// Reconciliation flow for ServerBIOS:
//  1. Invoke BIOS version and settings scan
//  2. If observed BIOS version is equal to desired BIOS version - update BIOS version in status
//  3. If observed BIOS settings are equal to desired BIOS settings - update BIOS settings in status
//  4. If BIOS version and(or) settings require update:
//     4.1. If server is not in Maintenance state - invoke server maintenance and exit
//     4.2. If server is in Maintenance state - invoke BIOS version or settings update jobs and exit
//  5. Patch server BIOS object status
func (r *ServerBIOSReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, serverBIOS, serverBIOSFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured finalizer has been added")

	updateRequired, err := r.reconcileScan(ctx, log, serverBIOS)
	if err != nil {
		return ctrl.Result{}, err
	}
	if updateRequired {
		return r.reconcileUpdate(ctx, log, serverBIOS)

	}
	return ctrl.Result{RequeueAfter: r.RequeueInterval}, nil
}

func (r *ServerBIOSReconciler) reconcileScan(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (bool, error) {
	log.V(1).Info("invoking scan job")
	result, err := r.TaskRunnerClient.Scan(ctx, serverBIOS.Name)
	if err != nil {
		return false, err
	}
	serverBIOSBase := serverBIOS.DeepCopy()
	versionUpdateRequired := serverBIOS.Spec.BIOS.Version != "" && (serverBIOS.Spec.BIOS.Version != result.Version)
	if !versionUpdateRequired {
		serverBIOS.Status.BIOS.Version = result.Version
	}
	settingsUpdateRequired := len(serverBIOS.Spec.BIOS.Settings) != 0 &&
		biosSettingsUpdateRequired(serverBIOS.Spec.BIOS.Settings, result.Settings)
	if !settingsUpdateRequired {
		serverBIOS.Status.BIOS.Settings = result.Settings
	}
	updateRequired := versionUpdateRequired || settingsUpdateRequired
	return updateRequired, r.Status().Patch(ctx, serverBIOS, client.MergeFrom(serverBIOSBase))
}

func (r *ServerBIOSReconciler) reconcileUpdate(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	server, err := r.getReferredServer(ctx, log, serverBIOS.Spec.ServerRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	// if referred server is not in Maintenance state - invoke server maintenance and stop reconciliation
	if server.Status.State != metalv1alpha1.ServerStateMaintenance {
		// todo: put server into Maintenance state
		//  REF: https://github.com/ironcore-dev/metal-operator/issues/76
		return ctrl.Result{RequeueAfter: r.RequeueInterval}, nil
	}
	// if desired bios version does not match actual version - run version update
	if serverBIOS.Spec.BIOS.Version != serverBIOS.Status.BIOS.Version {
		return r.reconcileVersionUpdate(ctx, log, serverBIOS)
	}
	// if desired bios settings do not match actual settings - run settings update
	if !cmp.Equal(serverBIOS.Spec.BIOS.Settings, serverBIOS.Status.BIOS.Settings) {
		return r.reconcileSettingsUpdate(ctx, log, serverBIOS)
	}
	return ctrl.Result{}, nil
}

func (r *ServerBIOSReconciler) reconcileVersionUpdate(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	log.V(1).Info("invoking version update job")
	return ctrl.Result{}, r.TaskRunnerClient.VersionUpdate(ctx, serverBIOS.Name)
}

func (r *ServerBIOSReconciler) reconcileSettingsUpdate(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	log.V(1).Info("invoking settings update job")
	result, err := r.TaskRunnerClient.SettingsApply(ctx, serverBIOS.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	server, err := r.getReferredServer(ctx, log, serverBIOS.Spec.ServerRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, r.patchServerCondition(ctx, &server, result.RebootRequired)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBIOSReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBIOS{}).
		Complete(r)
}

func (r *ServerBIOSReconciler) getReferredServer(
	ctx context.Context,
	log logr.Logger,
	name string,
) (metalv1alpha1.Server, error) {
	key := client.ObjectKey{Name: name, Namespace: metav1.NamespaceNone}
	server := metalv1alpha1.Server{}
	if err := r.Get(ctx, key, &server); err != nil {
		log.Error(err, "failed to get referred server")
		return server, err
	}
	return server, nil
}

func (r *ServerBIOSReconciler) patchBIOSSettingsRef(
	ctx context.Context,
	log logr.Logger,
	server *metalv1alpha1.Server,
	serverBIOSReference corev1.LocalObjectReference,
) error {
	var err error
	serverBase := server.DeepCopy()
	server.Spec.BIOSSettingsRef = serverBIOSReference
	if err = r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		log.Error(err, "failed to patch bios settings ref")
	}
	return err
}

// patchServerCondition patches the Server status with the given condition.
func (r *ServerBIOSReconciler) patchServerCondition(ctx context.Context, server *metalv1alpha1.Server, reboot bool) error {
	status := metav1.ConditionFalse
	reason := ""
	if reboot {
		status = metav1.ConditionTrue
		reason = "BIOSSettingsChanged"
	}
	serverBase := server.DeepCopy()
	changed := meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
		Type:   "RebootRequired",
		Status: status,
		Reason: reason,
	})
	if changed {
		return r.Status().Patch(ctx, serverBase, client.MergeFrom(server))

	}
	return nil
}

func biosSettingsUpdateRequired(desired, observed map[string]string) bool {
	for k, v := range desired {
		attr, ok := observed[k]
		if !ok {
			return false
		}
		if attr != v {
			return false
		}
	}
	return true
}
