// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const BMCFinalizer = "metal.ironcore.dev/bmc"

// BMCReconciler reconciles a BMC object
type BMCReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	Insecure             bool
	BMCFailureResetDelay time.Duration
	BMCPollingOptions    bmc.Options
	LastBMCFailure       map[types.NamespacedName]time.Time
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BMCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, req.NamespacedName, bmcObj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, bmcObj)
}

func (r *BMCReconciler) reconcileExists(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	if !bmcObj.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, bmcObj)
	}
	return r.reconcile(ctx, log, bmcObj)
}

func (r *BMCReconciler) delete(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	log.V(1).Info("Deleting BMC")
	if bmcObj.Spec.BMCSettingRef != nil {
		bmcSettings := &metalv1alpha1.BMCSettings{}
		if err := r.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.BMCSettingRef.Name}, bmcSettings); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get BMCSettings for BMC: %w", err)
		}
		if err := r.Delete(ctx, bmcSettings); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete referred BMCSettings. %w", err)
		}
	}

	if _, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, bmcObj, BMCFinalizer); err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Deleted BMC")
	return ctrl.Result{}, nil
}

func (r *BMCReconciler) reconcile(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) (ctrl.Result, error) {
	log.V(1).Info("Reconciling BMC")
	if shouldIgnoreReconciliation(bmcObj) {
		log.V(1).Info("Skipped BMC reconciliation")
		return ctrl.Result{}, nil
	}
	if err := r.handleAnnotionOperations(ctx, log, bmcObj); err != nil {
		return ctrl.Result{}, err
	}
	if bmcObj.Status.State == metalv1alpha1.BMCStateResetting {
		running, err := r.checkResetJobStatus(ctx, log, bmcObj)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("BMC reset job status: %w", err)
		}
		if running {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCPollingOptions)
	if err != nil {
		return r.handleBMCConnectionFailure(ctx, log, bmcObj, err)
	}
	defer bmcClient.Logout()
	// Reset the failure timestamp on successful connection
	delete(r.LastBMCFailure, types.NamespacedName{Name: bmcObj.Name})
	if err := r.updateBMCStatusDetails(ctx, log, bmcClient, bmcObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update BMC status: %w", err)
	}
	log.V(1).Info("Updated BMC status")

	if err := r.discoverServers(ctx, log, bmcClient, bmcObj); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to discover servers: %w", err)
	}
	log.V(1).Info("Discovered servers")

	log.V(1).Info("Reconciled BMC")
	// keep checking bmc status every 5 minutes
	return ctrl.Result{
		RequeueAfter: 5 * time.Minute,
	}, nil
}

func (r *BMCReconciler) updateBMCStatusDetails(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, bmcObj *metalv1alpha1.BMC) error {
	var (
		ip         metalv1alpha1.IP
		macAddress string
	)
	if bmcObj.Spec.EndpointRef != nil {
		endpoint := &metalv1alpha1.Endpoint{}
		if err := r.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.EndpointRef.Name}, endpoint); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get Endpoints for BMC: %w", err)
		}
		ip = endpoint.Spec.IP
		macAddress = endpoint.Spec.MACAddress
		log.V(1).Info("Got Endpoints for BMC", "Endpoints", endpoint.Name)
	}

	if bmcObj.Spec.Endpoint != nil {
		ip = bmcObj.Spec.Endpoint.IP
		macAddress = bmcObj.Spec.Endpoint.MACAddress
	}

	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.IP = ip
	bmcObj.Status.MACAddress = macAddress
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to patch IP and MAC address status: %w", err)
	}

	manager, err := bmcClient.GetManager(bmcObj.Spec.BMCUUID)
	if err != nil {
		return fmt.Errorf("failed to get manager details for BMC %s: %w", bmcObj.Name, err)
	}

	// parse time to metav1.Time: ISO 8601 format
	lastResetTime := &metav1.Time{}
	if manager.LastResetTime != "" {
		t, err := time.Parse(time.RFC3339, manager.LastResetTime)
		if err == nil {
			lastResetTime = &metav1.Time{Time: t}
		}
	}
	if manager != nil {
		bmcBase := bmcObj.DeepCopy()
		bmcObj.Status.Manufacturer = manager.Manufacturer
		bmcObj.Status.State = metalv1alpha1.BMCState(string(manager.Status.State))
		bmcObj.Status.PowerState = metalv1alpha1.BMCPowerState(string(manager.PowerState))
		bmcObj.Status.FirmwareVersion = manager.FirmwareVersion
		bmcObj.Status.SerialNumber = manager.SerialNumber
		bmcObj.Status.SKU = manager.PartNumber
		bmcObj.Status.Model = manager.Model
		bmcObj.Status.LastResetTime = lastResetTime
		if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
			return fmt.Errorf("failed to patch manager details for BMC %s: %w", bmcObj.Name, err)
		}
	} else {
		log.V(1).Info("Manager details not available for BMC", "BMC", bmcObj.Name)
	}

	return nil
}

