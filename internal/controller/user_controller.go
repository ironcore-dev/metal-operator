// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

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

// UserReconciler reconciles a Account object
type UserReconciler struct {
	client.Client
	Insecure   bool
	BMCOptions bmc.BMCOptions
	Scheme     *runtime.Scheme
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
func (r *UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	user := &metalv1alpha1.User{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, user)
}

func (r *UserReconciler) reconcileExists(ctx context.Context, log logr.Logger, accObj *metalv1alpha1.User) (ctrl.Result, error) {
	if !accObj.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, accObj)
	}
	return r.reconcile(ctx, log, accObj)
}

func (r *UserReconciler) reconcile(ctx context.Context, log logr.Logger, user *metalv1alpha1.User) (ctrl.Result, error) {
	if user.Spec.BMCRef == nil {
		log.Info("No BMC reference set for User, skipping reconciliation", "User", user.Name)
		return ctrl.Result{}, nil
	}
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: user.Namespace,
		Name:      user.Spec.BMCRef.Name,
	}, bmcObj); err != nil {
		return ctrl.Result{}, err
	}
	if bmcObj.Spec.AdminUserRef == nil {
		return ctrl.Result{}, fmt.Errorf("BMC %s does not have an admin user reference set", bmcObj.Name)
	}
	bmcClient, err := bmcutils.GetBMCClientFromAdminUserRef(ctx, r.Client, bmcObj, r.Insecure, r.BMCOptions)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()

	if user.Spec.BMCSecretRef == nil {
		// Generate a new password
		newPassword, err := GenerateRandomPassword(16)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to generate new password for BMC account %s: %w", user.Name, err)
		}
		log.Info("Creating BMC account with new password", "Account", user.Name)
		if err = r.updatePassword(ctx, log, user, string(newPassword), bmcClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update BMC account password: %w", err)
		}
	}
	bmcSecret := &metalv1alpha1.BMCSecret{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: user.Namespace,
		Name:      user.Spec.BMCSecretRef.Name,
	}, bmcSecret); err != nil {
		return ctrl.Result{}, err
	}
	effectiveSecret := &metalv1alpha1.BMCSecret{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: user.Namespace,
		Name:      user.Spec.BMCSecretRef.Name,
	}, effectiveSecret); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMCSecret: %w", err)
	}
	effUsernamne, effPasword, err := bmcutils.GetBMCCredentialsFromSecret(effectiveSecret)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get credentials from BMCSecret: %w", err)
	}
	username, password, err := bmcutils.GetBMCCredentialsFromSecret(bmcSecret)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get credentials from BMCSecret: %w", err)
	}
	if effPasword != password || effUsernamne != username {
		log.Info("BMCSecret credentials have changed, updating BMC account", "Account", user.Name)
		// remove effective BMCSecret reference from user status
		if err = r.updatePassword(ctx, log, user, string(password), bmcClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update BMC account password: %w", err)
		}
	}
	return r.rotatePassword(ctx, log, user, bmcClient)
}

func (r *UserReconciler) rotatePassword(ctx context.Context, log logr.Logger, user *metalv1alpha1.User, bmcClient bmc.BMC) (ctrl.Result, error) {
	if user.Spec.RotationPeriod == nil {
		return ctrl.Result{}, nil
	}
	if user.Status.LastRotation == nil {
		// If the rotation period is set but the last rotation time is not set, we need to initialize it
		log.Info("Initializing last rotation time for BMC user", "User", user.Name)
		userBase := user.DeepCopy()
		user.Status.LastRotation = &metav1.Time{Time: metav1.Now().Time}
		if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch User status with last rotation time: %w", err)
		}
		log.Info("Initialized last rotation time for BMC user", "User", user.Name)
		return ctrl.Result{}, nil
	}
	if user.Status.LastRotation.Add(user.Spec.RotationPeriod.Duration).After(metav1.Now().Time) {
		log.V(1).Info("BMC user password rotation is not needed yet", "User", user.Name)
		return ctrl.Result{}, nil
	}
	// Generate a new password
	newPassword, err := GenerateRandomPassword(16)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to generate new password for BMC account %s: %w", user.Name, err)
	}
	if err = r.updatePassword(ctx, log, user, string(newPassword), bmcClient); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update BMC account password: %w", err)
	}
	// Update the last rotation time
	userBase := user.DeepCopy()
	user.Status.LastRotation = &metav1.Time{Time: metav1.Now().Time}
	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch User status with last rotation time: %w", err)
	}
	log.Info("Updated last rotation time for BMC account", "Account", user.Name)
	return ctrl.Result{}, nil
}

