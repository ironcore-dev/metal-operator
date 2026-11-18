// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"cmp"
	"context"
	"fmt"

	metalv1alpha1ac "github.com/ironcore-dev/metal-operator/api/v1alpha1/applyconfiguration/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/boot"
	"github.com/ironcore-dev/metal-operator/internal/deploy"
	"github.com/ironcore-dev/metal-operator/internal/image"
	corev1 "k8s.io/api/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
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

	RepositoryProvider image.RepositoryProvider
	PowerControl       boot.PowerControl

	DirectDeployer *deploy.DirectDeployer
	DiskDeployer   *deploy.DiskDeployer
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
	log.V(1).Info("Deleting ServerClaim")
	if !controllerutil.ContainsFinalizer(claim, serverClaimFinalizer) {
		log.V(1).Info("Deleted ServerClaim")
		return ctrl.Result{}, nil
	}
	log.V(1).Info("Deleted ServerClaim")
	return ctrl.Result{}, nil
}

func (r *ServerClaimReconciler) getRecords(
	ctx context.Context,
	claim *metalv1alpha1.ServerClaim,
) (*metalv1alpha1.Records, error) {
	records := &metalv1alpha1.Records{}
	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, records, func() error {
		return ctrl.SetControllerReference(claim, records, r.Scheme)
	}); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *ServerClaimReconciler) applyRecords(
	ctx context.Context,
	claim *metalv1alpha1.ServerClaim,
	recordsAc *metalv1alpha1ac.RecordsApplyConfiguration,
) error {
	recordsAc = recordsAc.
		WithNamespace(claim.Namespace).
		WithName(claim.Name).
		WithOwnerReferences(
			metav1ac.OwnerReference().
				WithAPIVersion(metalv1alpha1.SchemeGroupVersion.String()).
				WithKind("ServerClaim").
				WithName(claim.Name).
				WithUID(claim.UID).
				WithController(true))
	if err := r.Apply(ctx, recordsAc, fieldOwner, client.ForceOwnership); err != nil {
		return fmt.Errorf("applying records: %w", err)
	}
	return nil
}

func (r *ServerClaimReconciler) resolveImage(
	ctx context.Context,
	server *metalv1alpha1.Server,
	claim *metalv1alpha1.ServerClaim,
	records *deploy.Records,
) error {
	log := logf.FromContext(ctx)

	img := claim.Spec.Image
	repo, err := r.RepositoryProvider.RepositoryFor(img)
	if err != nil {
		return fmt.Errorf("getting repository for %q: %w", img, err)
	}

	imageRef, imageKind, err := image.Resolve(ctx, repo, img, server.Spec.Architecture)
	if err != nil {
		return fmt.Errorf("resolving image ref of %q: %w", img, err)
	}

	log.V(1).Info("Resolved image", "image", img, "imageRef", imageRef, "imageKind", imageKind)

	log.V(5).Info("Setting image")
	return records.SetImage(ctx, metalv1alpha1.Image{
		Ref:  imageRef,
		Kind: imageKind,
	})
}

func (r *ServerClaimReconciler) reconcile(ctx context.Context, claim *metalv1alpha1.ServerClaim) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	server := &metalv1alpha1.Server{}
	serverKey := client.ObjectKey{Name: claim.Spec.ServerRef.Name}
	if err := r.Get(ctx, serverKey, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("getting server: %w", err)
	}

	records, err := deploy.RecordsFor(ctx, r.Client, claim)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting records: %w", err)
	}

	if records.Image() == nil {
		if err := r.resolveImage(ctx, server, claim, records); err != nil {
			return ctrl.Result{}, fmt.Errorf("resolving image: %w", err)
		}
	}

	if claim.Spec.Power == metalv1alpha1.PowerOff {
		res, err := r.PowerControl.ApplyPower(ctx, server, false, func() error {
			log.V(1).Info("Issued server shutdown")
			return nil
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("applying power: %w", err)
		}
		if res.Pending {
			return res.AsReconcileResult(), nil
		}
	}

	switch kind := records.Image().Kind; kind {
	case metalv1alpha1.ImageKindDirect:
		return r.reconcileDirect(ctx, server, claim, records)
	case metalv1alpha1.ImageKindDisk:
		return r.reconcileDisk(ctx, server, claim, records)
	default:
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("unknown image kind %q", kind))
	}
}

