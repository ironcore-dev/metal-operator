// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
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

	cacheUpdateInterval time.Duration = 20 * time.Millisecond
	cacheUpdateTimeout  time.Duration = time.Second
)

// ServerClaimReconciler reconciles a ServerClaim object
type ServerClaimReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
	claimMutex              sync.Mutex
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverclaims/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch;create;update;patch;delete

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
	if !controllerutil.ContainsFinalizer(claim, ServerClaimFinalizer) {
		log.V(1).Info("Deleted server claim")
		return ctrl.Result{}, nil
	}

	if err := r.cleanupAndShutdownServer(ctx, log, claim); err != nil {
		return ctrl.Result{}, err
	}
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, claim, ServerClaimFinalizer); !apierrors.IsNotFound(err) || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured that the finalizer has been removed")

	log.V(1).Info("Deleted server claim")
	return ctrl.Result{}, nil
}

func (r *ServerClaimReconciler) cleanupAndShutdownServer(ctx context.Context, log logr.Logger, claim *metalv1alpha1.ServerClaim) error {
	if claim.Spec.ServerRef == nil {
		return nil
	}

	server := &metalv1alpha1.Server{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: claim.Spec.ServerRef.Name}, server); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get server: %w", err)
		}
		log.V(1).Info("Server gone")
	}

	if server.Spec.ServerClaimRef != nil {
		if err := r.removeClaimRefFromServer(ctx, server); err != nil {
			return fmt.Errorf("failed to remove claim ref from server: %w", err)
		}
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

	if err := r.removeBootConfigRefFromServerAndPowerOff(ctx, config, server); err != nil {
		return fmt.Errorf("failed to remove boot config ref from server: %w", err)
	}
	log.V(1).Info("Removed boot config ref from server")
	return nil
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

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, claim, ServerClaimFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	server, modified, err := r.claimServer(ctx, log, claim)
	if err != nil || modified {
		return ctrl.Result{Requeue: true}, err
	}
	if server == nil {
		log.V(1).Info("No server found for claim")
		return ctrl.Result{}, nil
	}

	if modified, err := r.patchServerRef(ctx, claim, server); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Patched ServerRef in Claim")

	if err := r.applyBootConfiguration(ctx, log, server, claim); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply boot configuration: %w", err)
	}
	log.V(1).Info("Applied BootConfiguration for ServerClaim")

	if modified, err := r.patchServerClaimPhase(ctx, claim, metalv1alpha1.PhaseBound); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Patched ServerClaim phase", "Phase", claim.Status.Phase)

	if modified, err := r.ensurePowerStateForServer(ctx, log, claim, server); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured PowerState for Server", "Server", server.Name)

	log.V(1).Info("Reconciled server claim")
	return ctrl.Result{}, nil
}

func (r *ServerClaimReconciler) ensureObjectRefForServer(ctx context.Context, log logr.Logger, claim *metalv1alpha1.ServerClaim, server *metalv1alpha1.Server) (bool, error) {
	if server.Spec.ServerClaimRef != nil {
		log.V(1).Info("Server is already claimed", "Server", server.Name, "Claim", server.Spec.ServerClaimRef.Name)
		return false, nil
	}

	if server.Spec.ServerClaimRef == nil {
		serverBase := server.DeepCopy()
		server.Spec.ServerClaimRef = &v1.ObjectReference{
			APIVersion: "metal.ironcore.dev/v1alpha1",
			Kind:       "ServerClaim",
			Namespace:  claim.Namespace,
			Name:       claim.Name,
			UID:        claim.UID,
		}
		if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
			return false, fmt.Errorf("failed to patch claim ref for server: %w", err)
		}
		log.V(1).Info("Patched ServerClaim reference on Server", "Server", server.Name, "ServerClaimRef", claim.Name)
	}
	return true, nil
}

func (r *ServerClaimReconciler) ensurePowerStateForServer(ctx context.Context, log logr.Logger, claim *metalv1alpha1.ServerClaim, server *metalv1alpha1.Server) (bool, error) {
	if server.Spec.Power == claim.Spec.Power {
		return false, nil
	}
	if server.Spec.ServerClaimRef != nil {
		serverBase := server.DeepCopy()
		server.Spec.Power = claim.Spec.Power
		if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
			return false, fmt.Errorf("failed to patch power for server: %w", err)
		}
		log.V(1).Info("Patched desired Power of the claimed Server", "Server", server.Name)
	}
	return true, nil
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

