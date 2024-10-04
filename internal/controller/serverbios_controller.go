// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const serverBIOSFinalizer = "metal.ironcore.dev/serverbios"

// ServerBIOSReconciler reconciles a ServerBIOS object
type ServerBIOSReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Insecure bool
	// todo: need to decide how to provide jobs' configuration to controller
	JobNamespace          string
	JobImage              string
	JobServiceAccountName string
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
// - there is active job related to the object already running;
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

	// if related job is already running - stop reconciliation
	if jobInProgress, err := r.jobInProgress(ctx, log, serverBIOS.Status.RunningJob); err != nil || jobInProgress {
		log.V(1).Info("related job is already running")
		return ctrl.Result{}, err
	}

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
//  1. Ensure finalizer is set on the object
//  2. Ensure info about current BIOS version and settings is not outdated, otherwise:
//     2.1. Invoke scan job and set reference to this job in status
//     2.2. Wait until object will be updated by job runner with up-to-date info, empty reference to the job and the
//     last scan time
//  3. Ensure referred server is in Available state
//  4. Ensure desired and current BIOS versions match, otherwise:
//     4.1. Invoke BIOS version update job and set reference to this job in status
//     4.2. Wait until object will be updated by job runner with up-to-date info and empty reference to the job
//  5. Ensure desired and current BIOS settings match, otherwise:
//     5.1. Invoke BIOS settings update job and set reference to this job in status
//     5.2. Wait until object will be updated by job runner with up-to-date info and empty reference to the job
func (r *ServerBIOSReconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, serverBIOS, serverBIOSFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("ensured finalizer has been added")

	// if scanned data outdated - run scan
	if time.Since(serverBIOS.Status.LastScanTime.Time) > time.Duration(serverBIOS.Spec.ScanPeriodMinutes)*time.Minute {
		return r.reconcileScan(ctx, log, serverBIOS)
	}
	return r.reconcileUpdate(ctx, log, serverBIOS)
}

func (r *ServerBIOSReconciler) reconcileScan(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	log.V(1).Info("invoking scan job")
	jobReference, err := r.createJob(ctx, log, serverBIOS, metalv1alpha1.ScanBIOSVersionJobType)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.patchJobReference(ctx, log, serverBIOS, metalv1alpha1.RunningJobRef{
		Type:   metalv1alpha1.ScanBIOSVersionJobType,
		JobRef: jobReference,
	})
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
	// if referred server is not in Available state - stop reconciliation
	if server.Status.State != metalv1alpha1.ServerStateAvailable {
		// todo: maybe return ctrl.Result{RequeueAfter: ?}
		return ctrl.Result{}, nil
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
	jobReference, err := r.createJob(ctx, log, serverBIOS, metalv1alpha1.UpdateBIOSVersionJobType)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.patchJobReference(ctx, log, serverBIOS, metalv1alpha1.RunningJobRef{
		Type:   metalv1alpha1.UpdateBIOSVersionJobType,
		JobRef: jobReference,
	})
}

func (r *ServerBIOSReconciler) reconcileSettingsUpdate(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
) (ctrl.Result, error) {
	log.V(1).Info("invoking settings update job")
	jobReference, err := r.createJob(ctx, log, serverBIOS, metalv1alpha1.ApplyBIOSSettingsJobType)
	if err != nil {
		return ctrl.Result{}, err
	}
	return r.patchJobReference(ctx, log, serverBIOS, metalv1alpha1.RunningJobRef{
		Type:   metalv1alpha1.ApplyBIOSSettingsJobType,
		JobRef: jobReference,
	})
}

func (r *ServerBIOSReconciler) createJob(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	jobType metalv1alpha1.JobType,
) (corev1.ObjectReference, error) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", serverBIOS.Name),
			Namespace:    r.JobNamespace,
		},
		Spec: batchv1.JobSpec{
			Completions: nil,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"metal.ironcore.dev/serverbios": serverBIOS.Name,
					"metal.ironcore.dev/jobtype":    string(jobType),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"metal.ironcore.dev/serverbios": serverBIOS.Name,
						"metal.ironcore.dev/jobtype":    string(jobType),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: r.JobImage,
							Env: []corev1.EnvVar{
								{
									Name:  "JOB_TYPE",
									Value: string(jobType),
								},
								{
									Name:  "SERVER_BIOS_REF",
									Value: serverBIOS.Name,
								},
								{
									Name:  "INSECURE",
									Value: strconv.FormatBool(r.Insecure),
								},
							},
						},
					},
					ServiceAccountName:           r.JobServiceAccountName,
					AutomountServiceAccountToken: ptr.To(true),
					RestartPolicy:                corev1.RestartPolicyNever,
				},
			},
		},
	}
	if err := r.Create(ctx, job); err != nil {
		log.Error(err, "failed to create job")
		return corev1.ObjectReference{}, err
	}
	reference := corev1.ObjectReference{
		Kind:       "Job",
		Namespace:  job.Namespace,
		Name:       job.Name,
		UID:        job.UID,
		APIVersion: "batch/v1",
	}
	return reference, nil
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

func (r *ServerBIOSReconciler) jobInProgress(
	ctx context.Context,
	log logr.Logger,
	jobReference metalv1alpha1.RunningJobRef,
) (bool, error) {
	if jobReference == (metalv1alpha1.RunningJobRef{}) {
		return false, nil
	}
	key := client.ObjectKey{Namespace: r.JobNamespace, Name: r.JobImage}
	job := batchv1.Job{}
	if err := r.Get(ctx, key, &job); err != nil {
		log.Error(err, "failed to get job")
		return false, err
	}
	log.V(1).Info("active job found")
	return job.Status.Active > 0, nil
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

func (r *ServerBIOSReconciler) patchJobReference(
	ctx context.Context,
	log logr.Logger,
	serverBIOS *metalv1alpha1.ServerBIOS,
	jobReference metalv1alpha1.RunningJobRef,
) (ctrl.Result, error) {
	var err error
	serverBIOSBase := serverBIOS.DeepCopy()
	serverBIOS.Status.RunningJob = jobReference
	if err = r.Patch(ctx, serverBIOSBase, client.MergeFrom(serverBIOS)); err != nil {
		log.Error(err, "failed to patch server BIOS")
	}
	return ctrl.Result{}, err
}
