// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ServerMaintenanceFinalizer is the finalizer for the ServerMaintenance resource.
	ServerMaintenanceFinalizer = "metal.ironcore.dev/servermaintenance"
)

// ServerMaintenanceReconciler reconciles a ServerMaintenance object
type ServerMaintenanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServerMaintenance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *ServerMaintenanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	serverMaintenance := &metalv1alpha1.ServerMaintenance{}
	if err := r.Get(ctx, req.NamespacedName, serverMaintenance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, log, serverMaintenance)
}

func (r *ServerMaintenanceReconciler) reconcileExists(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	if !serverMaintenance.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, serverMaintenance)
	}
	return r.reconcile(ctx, log, serverMaintenance)
}

func (r *ServerMaintenanceReconciler) reconcile(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	_, err := r.getServerRef(ctx, serverMaintenance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Server not found, delete the ServerMaintenance CR
			log.Info("Server no longer found, deleting ServerMaintenance", "ServerMaintenance", serverMaintenance.Name)
			if err := r.Delete(ctx, serverMaintenance); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete ServerMaintenance: %w", err)
			}
			log.Info("Deleted ServerMaintenance", "ServerMaintenance", serverMaintenance.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get server: %w", err)
	}
	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, serverMaintenance, ServerMaintenanceFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	// set the servermaintenance state to pending if it is not set
	if serverMaintenance.Status.State == "" {
		if modified, err := r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStatePending); err != nil || modified {
			return ctrl.Result{}, err
		}
	}
	return r.ensureServerMaintenanceStateTransition(ctx, log, serverMaintenance)
}