func (r *BMCReconciler) discoverServers(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, bmcObj *metalv1alpha1.BMC) error {
	servers, err := bmcClient.GetSystems(ctx)
	if err != nil {
		return fmt.Errorf("failed to get servers from BMC %s: %w", bmcObj.Name, err)
	}
	var errs []error
	for i, s := range servers {
		server := &metalv1alpha1.Server{}
		server.Name = bmcutils.GetServerNameFromBMCandIndex(i, bmcObj)
		opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, server, func() error {
			metautils.SetLabels(server, bmcObj.Labels)
			server.Spec.UUID = strings.ToLower(s.UUID)
			server.Spec.SystemUUID = strings.ToLower(s.UUID)
			server.Spec.SystemURI = s.URI
			server.Spec.BMCRef = &v1.LocalObjectReference{Name: bmcObj.Name}
			return controllerutil.SetControllerReference(bmcObj, server, r.Scheme)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create or patch server %s: %w", server.Name, err))
			continue
		}
		log.V(1).Info("Created or patched Server", "Server", server.Name, "Operation", opResult)
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred during server discovery: %v", errs)
	}

	return nil
}

func (r *BMCReconciler) handleAnnotionOperations(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) error {
	operation, ok := bmcObj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !ok {
		return nil
	}
	switch operation {
	case metalv1alpha1.OperationAnnotationForceReset:
		// check if last reset was less than 1 minutes ago
		if !bmcObj.Status.LastResetTime.IsZero() && time.Since(bmcObj.Status.LastResetTime.Time) < 1*time.Minute {
			return fmt.Errorf("bmc reset done less than 1 minutes ago, not forcing another reset")
		}
		if bmcObj.Status.State == metalv1alpha1.BMCStateResetting {
			// BMC is resetting, give it some time
			return nil
		}
		log.V(1).Info("Handling operation", "Operation", operation)
		if err := r.resetBMC(ctx, log, bmcObj); err != nil {
			return fmt.Errorf("failed to reset BMC: %w", err)
		}
	}
	bmcBase := bmcObj.DeepCopy()
	metautils.DeleteAnnotation(bmcObj, metalv1alpha1.OperationAnnotation)
	if err := r.Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to remove operation annotation: %w", err)
	}
	log.V(1).Info("Removed operation annotation", "Operation", operation)

	return nil
}

func (r *BMCReconciler) handleBMCConnectionFailure(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC, err error) (ctrl.Result, error) {
	if bmcObj.Status.State == metalv1alpha1.BMCStateResetting {
		// BMC is resetting, give it some time
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if r.BMCFailureResetDelay == 0 {
		// no reset delay configured, just return the error
		return ctrl.Result{}, err
	}
	if bmcObj.Status.LastResetTime != nil && time.Since(bmcObj.Status.LastResetTime.Time) < 1*time.Minute {
		// recently reset, give it some time
		return ctrl.Result{}, fmt.Errorf("bmc reset done less than %v ago, waiting before attempting another reset", r.BMCFailureResetDelay)
	}
	if httpErr, ok := err.(*common.Error); ok {
		// only handle 5xx errors
		if httpErr.HTTPReturnedStatusCode < 500 || httpErr.HTTPReturnedStatusCode >= 600 {
			return ctrl.Result{}, fmt.Errorf("failed to get BMC client: %w", err)
		}
	} else {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client: %w", err)
	}
	lastFailure, ok := r.LastBMCFailure[types.NamespacedName{Name: bmcObj.Name}]
	if !ok {
		// First failure, record the timestamp
		r.LastBMCFailure[types.NamespacedName{Name: bmcObj.Name}] = time.Now()
	}
	if ok && time.Since(lastFailure) > r.BMCFailureResetDelay {
		log.Error(err, "BMC connection failure has persisted for more than %v", r.BMCFailureResetDelay)
		err := r.resetBMC(ctx, log, bmcObj)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reset job: %w", err)
		}
		log.Info("executing BMC reset after persistent connection failures")
		delete(r.LastBMCFailure, types.NamespacedName{Name: bmcObj.Name})
		// reset in progress, requeue after 5 minutes to check status
		return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
	}
	return ctrl.Result{}, err
}