func (r *UserReconciler) updatePassword(ctx context.Context, log logr.Logger, user *metalv1alpha1.User, password string, bmcClient bmc.BMC) error {
	if err := r.removeEffectiveSecret(ctx, log, user); err != nil {
		return fmt.Errorf("failed to remove effective BMCSecret: %w", err)
	}
	// Update the BMC account with the new password
	if err := bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, password, r.Insecure); err != nil {
		return fmt.Errorf("failed to create or update BMC account with new password: %w", err)
	}
	newSecret, err := r.createSecret(ctx, log, user, password)
	if err != nil {
		return fmt.Errorf("failed to create BMCSecret with new password: %w", err)
	}
	// Update the effective BMCSecret with the new password
	if err := r.setEffectiveSecretRef(ctx, log, user, newSecret); err != nil {
		return fmt.Errorf("failed to update effective BMCSecret: %w", err)
	}
	return nil
}

func (r *UserReconciler) removeEffectiveSecret(ctx context.Context, log logr.Logger, user *metalv1alpha1.User) error {
	log.Info("Removing effective BMCSecret for Account", "User", user.Name)

	// Remove the effective BMCSecret reference from the user status
	userBase := user.DeepCopy()
	user.Status.EffectiveBMCSecretRef = nil
	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return fmt.Errorf("failed to patch User status to remove effective BMCSecretRef: %w", err)
	}

	log.V(1).Info("Removed effective BMCSecret reference from User status", "User", user.Name)
	return nil
}

func (r *UserReconciler) createSecret(ctx context.Context, log logr.Logger, user *metalv1alpha1.User, password string) (*metalv1alpha1.BMCSecret, error) {
	log.Info("Creating BMCSecret for User", "User", user.Name)

	if password == "" {
		// Generate a new password
		passwordBytes, err := GenerateRandomPassword(16)
		if err != nil {
			return nil, fmt.Errorf("failed to generate new password for BMC account %s: %w", user.Name, err)
		}
		password = string(passwordBytes)
	}

	// Create the BMCSecret
	secret := &metalv1alpha1.BMCSecret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: user.Name + "-bmcsecret-",
			Namespace:    user.Namespace,
		},
		Data: map[string][]byte{
			metalv1alpha1.BMCSecretUsernameKeyName: []byte(user.Spec.UserName),
			metalv1alpha1.BMCSecretPasswordKeyName: []byte(password),
		},
	}
	op, err := controllerutil.CreateOrPatch(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetControllerReference(user, secret, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for BMCSecret: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create or patch BMCSecret: %w", err)
	}
	log.V(1).Info("", "operation", op)

	userBase := user.DeepCopy()
	user.Spec.BMCSecretRef = &v1.LocalObjectReference{Name: secret.Name}
	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return secret, fmt.Errorf("failed to patch User status with effective BMCSecretRef: %w", err)
	}
	return secret, nil
}

func (r *UserReconciler) setEffectiveSecretRef(ctx context.Context, log logr.Logger, user *metalv1alpha1.User, secret *metalv1alpha1.BMCSecret) error {
	log.Info("Creating effective BMCSecret for Account", "User", user.Name)

	userBase := user.DeepCopy()
	// Update the user status with the effective BMCSecret reference
	if user.Status.EffectiveBMCSecretRef == nil {
		if user.Status.EffectiveBMCSecretRef == nil {
			user.Status.EffectiveBMCSecretRef = &v1.LocalObjectReference{}
		}
		user.Status.EffectiveBMCSecretRef.Name = secret.Name
	}
	// Set the effective BMCSecret reference in the user status
	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return fmt.Errorf("failed to patch User status with effective BMCSecretRef: %w", err)
	}

	return nil
}

func (r *UserReconciler) delete(ctx context.Context, log logr.Logger, user *metalv1alpha1.User) (ctrl.Result, error) {
	log.V(1).Info("Deleting User", "User", user.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.User{}).
		Complete(r)
}
