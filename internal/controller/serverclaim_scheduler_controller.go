// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ServerClaimSchedulerReconciler schedules unbound ServerClaims onto matching Servers
// and drives the claim phase from Unbound to Bound once the binding is complete.
type ServerClaimSchedulerReconciler struct {
	client.Client
	APIReader               client.Reader
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servermaintenances,verbs=get;list;watch

// Reconcile moves a ServerClaim from unbound to bound by selecting a matching,
// claimable Server, claiming it and recording the server reference on the claim.
func (r *ServerClaimSchedulerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	claim := &metalv1alpha1.ServerClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.schedule(ctx, claim)
}

func (r *ServerClaimSchedulerReconciler) schedule(ctx context.Context, claim *metalv1alpha1.ServerClaim) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Scheduling server claim")

	if !claim.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	if shouldIgnoreReconciliation(claim) {
		log.V(1).Info("Skipped ServerClaim scheduling")
		return ctrl.Result{}, nil
	}

	// late state initialization, so unscheduled claims show up as Unbound
	if claim.Status.Phase == "" {
		if modified, err := patchServerClaimPhase(ctx, r.Client, claim, metalv1alpha1.PhaseUnbound); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	server, err := r.claimServer(ctx, claim)
	if err != nil {
		return ctrl.Result{}, err
	}
	if server == nil {
		log.V(1).Info("No server found for claim")
		return ctrl.Result{}, nil
	}

	if modified, err := r.patchServerRef(ctx, claim, server); err != nil || modified {
		return ctrl.Result{}, err
	}

	if modified, err := patchServerClaimPhase(ctx, r.Client, claim, metalv1alpha1.PhaseBound); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Bound Server to ServerClaim", "Server", server.Name, "ServerClaim", claim.Name)
	return ctrl.Result{}, nil
}

func (r *ServerClaimSchedulerReconciler) claimServer(ctx context.Context, claim *metalv1alpha1.ServerClaim) (*metalv1alpha1.Server, error) {
	log := ctrl.LoggerFrom(ctx)

	// An explicit server reference only allows that very server as a candidate.
	if claim.Spec.ServerRef != nil {
		server := &metalv1alpha1.Server{}
		if err := r.Get(ctx, client.ObjectKey{Name: claim.Spec.ServerRef.Name}, server); err != nil {
			return nil, client.IgnoreNotFound(err)
		}
		if ref := server.Spec.ServerClaimRef; ref != nil && ref.Name == claim.Name && ref.Namespace == claim.Namespace {
			return server, nil
		}
		selector, err := metav1.LabelSelectorAsSelector(claim.Spec.ServerSelector)
		if err != nil {
			return nil, err
		}
		if claim.Spec.ServerSelector != nil && !selector.Matches(labels.Set(server.Labels)) {
			log.V(1).Info("Specified server matches ServerRef but does not match label selector", "Server", server.Name, "Claim", claim.Name)
			return nil, nil
		}
		return r.tryClaimServer(ctx, claim, server)
	}

	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList); err != nil {
		return nil, err
	}

	// fetch previously claimed server if its present
	// keeping this separate for reason of clarity
	for _, server := range serverList.Items {
		// find previously claimed server
		if ref := server.Spec.ServerClaimRef; ref != nil {
			if ref.Name == claim.Name && ref.Namespace == claim.Namespace {
				// Re-fetch from the API server to confirm this claim still owns it.
				// The list above is cached and may be stale.
				fresh := server.DeepCopy()
				if err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(fresh), fresh); err != nil {
					return nil, client.IgnoreNotFound(err)
				}
				if ref := fresh.Spec.ServerClaimRef; ref == nil || ref.Name != claim.Name || ref.Namespace != claim.Namespace {
					return nil, nil
				}
				return fresh, nil
			}
		}
	}

	var selectedServer *metalv1alpha1.Server
	selector, err := metav1.LabelSelectorAsSelector(claim.Spec.ServerSelector)
	if err != nil {
		return nil, err
	}
	for _, server := range serverList.Items {
		if claim.Spec.ServerSelector != nil && !selector.Matches(labels.Set(server.Labels)) {
			log.V(1).Info("Specified server does not match label selector", "Server", server.Name, "Claim", claim.Name)
			continue
		}
		if r.isServerClaimable(ctx, &server, claim) {
			selectedServer = &server
			break
		}
	}
	if selectedServer == nil {
		return nil, nil
	}
	log.V(1).Info("Matching server found", "Server", selectedServer.Name)
	return r.tryClaimServer(ctx, claim, selectedServer)
}