func (r *ServerClaimReconciler) getUserData(ctx context.Context, namespace string, ref *metalv1alpha1.LocalObjectKeyReference) ([]byte, error) {
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: namespace, Name: ref.Name}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("getting secret %s: %w", ref.Name, err)
	}

	key := cmp.Or(ref.Key, metalv1alpha1.DefaultUserDataKey)
	userData, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("secret %q does not contain key %q", secret.Name, key)
	}
	return userData, nil
}

func (r *ServerClaimReconciler) reconcileDirect(ctx context.Context, server *metalv1alpha1.Server, claim *metalv1alpha1.ServerClaim, records *deploy.Records) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var userData []byte
	if ref := claim.Spec.UserDataRef; ref != nil {
		retrievedUserData, err := r.getUserData(ctx, claim.Namespace, ref)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("getting user data: %w", err)
		}

		userData = retrievedUserData
	}

	res, err := r.DirectDeployer.Deploy(ctx, server, records, deploy.DirectConfig{
		Metadata: boot.ServerClaimMetadata{
			Namespace: claim.Namespace,
			Name:      claim.Name,
			UID:       claim.UID,
		},
		ImageRef: records.Image().Ref,
		UserData: userData,
		Hostname: claim.Name,
		Cmdline:  claim.Spec.Cmdline,
		Args:     claim.Spec.Args,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deploy direct: %w", err)
	}
	if res.Pending {
		return res.AsReconcileResult(), nil
	}

	res, err = r.PowerControl.ApplyPower(ctx, server, true, func() error {
		log.V(1).Info("Issued server power-on")
		return nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("setting power: %w", err)
	}
	if res.Pending {
		return res.AsReconcileResult(), nil
	}
	if err := records.UpdateMarkers(ctx, deploy.AddMarker(metalv1alpha1.MarkerBooted)); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating markers: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *ServerClaimReconciler) reconcileDisk(ctx context.Context, server *metalv1alpha1.Server, claim *metalv1alpha1.ServerClaim, records *deploy.Records) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var userData []byte
	if ref := claim.Spec.UserDataRef; ref != nil {
		retrievedUserData, err := r.getUserData(ctx, claim.Namespace, ref)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("getting user data: %w", err)
		}

		userData = retrievedUserData
	}

	res, err := r.DiskDeployer.Deploy(ctx, server, records, deploy.DiskConfig{
		Metadata: boot.ServerClaimMetadata{
			Namespace: claim.Namespace,
			Name:      claim.Name,
			UID:       claim.UID,
		},
		ImageRef: records.Image().Ref,
		Hostname: claim.Name,
		UserData: userData,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deploy disk: %w", err)
	}
	if res.Pending {
		return res.AsReconcileResult(), nil
	}

	res, err = r.PowerControl.ApplyPower(ctx, server, true, func() error {
		log.V(1).Info("Issued server power-on")
		return nil
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("setting power: %w", err)
	}
	if res.Pending {
		return res.AsReconcileResult(), nil
	}
	if err := records.UpdateMarkers(ctx, deploy.AddMarker(metalv1alpha1.MarkerBooted)); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating markers: %w", err)
	}
	return ctrl.Result{}, nil
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
		Owns(&metalv1alpha1.Records{}).
		Watches(&metalv1alpha1.Server{}, r.enqueueServerClaimByRefs()).
		Complete(r)
}

func (r *ServerClaimReconciler) enqueueServerClaimByRefs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)

		server := object.(*metalv1alpha1.Server)

		claimList := &metalv1alpha1.ServerClaimList{}
		if err := r.List(ctx, claimList, client.MatchingFields{serverRefField: server.Name}); err != nil {
			log.Error(err, "Failed to list ServerClaims")
			return nil
		}
		req := make([]reconcile.Request, 0, len(claimList.Items))
		for _, claim := range claimList.Items {
			req = append(req, reconcile.Request{
				Namespace: claim.Namespace,
				Name:      claim.Name,
			})
		}
		return req
	})
}