func (r *BMCReconciler) resetBMC(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) error {
	bmcBase := bmcObj.DeepCopy()
	bmcObj.Status.State = metalv1alpha1.BMCStateResetting
	bmcObj.Status.LastResetTime = &metav1.Time{Time: time.Now()}
	if err := r.Status().Patch(ctx, bmcObj, client.MergeFrom(bmcBase)); err != nil {
		return fmt.Errorf("failed to patch IP and MAC address status: %w", err)
	}
	running, err := r.checkResetJobStatus(ctx, log, bmcObj)
	if err != nil {
		return fmt.Errorf("BMC reset job status: %w", err)
	}
	if running {
		// job is still running, requeue
		return fmt.Errorf("BMC reset job is still running")
	}
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCPollingOptions)
	if bmcClient != nil {
		if err := bmcClient.ResetManager(ctx, bmcObj.Spec.BMCUUID, redfish.GracefulRestartResetType); err == nil {
			log.Info("Successfully reset BMC via Redfish", "BMC", bmcObj.Name)
			return nil
		}
		log.Error(err, "failed to reset BMC via Redfish, falling back to k8s job", "BMC", bmcObj.Name)
	}
	if httpErr, ok := err.(*common.Error); ok {
		// only handle 5xx errors
		if httpErr.HTTPReturnedStatusCode < 500 || httpErr.HTTPReturnedStatusCode >= 600 {
			return fmt.Errorf("failed to get BMC client: %w", err)
		}
	} else {
		return fmt.Errorf("failed to get BMC client: %w", err)
	}
	// If we reach here, Redfish reset failed or was not possible
	job := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("bmc-reset-%s-", bmcObj.Name), Namespace: bmcObj.Namespace}, job); err == nil {
		// job already exists
		log.V(1).Info("BMC reset job already exists", "Job", job.Name)
		if job.Status.Active > 0 {
			log.V(1).Info("BMC reset job is already running", "Job", job.Name)
			return nil
		}
		// delete old job
		if err := r.Delete(ctx, job); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete old BMC reset job: %w", err)
		}
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get BMC reset job: %w", err)
	}
	// create new k8s job which resets the BMC
	newJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("bmc-reset-%s-", bmcObj.Name),
			Namespace: bmcObj.Namespace,
			Labels:    bmcObj.Labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: func(i int32) *int32 { return &i }(3),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						{
							Name:  "bmc-reset",
							Image: "ironcoredev/bmc:latest",
							Command: []string{
								"bmc",
								"reset",
								bmcObj.Name,
								"--model", bmcObj.Status.Model,
								"--bmc_address", bmcObj.Spec.Endpoint.IP.String(),
								"--bmc_manufacturer", bmcObj.Status.Manufacturer,
								"--timeout", "5m",
							},
							ImagePullPolicy: v1.PullIfNotPresent,
							Env: []v1.EnvVar{
								{
									Name: metalv1alpha1.BMCSecretPasswordKeyName,
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											Key: metalv1alpha1.BMCSecretPasswordKeyName,
											LocalObjectReference: v1.LocalObjectReference{
												Name: bmcObj.Spec.BMCSecretRef.Name,
											},
										},
									},
								},
								{
									Name: "Username",
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											Key: metalv1alpha1.BMCSecretPasswordKeyName,
											LocalObjectReference: v1.LocalObjectReference{
												Name: bmcObj.Spec.BMCSecretRef.Name,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, newJob, func() error {
		return controllerutil.SetControllerReference(bmcObj, newJob, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to create or patch BMC reset job: %w", err)
	}
	log.Info("Created or patched BMC reset job", "Job", job.Name, "Operation", opResult)
	return nil
}

func (r *BMCReconciler) checkResetJobStatus(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC) (bool, error) {
	// some bmcs need time to start the reset process
	if !bmcObj.Status.LastResetTime.IsZero() && time.Since(bmcObj.Status.LastResetTime.Time) < 5*time.Second {
		return true, nil
	}
	job := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("bmc-reset-%s-", bmcObj.Name), Namespace: bmcObj.Namespace}, job); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get BMC reset job: %w", err)
	}
	if job.Status.Failed > 0 && job.Status.Active == 0 {
		return false, fmt.Errorf("BMC reset job failed")
	}
	if job.Status.Active > 0 {
		return true, nil
	}
	if job.Status.Succeeded > 0 {
		// delete job after successful completion
		if err := r.Delete(ctx, job); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to delete completed BMC reset job: %w", err)
		}
		log.Info("Deleted completed BMC reset job", "Job", job.Name)
	}
	return false, nil
}

func (r *BMCReconciler) enqueueBMCByEndpoint(ctx context.Context, obj client.Object) []ctrl.Request {
	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{Name: obj.(*metalv1alpha1.Endpoint).Name},
		},
	}
}

func (r *BMCReconciler) enqueueBMCByBMCSecret(ctx context.Context, obj client.Object) []ctrl.Request {
	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{Name: obj.(*metalv1alpha1.BMCSecret).Name},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.LastBMCFailure = make(map[types.NamespacedName]time.Time)

	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMC{}).
		Owns(&metalv1alpha1.Server{}).
		Owns(&batchv1.Job{}).
		Watches(&metalv1alpha1.Endpoint{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByEndpoint)).
		Watches(&metalv1alpha1.BMCSecret{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBMCByBMCSecret)).
		Complete(r)
}