func (r *ServerClaimSchedulerReconciler) tryClaimServer(ctx context.Context, claim *metalv1alpha1.ServerClaim, server *metalv1alpha1.Server) (*metalv1alpha1.Server, error) {
	// Re-fetch the selected server directly from the API server before claiming.
	// The server may come from the informer cache; a concurrent reconciler may have
	// already claimed or changed the server since then. Re-fetching here ensures
	// isServerClaimable and ensureObjectRefForServer act on consistent state.
	if err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(server), server); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if !r.isServerClaimable(ctx, server, claim) {
		return nil, nil
	}

	// ensureObjectRefForServer uses optimistic locking on the patch, so it will
	// not overwrite a claim that was set between our re-fetch and the write.
	if err := r.ensureObjectRefForServer(ctx, claim, server); err != nil {
		return nil, err
	}
	// If another reconciler won the optimistic-lock race, the ref will point to
	// their claim. Return an error to requeue with backoff so this claim retries.
	if ref := server.Spec.ServerClaimRef; ref == nil || ref.Name != claim.Name || ref.Namespace != claim.Namespace {
		return nil, fmt.Errorf("server %s was claimed concurrently, will retry", server.Name)
	}
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Ensured ObjectRef for Server", "Server", server.Name)
	return server, nil
}

func (r *ServerClaimSchedulerReconciler) ensureObjectRefForServer(ctx context.Context, claim *metalv1alpha1.ServerClaim, server *metalv1alpha1.Server) error {
	log := ctrl.LoggerFrom(ctx)

	if server.Spec.ServerClaimRef != nil {
		log.V(1).Info("Server already claimed", "Server", server.Name, "Claim", server.Spec.ServerClaimRef.Name)
		return nil
	}

	serverBase := server.DeepCopy()
	server.Spec.ServerClaimRef = &metalv1alpha1.ImmutableObjectReference{
		Namespace: claim.Namespace,
		Name:      claim.Name,
	}
	if err := r.Patch(ctx, server, client.MergeFromWithOptions(serverBase, client.MergeFromWithOptimisticLock{})); err != nil {
		return fmt.Errorf("failed to patch claim ref for server: %w", err)
	}
	log.V(1).Info("Patched ServerClaim reference on Server", "Server", server.Name, "ServerClaimRef", claim.Name)
	return nil
}

func (r *ServerClaimSchedulerReconciler) patchServerRef(ctx context.Context, claim *metalv1alpha1.ServerClaim, server *metalv1alpha1.Server) (bool, error) {
	if claim.Spec.ServerRef == nil {
		claimBase := claim.DeepCopy()
		claim.Spec.ServerRef = &v1.LocalObjectReference{Name: server.Name}
		controllerutil.AddFinalizer(claim, serverClaimFinalizer)
		if err := r.Patch(ctx, claim, client.MergeFrom(claimBase)); err != nil {
			return false, err
		}
		return true, nil
	}

	if claim.Spec.ServerRef.Name == server.Name {
		return false, nil
	}

	return false, fmt.Errorf("failed to patch server ref for claim: server reference is immutable")
}

func (r *ServerClaimSchedulerReconciler) isUnderMaintenanceQueue(ctx context.Context, server *metalv1alpha1.Server) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if server.Status.State == metalv1alpha1.ServerStateMaintenance || server.Spec.ServerMaintenanceRef != nil {
		log.V(1).Info("Server in or entering Maintenance, Hence can not be claimed")
		return true, nil
	}
	// check if the current available state is a temporary state between multiple Maintenances states.
	// We do not want to claim server while it is undergoing series of Maintenance in sequence.
	serverMaintenancesList := &metalv1alpha1.ServerMaintenanceList{}
	if err := clientutils.ListAndFilter(ctx, r.Client, serverMaintenancesList, func(object client.Object) (bool, error) {
		serverMaintenance := object.(*metalv1alpha1.ServerMaintenance)
		return serverMaintenance.Spec.ServerRef.Name == server.Name, nil
	}); err != nil {
		return true, err
	}
	if len(serverMaintenancesList.Items) == 0 {
		return false, nil
	}
	log.V(1).Info("Server has ongoing Maintenances, Hence can not be claimed")
	return true, nil
}

