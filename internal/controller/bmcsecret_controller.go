// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// BMCSecretReconciler reconciles a BMCSecret object
type BMCSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BMCSecret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *BMCSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	secret := &metalv1alpha1.BMCSecret{}
	if err := r.Get(ctx, req.NamespacedName, secret); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, log, secret)
}

func (r *BMCSecretReconciler) reconcileExists(ctx context.Context, log logr.Logger, secret *metalv1alpha1.BMCSecret) (ctrl.Result, error) {
	if !secret.DeletionTimestamp.IsZero() {
		//return r.delete(ctx, log, server)
	}
	return r.reconcile(ctx, log, secret)
}

func (r *BMCSecretReconciler) reconcile(ctx context.Context, log logr.Logger, secret *metalv1alpha1.BMCSecret) (ctrl.Result, error) {
	/*
		if secret.PasswordPolicy == string(metalv1alpha1.PasswordPolicyExternal) {
			return ctrl.Result{}, nil
		}
		selector, err := metav1.LabelSelectorAsSelector(secret.ServerSelector)
		if err != nil {
			return ctrl.Result{}, err
		}
		serverList := &metalv1alpha1.ServerList{}
		if err := r.List(ctx, serverList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
			return ctrl.Result{}, err
		}
		for _, server := range serverList.Items {
			//bmcutils.SetBMCSecret(&server, secret)
			user, pw, err := bmcutils.GetBMCCredentialsFromSecret(secret)
			if err != nil {
				return ctrl.Result{}, err
			}
			bmcutils.GetBMCClientForServer(ctx, r.Client, &server, false, bmc.BMCOptions{
				Username: user,
				Password: pw,
			})
		}
	*/
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMCSecret{}).
		Complete(r)
}
