// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	serverClaimFinalizer = "metal.ironcore.dev/serverclaim"
)

// ServerClaimReconciler reconciles a ServerClaim object.
type ServerClaimReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	claim := &metalv1alpha1.ServerClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, claim)
}

func (r *ServerClaimReconciler) reconcileExists(ctx context.Context, claim *metalv1alpha1.ServerClaim) (ctrl.Result, error) {
	if !claim.DeletionTimestamp.IsZero() {
		return r.delete(ctx, claim)
	}
	return r.reconcile(ctx, claim)
}

func (r *ServerClaimReconciler) delete(ctx context.Context, claim *metalv1alpha1.ServerClaim) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Deleting server claim")
	if !controllerutil.ContainsFinalizer(claim, serverClaimFinalizer) {
		log.V(1).Info("Deleted server claim")
		return ctrl.Result{}, nil
	}

	if err := r.cleanupAndShutdownServer(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}
	if _, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, claim, serverClaimFinalizer); err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured that the finalizer has been removed")

	log.V(1).Info("Deleted server claim")
	return ctrl.Result{}, nil
}

func (r *ServerClaimReconciler) cleanupAndShutdownServer(ctx context.Context, claim *metalv1alpha1.ServerClaim) error {
	log := ctrl.LoggerFrom(ctx)
	if claim.Spec.ServerRef == nil {
		return nil
	}

	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: claim.Spec.ServerRef.Name}, server); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get server: %w", err)
		}
		log.V(1).Info("Server gone")
	}

	config := &metalv1alpha1.ServerBootConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claim.Name,
			Namespace: claim.Namespace,
		},
	}
	if err := r.Delete(ctx, config); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete serverbootconfig: %w", err)
		}
		log.V(1).Info("ServerBootConfiguration gone")
	}

	return nil
}

// Reconciliation flow of a bound ServerClaim:
// - Handle reconciliation ignore
// - Ensure finalizer is set on claim
// - Wait until the scheduler has bound and marked the claim bound
// - Apply Boot configuration once the server is reserved
// - Ensure the power state
func (r *ServerClaimReconciler) reconcile(ctx context.Context, claim *metalv1alpha1.ServerClaim) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling server claim")
	if shouldIgnoreReconciliation(claim) {
		log.V(1).Info("Skipped Server claim reconciliation")
		return ctrl.Result{}, nil
	}

	if claim.Spec.ServerRef != nil {
		server := &metalv1alpha1.Server{}
		if err := r.Get(ctx, client.ObjectKey{Name: claim.Spec.ServerRef.Name}, server); err == nil {
			if isServerParkingOrParked(server) {
				log.V(1).Info("Bound server is parked, standing down", "Server", server.Name)
				return ctrl.Result{}, nil
			}
		} else if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get server for parked check: %w", err)
		}
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, claim, serverClaimFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	if claim.Spec.ServerRef == nil {
		log.V(1).Info("Claim is not scheduled to a server yet")
		return ctrl.Result{}, nil
	}

	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: claim.Spec.ServerRef.Name}, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if claim.Status.Phase != metalv1alpha1.PhaseBound {
		log.V(1).Info("Claim is not bound yet, waiting for scheduler")
		return ctrl.Result{}, nil
	}

	if ref := server.Spec.ServerClaimRef; ref == nil || ref.Name != claim.Name || ref.Namespace != claim.Namespace {
		log.V(1).Info("Server is not claimed for this claim yet, waiting for scheduler", "Server", server.Name)
		return ctrl.Result{}, nil
	}

	if server.Status.State != metalv1alpha1.ServerStateReserved {
		log.V(1).Info("Server is not in reserved state", "Server", server.Name, "ServerState", server.Status.State)
		return ctrl.Result{}, nil
	}

	if err := r.applyBootConfiguration(ctx, server, claim); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply boot configuration: %w", err)
	}
	log.V(1).Info("Applied BootConfiguration for ServerClaim")

	log.V(1).Info("Reconciled server claim")
	return ctrl.Result{}, nil
}

func (r *ServerClaimReconciler) applyBootConfiguration(ctx context.Context, server *metalv1alpha1.Server, claim *metalv1alpha1.ServerClaim) error {
	log := ctrl.LoggerFrom(ctx)
	config := &metalv1alpha1.ServerBootConfiguration{}
	config.Name = claim.Name
	config.Namespace = claim.Namespace
	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, config, func() error {
		// TODO: we might want to add a finalizer on the ignition secret
		config.Spec.ServerRef = *claim.Spec.ServerRef
		config.Spec.Image = claim.Spec.Image
		config.Spec.IgnitionSecretRef = claim.Spec.IgnitionSecretRef
		return ctrl.SetControllerReference(claim, config, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to create or patch ServerBootConfiguration: %w", err)
	}
	log.V(1).Info("Created or patched ServerBootConfiguration", "ServerBootConfiguration", config.Name, "Operation", opResult)

	serverBase := server.DeepCopy()
	server.Spec.BootConfigurationRef = &metalv1alpha1.ObjectReference{
		Namespace: config.Namespace,
		Name:      config.Name,
	}
	return r.Patch(ctx, server, client.MergeFrom(serverBase))
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.MaxConcurrentReconciles,
		}).
		For(&metalv1alpha1.ServerClaim{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return object.(*metalv1alpha1.ServerClaim).Spec.ServerRef != nil
		}))).
		Owns(&metalv1alpha1.ServerBootConfiguration{}).
		Watches(&metalv1alpha1.Server{}, r.enqueueServerClaimByRefs()).
		Complete(r)
}

func (r *ServerClaimReconciler) enqueueServerClaimByRefs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)

		server := object.(*metalv1alpha1.Server)

		var req []reconcile.Request
		claimList := &metalv1alpha1.ServerClaimList{}
		if err := r.List(ctx, claimList); err != nil {
			log.Error(err, "Failed to list ServerClaims")
			return nil
		}
		for _, claim := range claimList.Items {
			if claim.Spec.ServerRef != nil && claim.Spec.ServerRef.Name == server.Name {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name},
				})
			}
		}
		return req
	})
}