func (r *ServerMaintenanceReconciler) ensureServerMaintenanceStateTransition(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	switch serverMaintenance.Status.State {
	case metalv1alpha1.ServerMaintenanceStatePending:
		return r.handlePendingState(ctx, log, serverMaintenance)
	case metalv1alpha1.ServerMaintenanceStateInMaintenance:
		return r.handleInMaintenanceState(ctx, log, serverMaintenance)
	case metalv1alpha1.ServerMaintenanceStateFailed:
		return r.handleFailedState(log, serverMaintenance)
	}
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) handlePendingState(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (result ctrl.Result, err error) {
	server, err := r.getServerRef(ctx, serverMaintenance)
	if err != nil {
		return ctrl.Result{}, err
	}
	if server.Spec.ServerMaintenanceRef != nil {
		if server.Spec.ServerMaintenanceRef.UID != serverMaintenance.UID {
			log.V(1).Info("Server is already in maintenance", "Server", server.Name, "Maintenance", server.Spec.ServerMaintenanceRef.Name)
			return ctrl.Result{}, nil
		}
	}
	if server.Spec.ServerClaimRef == nil {
		log.V(1).Info("Server has no claim, move to maintenance right away", "Server", server.Name)
		if err = r.updateServerRef(ctx, log, serverMaintenance, server); err != nil {
			log.Error(err, "failed to patch server maintenance ref")
			return ctrl.Result{}, err
		}
		if modified, err := r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStateInMaintenance); err != nil || modified {
			return ctrl.Result{}, err
		}
	}
	serverClaim := &metalv1alpha1.ServerClaim{}
	if err := r.Get(ctx,
		client.ObjectKey{
			Name:      server.Spec.ServerClaimRef.Name,
			Namespace: server.Spec.ServerClaimRef.Namespace,
		}, serverClaim); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get server claim: %w", err)
		}
		log.V(1).Info("ServerClaim gone")
		return ctrl.Result{}, nil
	}
	claimAnnotations := map[string]string{
		metalv1alpha1.ServerMaintenanceNeededLabelKey: "true",
	}
	if serverMaintenance.Annotations[metalv1alpha1.ServerMaintenanceReasonAnnotationKey] != "" {
		claimAnnotations[metalv1alpha1.ServerMaintenanceReasonAnnotationKey] = serverMaintenance.Annotations[metalv1alpha1.ServerMaintenanceReasonAnnotationKey]
	}
	if err := r.patchServerClaimAnnotation(ctx, log, serverClaim, claimAnnotations); err != nil {
		return ctrl.Result{}, err
	}
	if serverMaintenance.Spec.Policy == metalv1alpha1.ServerMaintenancePolicyOwnerApproval {
		claimAnnotations := serverClaim.GetAnnotations()
		if _, ok := claimAnnotations[metalv1alpha1.ServerMaintenanceApprovalKey]; !ok {
			log.V(1).Info("Server not approved for maintenance, waiting for approval", "Server", server.Name)
			return ctrl.Result{}, nil
		}
		log.V(1).Info("Server approved for maintenance", "Server", server.Name, "Maintenance", serverMaintenance.Name)
		if err = r.updateServerRef(ctx, log, serverMaintenance, server); err != nil {
			log.Error(err, "failed to patch server maintenance ref")
			return ctrl.Result{}, err
		}
		if modified, err := r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStateInMaintenance); err != nil || modified {
			return ctrl.Result{}, err
		}
	}
	if serverMaintenance.Spec.Policy == metalv1alpha1.ServerMaintenancePolicyEnforced {
		log.V(1).Info("Enforcing maintenance", "Server", server.Name, "Maintenance", serverMaintenance.Name)
		if err = r.updateServerRef(ctx, log, serverMaintenance, server); err != nil {
			log.Error(err, "failed to patch server maintenance ref")
			return ctrl.Result{}, err
		}
		if modified, err := r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStateInMaintenance); err != nil || modified {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) handleInMaintenanceState(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	server, err := r.getServerRef(ctx, serverMaintenance)
	if err != nil {
		return ctrl.Result{}, err
	}
	config, err := r.applyServerBootConfiguration(ctx, log, serverMaintenance, server)
	if err != nil {
		return ctrl.Result{}, err
	}
	if config == nil {
		if err := r.setAndPatchServerPowerState(ctx, log, server, serverMaintenance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if config.Status.State == metalv1alpha1.ServerBootConfigurationStatePending || config.Status.State == "" {
		log.V(1).Info("Server boot configuration is pending", "Server", server.Name)
		return ctrl.Result{}, nil
	}
	if config.Status.State == metalv1alpha1.ServerBootConfigurationStateError {
		if modified, err := r.patchMaintenanceState(ctx, serverMaintenance, metalv1alpha1.ServerMaintenanceStateFailed); err != nil || modified {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if config.Status.State == metalv1alpha1.ServerBootConfigurationStateReady {
		log.V(1).Info("Server maintenance boot configuration is ready", "Server", server.Name)
		if err := r.setAndPatchServerPowerState(ctx, log, server, serverMaintenance); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) applyServerBootConfiguration(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance, server *metalv1alpha1.Server) (*metalv1alpha1.ServerBootConfiguration, error) {
	if maintenance.Spec.ServerBootConfigurationTemplate == nil {
		log.V(1).Info("No ServerBootConfigurationTemplate specified")
		return nil, nil
	}

	log.V(1).Info("Creating/Patching server maintenance boot configuration", "Server", server.Name)
	config := &metalv1alpha1.ServerBootConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      maintenance.Name,
			Namespace: maintenance.Namespace,
		},
	}
	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, config, func() error {
		config.Spec = maintenance.Spec.ServerBootConfigurationTemplate.Spec
		return controllerutil.SetControllerReference(maintenance, config, r.Scheme)
	})
	if err != nil {
		return config, fmt.Errorf("failed to create server boot configuration: %w", err)
	}
	log.V(1).Info("Created or patched Config", "Config", config.Name, "Operation", opResult)
	serverBase := server.DeepCopy()
	server.Spec.MaintenanceBootConfigurationRef = &v1.ObjectReference{
		Namespace:  config.Namespace,
		Name:       config.Name,
		UID:        config.UID,
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerBootConfiguration",
	}
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return config, fmt.Errorf("failed to patch server maintenance boot configuration ref: %w", err)
	}
	return config, nil
}

func (r *ServerMaintenanceReconciler) setAndPatchServerPowerState(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server, maintenance *metalv1alpha1.ServerMaintenance) error {
	serverBase := server.DeepCopy()
	server.Spec.Power = maintenance.Spec.ServerPower
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch server power state: %w", err)
	}
	log.V(1).Info("Patched server power state", "Server", server.Name, "Power", server.Spec.Power)

	return nil
}

