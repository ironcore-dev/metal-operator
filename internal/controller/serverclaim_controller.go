/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	v1 "k8s.io/api/core/v1"
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
	ServerClaimFinalizer = "metal.ironcore.dev/serverclaim"
)

// ServerClaimReconciler reconciles a ServerClaim object
type ServerClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims/finalizers,verbs=update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/finalizers,verbs=update
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	claim := &metalv1alpha1.ServerClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, claim)
}

func (r *ServerClaimReconciler) reconcileExists(ctx context.Context, log logr.Logger, claim *metalv1alpha1.ServerClaim) (ctrl.Result, error) {
	if !claim.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, claim)
	}
	return r.reconcile(ctx, log, claim)
}

func (r *ServerClaimReconciler) delete(ctx context.Context, log logr.Logger, claim *metalv1alpha1.ServerClaim) (ctrl.Result, error) {
	log.V(1).Info("Deleting server claim")

	server := &metalv1alpha1.Server{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: claim.Spec.ServerRef.Name}, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get server: %w", err)
	}

	if server.Spec.ServerClaimRef != nil {
		if err := r.removeClaimRefFromServer(ctx, server); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove claim ref from server: %w", err)
		}
	}

	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, claim, ServerClaimFinalizer); !apierrors.IsNotFound(err) || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured that the finalizer has been removed")

	log.V(1).Info("Deleted server claim")
	return ctrl.Result{}, nil
}

// Reconciliation flow of a ServerClaim:
// - Handle reconciliation ignore and late state initialization
// - Check if a ServerRef has been set
// - Ensure finalizer is set on claim
// - Ensure server spec matches claim & set claim ref on server
// - Patch the claim status to bound
// - Apply Boot configuration
func (r *ServerClaimReconciler) reconcile(ctx context.Context, log logr.Logger, claim *metalv1alpha1.ServerClaim) (ctrl.Result, error) {
	log.V(1).Info("Reconciling server claim")
	if shouldIgnoreReconciliation(claim) {
		log.V(1).Info("Skipped Server reconciliation")
		return ctrl.Result{}, nil
	}

	// do late state initialization
	if claim.Status.Phase == "" {
		if modified, err := r.patchServerClaimPhase(ctx, claim, metalv1alpha1.PhaseUnbound); err != nil || modified {
			return ctrl.Result{}, err
		}
	}

	if claim.Spec.ServerRef == nil {
		log.V(1).Info("Claim is not claiming any server")
		return ctrl.Result{}, nil
	}

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, claim, ServerClaimFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	server := &metalv1alpha1.Server{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: claim.Spec.ServerRef.Name}, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get server: %w", err)
	}

	if server.Status.State != metalv1alpha1.ServerStateAvailable {
		log.V(1).Info("Failed to claim server in non available state", "Server", server.Name, "ServerState", server.Status.State)
		return ctrl.Result{}, nil
	}

	// did somebody else claimed this server?
	if claimRef := server.Spec.ServerClaimRef; claimRef != nil && claimRef.UID != claim.UID {
		log.V(1).Info("Server claim ref UID does not match claim", "Server", server.Name, "ClaimUID", claimRef.UID)
		return ctrl.Result{}, nil
	}

	serverBase := server.DeepCopy()
	server.Spec.ServerClaimRef = &v1.ObjectReference{
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerClaim",
		Namespace:  claim.Namespace,
		Name:       claim.Name,
		UID:        claim.UID,
	}
	server.Spec.Power = claim.Spec.Power
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch claim ref for server: %w", err)
	}

	if modified, err := r.patchServerClaimPhase(ctx, claim, metalv1alpha1.PhaseBound); err != nil || modified {
		return ctrl.Result{}, err
	}

	if err := r.applyBootConfiguration(ctx, server, claim); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply boot configuration: %w", err)
	}

	log.V(1).Info("Reconciled server claim")
	return ctrl.Result{}, nil
}

func (r *ServerClaimReconciler) patchServerClaimPhase(ctx context.Context, claim *metalv1alpha1.ServerClaim, phase metalv1alpha1.Phase) (bool, error) {
	if claim.Status.Phase == phase {
		return false, nil
	}
	claimBase := claim.DeepCopy()
	claim.Status.Phase = phase
	if err := r.Status().Patch(ctx, claim, client.MergeFrom(claimBase)); err != nil {
		return false, fmt.Errorf("failed to patch server claim phase: %w", err)
	}
	return true, nil
}

func (r *ServerClaimReconciler) applyBootConfiguration(ctx context.Context, server *metalv1alpha1.Server, claim *metalv1alpha1.ServerClaim) error {
	config := &metalv1alpha1.ServerBootConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "metal.ironcore.dev/v1alpha1",
			Kind:       "ServerBootConfiguration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: claim.Namespace,
			Name:      claim.Name,
		},
		Spec: metalv1alpha1.ServerBootConfigurationSpec{
			ServerRef:         *claim.Spec.ServerRef,
			Image:             claim.Spec.Image,
			IgnitionSecretRef: claim.Spec.IgnitionSecretRef,
		},
	}

	if err := ctrl.SetControllerReference(claim, config, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// TODO: we might want to add a finalizer on the ignition secret
	if err := r.Patch(ctx, config, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to apply boot configuration: %w", err)
	}

	serverBase := server.DeepCopy()
	server.Spec.BootConfigurationRef = &v1.ObjectReference{
		Namespace:  config.Namespace,
		Name:       config.Name,
		UID:        config.UID,
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerBootConfiguration",
	}
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return err
	}

	return nil
}

func (r *ServerClaimReconciler) removeClaimRefFromServer(ctx context.Context, server *metalv1alpha1.Server) error {
	serverBase := server.DeepCopy()
	server.Spec.ServerClaimRef = nil
	return r.Patch(ctx, server, client.MergeFrom(serverBase))
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerClaim{}).
		Owns(&metalv1alpha1.ServerBootConfiguration{}).
		Watches(&metalv1alpha1.Server{}, r.enqueueServerClaimByRefs()).
		Complete(r)
}

func (r *ServerClaimReconciler) enqueueServerClaimByRefs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)

		host := object.(*metalv1alpha1.Server)
		var req []reconcile.Request
		claimList := &metalv1alpha1.ServerClaimList{}
		if err := r.List(ctx, claimList); err != nil {
			log.Error(err, "failed to list host claims")
			return nil
		}
		for _, claim := range claimList.Items {
			if claim.Spec.ServerRef.Name == host.Name {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name},
				})
				return req
			}
		}
		return req
	})
}