func (r *ServerClaimSchedulerReconciler) isServerClaimable(ctx context.Context, server *metalv1alpha1.Server, claim *metalv1alpha1.ServerClaim) bool {
	log := ctrl.LoggerFrom(ctx)
	if claimRef := server.Spec.ServerClaimRef; claimRef != nil && (claimRef.Name != claim.Name || claimRef.Namespace != claim.Namespace) {
		log.V(1).Info("Server claim ref does not match claim", "Server", server.Name, "ClaimName", claimRef.Name)
		return false
	}
	if server.Status.State != metalv1alpha1.ServerStateAvailable {
		log.V(1).Info("Server not in a claimable state", "Server", server.Name, "ServerState", server.Status.State)
		return false
	}
	if server.Spec.Unclaimable {
		log.V(1).Info("Server is cordoned", "Server", server.Name, "Claim", claim.Name)
		return false
	}
	if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
		log.V(1).Info("Server is not powered off", "Server", server.Name, "PowerState", server.Status.PowerState)
		return false
	}
	isUnderMaintenance, err := r.isUnderMaintenanceQueue(ctx, server)
	// is undergoing maintenance and not in Reserved State, we should not claim this server
	if err != nil || isUnderMaintenance {
		log.V(1).Info("Server is undergoing Maintenances", "Server", server.Name, "error", err)
		return false
	}
	if !tolerates(server.Spec.Taints, claim.Spec.Tolerations) {
		log.V(1).Info("Server taints not tolerated by claim", "Server", server.Name, "Claim", claim.Name)
		return false
	}
	return true
}

func patchServerClaimPhase(ctx context.Context, c client.Client, claim *metalv1alpha1.ServerClaim, phase metalv1alpha1.Phase) (bool, error) {
	if claim.Status.Phase == phase {
		return false, nil
	}
	claimBase := claim.DeepCopy()
	claim.Status.Phase = phase
	// Optimistic lock: concurrent reconciles may hold a stale claim copy; the
	// loser must conflict and requeue instead of overwriting a newer phase.
	if err := c.Status().Patch(ctx, claim, client.MergeFromWithOptions(claimBase, client.MergeFromWithOptimisticLock{})); err != nil {
		return false, fmt.Errorf("failed to patch server claim phase: %w", err)
	}
	return true, nil
}

func isUnscheduledServerClaim(object client.Object) bool {
	claim := object.(*metalv1alpha1.ServerClaim)
	return claim.Spec.ServerRef == nil || claim.Status.Phase != metalv1alpha1.PhaseBound
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerClaimSchedulerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("serverclaim-scheduler").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.MaxConcurrentReconciles,
		}).
		For(&metalv1alpha1.ServerClaim{}, builder.WithPredicates(predicate.NewPredicateFuncs(isUnscheduledServerClaim))).
		Watches(&metalv1alpha1.Server{}, r.enqueueUnboundServerClaims()).
		Complete(r)
}

func (r *ServerClaimSchedulerReconciler) enqueueUnboundServerClaims() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)

		server := object.(*metalv1alpha1.Server)

		if server.Status.State == metalv1alpha1.ServerStateMaintenance || server.Spec.ServerMaintenanceRef != nil {
			return nil
		}
		var req []reconcile.Request
		claimList := &metalv1alpha1.ServerClaimList{}
		if err := r.List(ctx, claimList); err != nil {
			log.Error(err, "Failed to list ServerClaims")
			return nil
		}
		for _, claim := range claimList.Items {
			if claim.Spec.ServerRef == nil || claim.Spec.ServerRef.Name == server.Name {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name},
				})
			}
		}
		return req
	})
}