func (r *ServerMaintenanceReconciler) updateServerRef(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance, server *metalv1alpha1.Server) error {
	if server.Spec.ServerMaintenanceRef != nil {
		log.V(1).Info("Server is already in Maintenance", "Server", server.Name, "Maintenance", server.Spec.ServerMaintenanceRef.Name)
		return nil
	}
	server.Spec.ServerMaintenanceRef = &v1.ObjectReference{
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerMaintenance",
		Namespace:  maintenance.Namespace,
		Name:       maintenance.Name,
		UID:        maintenance.UID,
	}
	// use update to not overwrite ServerMaintenanceRef if another maintenance was quicker
	if err := r.Update(ctx, server); err != nil {
		return fmt.Errorf("failed to patch maintenance ref for server: %w", err)
	}
	log.V(1).Info("Updated ServerMaintenance reference on Server", "Server", server.Name, "ServerMaintenanceeRef", maintenance.Name)

	return nil
}

func (r *ServerMaintenanceReconciler) handleFailedState(log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	log.V(1).Info("ServerMaintenance failed", "ServerMaintenance", serverMaintenance.Name)
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) delete(ctx context.Context, log logr.Logger, serverMaintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	log.V(1).Info("Deleting ServerMaintenance", "ServerMaintenance", serverMaintenance.Name)
	if serverMaintenance.Spec.ServerRef == nil {
		return ctrl.Result{}, nil
	}
	server, err := r.getServerRef(ctx, serverMaintenance)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get server: %w", err)
		}
	}
	if server != nil {
		if err := r.cleanup(ctx, log, server); err != nil {
			return ctrl.Result{}, err
		}
	}
	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, serverMaintenance, ServerMaintenanceFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) getServerRef(ctx context.Context, serverMaintenance *metalv1alpha1.ServerMaintenance) (*metalv1alpha1.Server, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: serverMaintenance.Spec.ServerRef.Name}, server); err != nil {
		return nil, err
	}
	return server, nil
}

func (r *ServerMaintenanceReconciler) cleanup(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	if server != nil && server.Spec.ServerMaintenanceRef != nil {
		if err := r.removeMaintenanceRefFromServer(ctx, server); err != nil {
			log.Error(err, "failed to remove maintenance ref from server")
		}
	}
	if server.Spec.MaintenanceBootConfigurationRef != nil {
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      server.Spec.MaintenanceBootConfigurationRef.Name,
				Namespace: server.Spec.MaintenanceBootConfigurationRef.Namespace,
			},
		}
		if err := r.Delete(ctx, config); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete serverbootconfig: %w", err)
			}
		}
		if err := r.removeBootConfigRefFromServer(ctx, log, config, server); err != nil {
			return fmt.Errorf("failed to remove maintenance boot config ref from server: %w", err)
		}
	}

	if server.Spec.ServerClaimRef == nil {
		return nil
	}
	serverClaim := &metalv1alpha1.ServerClaim{}
	if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.ServerClaimRef.Name, Namespace: server.Spec.ServerClaimRef.Namespace}, serverClaim); err != nil {
		return fmt.Errorf("failed to get server claim: %w", err)
	}
	serverClaimBase := serverClaim.DeepCopy()
	metautils.DeleteAnnotations(serverClaim, []string{
		metalv1alpha1.ServerMaintenanceApprovalKey,
		metalv1alpha1.ServerMaintenanceNeededLabelKey,
		metalv1alpha1.ServerMaintenanceReasonAnnotationKey,
	})
	if err := r.Patch(ctx, serverClaim, client.MergeFrom(serverClaimBase)); err != nil {
		return fmt.Errorf("failed to patch server claim annotations: %w", err)
	}
	return nil
}

func (r *ServerMaintenanceReconciler) removeBootConfigRefFromServer(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration, server *metalv1alpha1.Server) error {
	if server == nil {
		return nil
	}
	if ref := server.Spec.MaintenanceBootConfigurationRef; ref == nil || (ref.Name != config.Name && ref.Namespace != config.Namespace) {
		return nil
	}
	serverBase := server.DeepCopy()
	server.Spec.MaintenanceBootConfigurationRef = nil
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	log.V(1).Info("Removed maintenance boot configuration ref from server", "Server", server.Name)
	return nil
}

