// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const (
	serverReadinessRuleFinalizer = "readiness.metal.ironcore.dev/cleanup-taints"
)

// ServerReadinessRuleReconciler reconciles a ServerReadinessRule object
type ServerReadinessRuleReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverreadinessrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverreadinessrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverreadinessrules/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
<<<<<<< HEAD
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServerReadinessRule object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
=======
>>>>>>> tmp-original-23-06-26-01-04
func (r *ServerReadinessRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	rule := &metalv1alpha1.ServerReadinessRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !rule.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, rule)
	}

	base := rule.DeepCopy()
	if modified := controllerutil.AddFinalizer(rule, serverReadinessRuleFinalizer); !modified {
		log.V(1).Info("Finalizer present, nothing to do")
		return ctrl.Result{}, nil
	}
	if err := r.Patch(ctx, rule, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
	}
	log.V(1).Info("Added finalizer")

	return ctrl.Result{}, nil
}

func (r *ServerReadinessRuleReconciler) reconcileDelete(
	ctx context.Context,
	rule *metalv1alpha1.ServerReadinessRule,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if !controllerutil.ContainsFinalizer(rule, serverReadinessRuleFinalizer) {
		log.V(1).Info("Finalizer not present, nothing to do")
		return ctrl.Result{}, nil
	}

	sel, err := metav1.LabelSelectorAsSelector(&rule.Spec.ServerSelector)
	if err != nil {
		return ctrl.Result{}, reconcile.TerminalError(fmt.Errorf("parsing label selector: %w", err))
	}

	serverList := &metalv1alpha1.ServerList{}
	if err := r.List(ctx, serverList, client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing servers: %w", err)
	}

	var errs []error
	for _, server := range serverList.Items {
		base := server.DeepCopy()
		modified := removeTaint(&server, rule.Spec.Taint)
		if !modified {
			continue
		}
		if err := r.Patch(ctx, &server, client.MergeFrom(base)); err != nil {
			errs = append(errs, fmt.Errorf("patching server %s: %w", server.Name, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching servers: %w", err)
	}

	log.V(1).Info("Patched all servers, releasing finalizer")
	base := rule.DeepCopy()
	controllerutil.RemoveFinalizer(rule, serverReadinessRuleFinalizer)
	if err := r.Patch(ctx, rule, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing finalizer")
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerReadinessRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerReadinessRule{}).
		Named("serverreadinessrule").
		Complete(r)
}