func (r *ServerClaimReconciler) patchServerRef(ctx context.Context, claim *metalv1alpha1.ServerClaim, server *metalv1alpha1.Server) (bool, error) {
	if claim.Spec.ServerRef == nil {
		claimBase := claim.DeepCopy()
		claim.Spec.ServerRef = &v1.LocalObjectReference{Name: server.Name}
		if err := r.Patch(ctx, claim, client.MergeFrom(claimBase)); err != nil {
			return false, err
		}
		return true, nil
	}

	if claim.Spec.ServerRef != nil && claim.Spec.ServerRef.Name == server.Name {
		return false, nil
	}

	return false, fmt.Errorf("failed to patch server ref for claim: server reference is immutable")
}

func (r *ServerClaimReconciler) applyBootConfiguration(ctx context.Context, log logr.Logger, server *metalv1alpha1.Server, claim *metalv1alpha1.ServerClaim) error {
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
	server.Spec.BootConfigurationRef = &v1.ObjectReference{
		Namespace:  config.Namespace,
		Name:       config.Name,
		UID:        config.UID,
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerBootConfiguration",
	}
	return r.Patch(ctx, server, client.MergeFrom(serverBase))
}

func (r *ServerClaimReconciler) removeClaimRefFromServer(ctx context.Context, server *metalv1alpha1.Server) error {
	serverBase := server.DeepCopy()
	server.Spec.ServerClaimRef = nil
	return r.Patch(ctx, server, client.MergeFrom(serverBase))
}

func (r *ServerClaimReconciler) removeBootConfigRefFromServerAndPowerOff(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration, server *metalv1alpha1.Server) error {
	if ref := server.Spec.BootConfigurationRef; ref == nil || (ref.Name != config.Name && ref.Namespace != config.Namespace) {
		return nil
	}

	serverBase := server.DeepCopy()
	server.Spec.BootConfigurationRef = nil
	server.Spec.Power = metalv1alpha1.PowerOff
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *ServerClaimReconciler) claimServer(ctx context.Context, log logr.Logger, claim *metalv1alpha1.ServerClaim) (*metalv1alpha1.Server, bool, error) {
	// fast path: check if the server is already points to the current claim
	// read-only operation, no need to lock
	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList); err != nil {
		return nil, false, err
	}
	if server := checkForPrevUsedServer(log, serverList.Items, claim); server != nil {
		return server, false, nil
	}

	// slow path: claim a server
	// The claimMutex ensures that claiming operations are serialized.
	r.claimMutex.Lock()
	defer r.claimMutex.Unlock()

	var (
		server *metalv1alpha1.Server
		err    error
	)
	switch {
	case claim.Spec.ServerRef != nil:
		server, err = r.claimServerByReference(ctx, log, claim)
	case claim.Spec.ServerSelector != nil:
		server, err = r.claimServerBySelector(ctx, log, claim)
	default:
		server, err = r.claimFirstBestServer(ctx, log)
	}
	if err != nil {
		return nil, false, err
	}
	if server == nil {
		return nil, false, nil
	}
	log.V(1).Info("Matching server found", "Server", server.Name)

	modified, err := r.ensureObjectRefForServer(ctx, log, claim, server)
	if err != nil {
		return nil, modified, err
	}
	log.V(1).Info("Ensured ObjectRef for Server", "Server", server.Name)
	if !modified {
		return server, modified, nil
	}
	// controller-runtime does use a cached client by default, which is updated asynchronously.
	// As the next claiming operation might be performed as soon as the mutex is released
	// it is required to ensure that the server object is up-to-date. Otherwise, the same server
	// might be claimed again by another claim.
	err = wait.PollUntilContextTimeout(ctx, cacheUpdateInterval, cacheUpdateTimeout, true, func(ctx context.Context) (bool, error) {
		var nextServer metalv1alpha1.Server
		if err := r.Get(ctx, client.ObjectKey{Name: server.Name}, &nextServer); err != nil {
			return false, err
		}
		return nextServer.ResourceVersion >= server.ResourceVersion, nil
	})
	return server, modified, err
}