func (r *ServerMaintenanceReconciler) removeMaintenanceRefFromServer(ctx context.Context, server *metalv1alpha1.Server) error {
	serverBase := server.DeepCopy()
	server.Spec.ServerMaintenanceRef = nil
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return fmt.Errorf("failed to patch claim ref for server: %w", err)
	}
	return nil
}

func (r *ServerMaintenanceReconciler) patchMaintenanceState(ctx context.Context, serverMaintenance *metalv1alpha1.ServerMaintenance, state metalv1alpha1.ServerMaintenanceState) (bool, error) {
	if serverMaintenance.Status.State == state {
		return false, nil
	}
	base := serverMaintenance.DeepCopy()
	serverMaintenance.Status.State = state
	if err := r.Status().Patch(ctx, serverMaintenance, client.MergeFrom(base)); err != nil {
		return false, fmt.Errorf("failed to patch serverMaintenance state: %w", err)
	}
	return true, nil
}

func (r *ServerMaintenanceReconciler) patchServerClaimAnnotation(ctx context.Context, log logr.Logger, serverClaim *metalv1alpha1.ServerClaim, set map[string]string) error {
	anno := serverClaim.GetAnnotations()
	change := false
	for k, v := range set {
		if anno[k] != v {
			change = true
			break
		}
	}
	if !change {
		return nil
	}
	metautils.SetAnnotations(serverClaim, set)
	if err := r.Update(ctx, serverClaim); err != nil {
		return fmt.Errorf("failed to update serverclaim annotations: %w", err)
	}
	log.V(1).Info("Updated server claim annotations", "ServerClaim", serverClaim.Name, "Annotations", set)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerMaintenanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerMaintenance{}).
		Owns(&metalv1alpha1.ServerBootConfiguration{}).
		Watches(&metalv1alpha1.Server{}, r.enqueueMaintenanceByServerRefs()).
		Watches(&metalv1alpha1.ServerClaim{}, r.enqueueMaintenanceByClaimRefs()).
		Complete(r)
}

func (r *ServerMaintenanceReconciler) enqueueMaintenanceByServerRefs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)
		server := object.(*metalv1alpha1.Server)
		var req []reconcile.Request

		maintenanceList := &metalv1alpha1.ServerMaintenanceList{}
		if err := r.List(ctx, maintenanceList); err != nil {
			log.Error(err, "failed to list host serverMaintenances")
			return nil
		}
		for _, maintenance := range maintenanceList.Items {
			if server.Spec.ServerMaintenanceRef != nil && maintenance.Name == server.Spec.ServerMaintenanceRef.Name {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: maintenance.Namespace, Name: maintenance.Name},
				})
				return req
			}
			if server.Spec.ServerMaintenanceRef == nil {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: maintenance.Namespace, Name: maintenance.Name},
				})
			}
		}
		return req
	})
}

func (r *ServerMaintenanceReconciler) enqueueMaintenanceByClaimRefs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)
		claim := object.(*metalv1alpha1.ServerClaim)
		var req []reconcile.Request
		annotations := claim.GetAnnotations()
		if _, ok := annotations[metalv1alpha1.ServerMaintenanceNeededLabelKey]; !ok {
			return req
		}

		maintenanceList := &metalv1alpha1.ServerMaintenanceList{}
		if err := r.List(ctx, maintenanceList); err != nil {
			log.Error(err, "failed to list host serverMaintenances")
			return nil
		}
		for _, maintenance := range maintenanceList.Items {
			if maintenance.Spec.ServerRef != nil && maintenance.Spec.ServerRef.Name == claim.Spec.ServerRef.Name {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: maintenance.Namespace, Name: maintenance.Name},
				})
				return req
			}
			if maintenance.Spec.ServerRef == nil {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: maintenance.Namespace, Name: maintenance.Name},
				})
				return req
			}
		}
		return req
	})
}
