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
func (r *ServerMaintenanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	maintenance := &metalv1alpha1.ServerMaintenance{}
	if err := r.Get(ctx, req.NamespacedName, maintenance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, log, maintenance)
}

func (r *ServerMaintenanceReconciler) reconcileExists(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	if !maintenance.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, maintenance)
	}
	return r.reconcile(ctx, log, maintenance)
}

func (r *ServerMaintenanceReconciler) reconcile(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerMaintenance")

	if shouldIgnoreReconciliation(maintenance) {
		log.V(1).Info("Skipped ServerMaintenance reconciliation")
		return ctrl.Result{}, nil
	}

	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: maintenance.Spec.ServerRef.Name}, server); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Server: %w", err)
	}
	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, maintenance, ServerMaintenanceFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	// set the ServerMaintenance state to pending if it is not set
	if maintenance.Status.State == "" {
		if modified, err := r.patchMaintenanceState(ctx, maintenance, metalv1alpha1.ServerMaintenanceStatePending); err != nil || modified {
			return ctrl.Result{}, err
		}
	}
	return r.ensureServerMaintenanceStateTransition(ctx, log, maintenance)
}

func (r *ServerMaintenanceReconciler) ensureServerMaintenanceStateTransition(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	switch maintenance.Status.State {
	case metalv1alpha1.ServerMaintenanceStatePending:
		return r.handlePendingState(ctx, log, maintenance)
	case metalv1alpha1.ServerMaintenanceStateInMaintenance:
		return r.handleInMaintenanceState(ctx, log, maintenance)
	case metalv1alpha1.ServerMaintenanceStateFailed:
		return r.handleFailedState(log, maintenance)
	default:
		log.V(1).Info("Unknown ServerMaintenance state, skipping reconciliation", "State", maintenance.Status.State)
		return ctrl.Result{}, nil
	}
}

