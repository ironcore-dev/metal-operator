// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"net/url"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// GlobalExposureReconciler reconciles a GlobalExposure object
type GlobalExposureReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	BaseURL *url.URL
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=globalexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=globalexposures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=globalexposures/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GlobalExposureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	exposure := &metalv1alpha1.GlobalExposure{}
	if err := r.Get(ctx, req.NamespacedName, exposure); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !exposure.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	base := exposure.DeepCopy()
	exposure.Status.URL = r.BaseURL.JoinPath("gl", exposure.Name).String()
	if err := r.Status().Patch(ctx, exposure, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching exposure status: %w", err)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GlobalExposureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.GlobalExposure{}).
		Named("globalexposure").
		Complete(r)
}