func (r *ServerClaimReconciler) claimServerByReference(ctx context.Context, log logr.Logger, claim *metalv1alpha1.ServerClaim) (*metalv1alpha1.Server, error) {
	log.V(1).Info("Trying to claim server by reference")
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: claim.Spec.ServerRef.Name}, server); err != nil {
		return nil, err
	}
	if claimRef := server.Spec.ServerClaimRef; claimRef != nil && claimRef.UID != claim.UID {
		log.V(1).Info("Server claim ref UID does not match claim", "Server", server.Name, "ClaimUID", claimRef.UID)
		return nil, nil
	}
	if server.Status.State != metalv1alpha1.ServerStateAvailable && server.Status.State != metalv1alpha1.ServerStateReserved {
		log.V(1).Info("Server not in a claimable state", "Server", server.Name, "ServerState", server.Status.State)
		return nil, nil
	}
	if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
		log.V(1).Info("Server is not powered off", "Server", server.Name, "PowerState", server.Status.PowerState)
		return nil, nil
	}
	if claim.Spec.ServerSelector == nil {
		return server, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(claim.Spec.ServerSelector)
	if err != nil {
		return nil, err
	}
	if !selector.Matches(labels.Set(server.ObjectMeta.Labels)) {
		log.V(1).Info("Specified server does not match label selector", "Server", server.Name, "Claim", claim.Name)
		return nil, nil
	}
	return server, nil
}

func (r *ServerClaimReconciler) claimServerBySelector(ctx context.Context, log logr.Logger, claim *metalv1alpha1.ServerClaim) (*metalv1alpha1.Server, error) {
	log.V(1).Info("Trying to claim server by selector")
	selector, err := metav1.LabelSelectorAsSelector(claim.Spec.ServerSelector)
	if err != nil {
		return nil, err
	}
	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}
	for _, server := range serverList.Items {
		if claimRef := server.Spec.ServerClaimRef; claimRef != nil && claimRef.UID != claim.UID {
			log.V(1).Info("Server claim ref UID does not match claim", "Server", server.Name, "ClaimUID", claimRef.UID)
			continue
		}
		if server.Status.State != metalv1alpha1.ServerStateAvailable && server.Status.State != metalv1alpha1.ServerStateReserved {
			log.V(1).Info("Server not in a claimable state", "Server", server.Name, "ServerState", server.Status.State)
			continue
		}
		if server.Status.PowerState != metalv1alpha1.ServerOffPowerState {
			log.V(1).Info("Server is not powered off", "Server", server.Name, "PowerState", server.Status.PowerState)
			continue
		}
		return &server, nil
	}
	return nil, nil
}

func checkForPrevUsedServer(log logr.Logger, servers []metalv1alpha1.Server, claim *metalv1alpha1.ServerClaim) *metalv1alpha1.Server {
	log.V(1).Info("Check for previous claimed server")
	for _, server := range servers {
		if ref := server.Spec.ServerClaimRef; ref != nil {
			if ref.UID == claim.UID && ref.Name == claim.Name && ref.Namespace == claim.Namespace {
				return &server
			}
		}
	}
	return nil
}

func (r *ServerClaimReconciler) claimFirstBestServer(ctx context.Context, log logr.Logger) (*metalv1alpha1.Server, error) {
	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList); err != nil {
		return nil, err
	}
	log.V(1).Info("Trying to claim first best server")
	for _, server := range serverList.Items {
		if server.Spec.ServerClaimRef != nil {
			continue
		}
		if server.Status.State != metalv1alpha1.ServerStateAvailable {
			continue
		}
		return &server, nil
	}

	return nil, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.MaxConcurrentReconciles,
		}).
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
			if claim.Spec.ServerRef != nil && claim.Spec.ServerRef.Name == host.Name {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name},
				})
				return req
			}
			if claim.Spec.ServerRef == nil {
				req = append(req, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: claim.Namespace, Name: claim.Name},
				})
				return req
			}
		}
		return req
	})
}