func (r *ServerMaintenanceReconciler) handlePendingState(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance) (result ctrl.Result, err error) {
	if maintenance.Spec.ServerRef == nil {
		return ctrl.Result{}, fmt.Errorf("server reference is nil")
	}

	server, err := GetServerByName(ctx, r.Client, maintenance.Spec.ServerRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	if server.Spec.ServerMaintenanceRef != nil {
		if server.Spec.ServerMaintenanceRef.UID != maintenance.UID {
			log.V(1).Info("Server is already in maintenance", "Server", server.Name)
			return ctrl.Result{}, nil
		}
	}

	if server.Spec.ServerClaimRef == nil {
		log.V(1).Info("Server has no ServerClaim, move to maintenance state right away", "Server", server.Name)
		if err = r.updateServerRef(ctx, log, maintenance, server); err != nil {
			return ctrl.Result{}, err
		}
		if modified, err := r.patchMaintenanceState(ctx, maintenance, metalv1alpha1.ServerMaintenanceStateInMaintenance); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	serverClaim := &metalv1alpha1.ServerClaim{}
	if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.ServerClaimRef.Name, Namespace: server.Spec.ServerClaimRef.Namespace}, serverClaim); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get ServerClaim: %w", err)
		}
		log.V(1).Info("ServerClaim gone")
		return ctrl.Result{}, nil
	}
	annotations := map[string]string{
		metalv1alpha1.ServerMaintenanceNeededLabelKey: "true",
	}
	if maintenance.Annotations[metalv1alpha1.ServerMaintenanceReasonAnnotationKey] != "" {
		annotations[metalv1alpha1.ServerMaintenanceReasonAnnotationKey] = maintenance.Annotations[metalv1alpha1.ServerMaintenanceReasonAnnotationKey]
	}
	if err := r.patchServerClaimAnnotations(ctx, serverClaim, annotations); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch server claim annotations: %w", err)
	}
	log.V(1).Info("Patched ServerClaim annotations", "ServerClaim", client.ObjectKeyFromObject(serverClaim))

	if maintenance.Spec.Policy == metalv1alpha1.ServerMaintenancePolicyOwnerApproval {
		annotations := serverClaim.GetAnnotations()
		if _, ok := annotations[metalv1alpha1.ServerMaintenanceApprovalKey]; !ok {
			log.V(1).Info("Server not approved for maintenance, waiting for approval", "Server", server.Name)
			return ctrl.Result{}, nil
		}
		log.V(1).Info("Server approved for maintenance", "Server", server.Name)
		if err = r.updateServerRef(ctx, log, maintenance, server); err != nil {
			return ctrl.Result{}, err
		}
		if modified, err := r.patchMaintenanceState(ctx, maintenance, metalv1alpha1.ServerMaintenanceStateInMaintenance); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	if maintenance.Spec.Policy == metalv1alpha1.ServerMaintenancePolicyEnforced {
		log.V(1).Info("Enforcing maintenance", "Server", server.Name)
		if err := r.updateServerRef(ctx, log, maintenance, server); err != nil {
			return ctrl.Result{}, err
		}
		if modified, err := r.patchMaintenanceState(ctx, maintenance, metalv1alpha1.ServerMaintenanceStateInMaintenance); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	log.V(1).Info("Reconciled ServerMaintenance in Pending state")
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) handleInMaintenanceState(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	if maintenance.Spec.ServerRef == nil {
		return ctrl.Result{}, fmt.Errorf("server reference is nil")
	}

	server, err := GetServerByName(ctx, r.Client, maintenance.Spec.ServerRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	config, err := r.applyServerBootConfiguration(ctx, log, maintenance, server)
	if err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Applied ServerBootConfiguration for Server")

	if config == nil {
		if err := r.setAndPatchServerPowerState(ctx, server, maintenance); err != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("Patched server power state", "Server", server.Name, "Power", maintenance.Spec.ServerPower)
		return ctrl.Result{}, nil
	}

	if config.Status.State == metalv1alpha1.ServerBootConfigurationStatePending || config.Status.State == "" {
		log.V(1).Info("ServerBootConfiguration is in Pending state", "Server", server.Name)
		return ctrl.Result{}, nil
	}

	if config.Status.State == metalv1alpha1.ServerBootConfigurationStateError {
		if modified, err := r.patchMaintenanceState(ctx, maintenance, metalv1alpha1.ServerMaintenanceStateFailed); err != nil || modified {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if config.Status.State == metalv1alpha1.ServerBootConfigurationStateReady {
		log.V(1).Info("Server maintenance boot configuration is ready", "Server", server.Name)
		if err := r.setAndPatchServerPowerState(ctx, server, maintenance); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.V(1).Info("Reconciled ServerMaintenance in InMaintenance state")
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
	server.Spec.MaintenanceBootConfigurationRef = &metalv1alpha1.ObjectReference{
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerBootConfiguration",
		Namespace:  config.Namespace,
		Name:       config.Name,
		UID:        config.UID,
	}
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return config, fmt.Errorf("failed to patch server maintenance boot configuration ref: %w", err)
	}
	return config, nil
}

func (r *ServerMaintenanceReconciler) setAndPatchServerPowerState(ctx context.Context, server *metalv1alpha1.Server, maintenance *metalv1alpha1.ServerMaintenance) error {
	serverBase := server.DeepCopy()
	server.Spec.Power = maintenance.Spec.ServerPower
	return r.Patch(ctx, server, client.MergeFrom(serverBase))
}

func (r *ServerMaintenanceReconciler) updateServerRef(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance, server *metalv1alpha1.Server) error {
	if server.Spec.ServerMaintenanceRef != nil {
		log.V(1).Info("Server is already in Maintenance", "Server", server.Name, "Maintenance", server.Spec.ServerMaintenanceRef.Name)
		return nil
	}
	server.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{
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
	log.V(1).Info("Updated ServerMaintenance reference on Server", "Server", server.Name)

	return nil
}

func (r *ServerMaintenanceReconciler) handleFailedState(log logr.Logger, _ *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	log.V(1).Info("Reconciled ServerMaintenance in Failed state")
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) delete(ctx context.Context, log logr.Logger, maintenance *metalv1alpha1.ServerMaintenance) (ctrl.Result, error) {
	log.V(1).Info("Deleting ServerMaintenance")
	if maintenance.Spec.ServerRef == nil {
		return ctrl.Result{}, nil
	}
	server, err := GetServerByName(ctx, r.Client, maintenance.Spec.ServerRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.cleanup(ctx, log, server); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Removed dependencies")

	log.V(1).Info("Ensuring that the finalizer is removed")
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, maintenance, ServerMaintenanceFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured that the finalizer is removed")

	log.V(1).Info("Deleted ServerMaintenance")
	return ctrl.Result{}, nil
}

func (r *ServerMaintenanceReconciler) cleanup(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server) error {
	if server == nil {
		return nil
	}

	if server.Spec.ServerMaintenanceRef != nil {
		if err := r.removeMaintenanceRefFromServer(ctx, server); err != nil {
			log.Error(err, "failed to remove ServerMaintenance ref from server")
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
				return fmt.Errorf("failed to delete ServerBootConfiguration: %w", err)
			}
			log.V(1).Info("ServerBootConfiguration already deleted", "Config", client.ObjectKeyFromObject(config))
		}
		if err := r.removeBootConfigRefFromServer(ctx, config, server); err != nil {
			return fmt.Errorf("failed to remove ServerMaintenance boot config ref from Server: %w", err)
		}
		log.V(1).Info("Removed ServerMaintenance boot configuration ref from Server", "Server", server.Name)
	}

	if server.Spec.ServerClaimRef == nil {
		return nil
	}
	serverClaim := &metalv1alpha1.ServerClaim{}
	if err := r.Get(ctx, client.ObjectKey{Name: server.Spec.ServerClaimRef.Name, Namespace: server.Spec.ServerClaimRef.Namespace}, serverClaim); err != nil {
		return fmt.Errorf("failed to get ServerClaim: %w", err)
	}
	serverClaimBase := serverClaim.DeepCopy()
	metautils.DeleteAnnotations(serverClaim, []string{
		metalv1alpha1.ServerMaintenanceApprovalKey,
		metalv1alpha1.ServerMaintenanceNeededLabelKey,
		metalv1alpha1.ServerMaintenanceReasonAnnotationKey,
	})
	if err := r.Patch(ctx, serverClaim, client.MergeFrom(serverClaimBase)); err != nil {
		return fmt.Errorf("failed to patch ServerClaim annotations: %w", err)
	}
	return nil
}

func (r *ServerMaintenanceReconciler) removeBootConfigRefFromServer(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration, server *metalv1alpha1.Server) error {
	if ref := server.Spec.MaintenanceBootConfigurationRef; ref == nil || (ref.Name != config.Name && ref.Namespace != config.Namespace) {
		return nil
	}
	serverBase := server.DeepCopy()
	server.Spec.MaintenanceBootConfigurationRef = nil
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
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

func (r *ServerMaintenanceReconciler) patchMaintenanceState(ctx context.Context, maintenance *metalv1alpha1.ServerMaintenance, state metalv1alpha1.ServerMaintenanceState) (bool, error) {
	if maintenance == nil {
		return false, fmt.Errorf("ServerMaintenance is nil")
	}
	if maintenance.Status.State == state {
		return false, nil
	}
	maintenanceBase := maintenance.DeepCopy()
	maintenance.Status.State = state
	if err := r.Status().Patch(ctx, maintenance, client.MergeFrom(maintenanceBase)); err != nil {
		return false, fmt.Errorf("failed to patch ServerMaintenance status: %w", err)
	}
	return true, nil
}

func (r *ServerMaintenanceReconciler) patchServerClaimAnnotations(ctx context.Context, claim *metalv1alpha1.ServerClaim, set map[string]string) error {
	if claim == nil {
		return fmt.Errorf("ServerClaim is nil")
	}
	claimBase := claim.DeepCopy()
	metautils.SetAnnotations(claim, set)
	if err := r.Patch(ctx, claim, client.MergeFrom(claimBase)); err != nil {
		return fmt.Errorf("failed to patch ServerClaim annotations: %w", err)
	}
	return nil
}

func (r *ServerMaintenanceReconciler) enqueueMaintenanceByServerRefs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)
		server, ok := object.(*metalv1alpha1.Server)
		if !ok {
			log.Error(nil, "expected object to be a Server", "object", object)
			return nil
		}

		var req []reconcile.Request
		processedNames := make(map[string]struct{})

		if server.Status.State == metalv1alpha1.ServerStateInitial {
			return nil
		}

		if server.Spec.ServerMaintenanceRef != nil {
			name := server.Spec.ServerMaintenanceRef.Name
			if _, found := processedNames[name]; !found {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: server.Namespace, Name: name},
				})
				processedNames[name] = struct{}{}
			}
		}

		maintenanceList := &metalv1alpha1.ServerMaintenanceList{}
		if err := r.List(ctx, maintenanceList, client.MatchingFields{serverRefField: server.Name}); err != nil {
			log.Error(err, "failed to list ServerMaintenances")
		} else {
			for _, maintenance := range maintenanceList.Items {
				name := maintenance.Name
				if _, found := processedNames[name]; !found {
					req = append(req, reconcile.Request{
						NamespacedName: types.NamespacedName{Namespace: maintenance.Namespace, Name: name},
					})
					processedNames[name] = struct{}{}
				}
			}
		}
		return req
	})
}

func (r *ServerMaintenanceReconciler) enqueueMaintenanceByClaimRefs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)
		claim, ok := object.(*metalv1alpha1.ServerClaim)
		if !ok {
			log.Error(nil, "expected object to be a ServerClaim", "object", object)
			return nil
		}

		annotations := claim.GetAnnotations()
		if _, ok := annotations[metalv1alpha1.ServerMaintenanceNeededLabelKey]; !ok {
			return nil
		}

		if claim.Spec.ServerRef == nil || claim.Spec.ServerRef.Name == "" {
			return nil
		}

		maintenanceList := &metalv1alpha1.ServerMaintenanceList{}
		if err := r.List(ctx, maintenanceList, client.MatchingFields{serverRefField: claim.Spec.ServerRef.Name}); err != nil {
			log.Error(err, "failed to list ServerMaintenances")
			return nil
		}

		var req []reconcile.Request
		for _, maintenance := range maintenanceList.Items {
			req = append(req, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: maintenance.Namespace,
					Name:      maintenance.Name,
				},
			})
		}

		return req
	})
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
