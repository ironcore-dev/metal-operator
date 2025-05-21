// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

// AccountReconciler reconciles a Account object
type AccountReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=accounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=accounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=accounts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Account object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *AccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	accObj := &metalv1alpha1.Account{}
	if err := r.Get(ctx, req.NamespacedName, accObj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, accObj)
}

func (r *AccountReconciler) reconcileExists(ctx context.Context, log logr.Logger, accObj *metalv1alpha1.Account) (ctrl.Result, error) {
	if !accObj.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, accObj)
	}
	return r.reconcile(ctx, log, accObj)
}

func (r *AccountReconciler) reconcile(ctx context.Context, log logr.Logger, accObj *metalv1alpha1.Account) (ctrl.Result, error) {
	selector, err := metav1.LabelSelectorAsSelector(accObj.Spec.BMCSelector)
	if err != nil {
		return ctrl.Result{}, err
	}
	bmcList := &metalv1alpha1.BMCList{}
	if err := r.List(ctx, bmcList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return ctrl.Result{}, err
	}
	accSecret := &metalv1alpha1.BMCSecret{}
	if accObj.Spec.BMCSecretRef.Name == "" {
		password, err := GenerateRandomPassword(16)
		if err != nil {
			return ctrl.Result{}, err
		}
		accSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: accObj.Namespace,
				Name:      accObj.Name,
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte(accObj.Spec.Name),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte(password),
			},
		}
		if err := controllerutil.SetControllerReference(accObj, accSecret, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, accSecret); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// if the secret already exists, get it
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: accObj.Namespace,
			Name:      accObj.Spec.BMCSecretRef.Name,
		}, accSecret); err != nil {
			return ctrl.Result{}, err
		}
	}

	username, pasword, err := bmcutils.GetBMCCredentialsFromSecret(accSecret)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, b := range bmcList.Items {
		if accObj.Spec.MetalUser {
			b.Spec.BMCSecretRef = v1.LocalObjectReference{
				Name: accSecret.Name,
			}
			if err := r.Update(ctx, &b); err != nil {
				return ctrl.Result{}, err
			}
		}
		bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, &b, false, bmc.BMCOptions{})
		if err != nil {
			return ctrl.Result{}, err
		}
		bmcClient.CreateOrUpdateAccount(ctx, accObj.Spec.Name, username, accObj.Spec.RoleID, pasword, accObj.Spec.Enabled)
		// set the active user for the bmc client
		if accObj.Status.State == metalv1alpha1.AccountStateActive {
			bmcSecret := &metalv1alpha1.BMCSecret{}
			if err = r.Get(ctx, client.ObjectKey{
				Namespace: b.Namespace,
				Name:      b.Spec.BMCSecretRef.Name,
			}, bmcSecret); err != nil {
				return ctrl.Result{}, err
			}
			bmcSecret.Data["username"] = []byte(username)
			bmcSecret.Data["password"] = []byte(pasword)
			if err = r.Update(ctx, bmcSecret); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *AccountReconciler) delete(ctx context.Context, log logr.Logger, accObj *metalv1alpha1.Account) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.Account{}).
		Complete(r)
}
