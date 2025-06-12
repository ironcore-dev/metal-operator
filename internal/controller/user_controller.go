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

func (r *UserReconciler) reconcileExists(ctx context.Context, log logr.Logger, user *metalv1alpha1.User) (ctrl.Result, error) {
	if !user.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, user)
	}
	return r.reconcile(ctx, log, user)
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
	bmcClient, err := r.getBMCClient(ctx, log, bmcObj, user)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client: %w", err)
	}
	defer bmcClient.Logout()
	if user.Spec.BMCSecretRef == nil {
		log.Info("No BMCSecret reference set for User, creating a new one", "User", user.Name)
		if err := r.handleMissingBMCSecretRef(ctx, log, bmcClient, user); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to handle missing BMCSecret reference: %w", err)
		}
	}
	bmcSecret := &metalv1alpha1.BMCSecret{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: user.Namespace,
		Name:      user.Spec.BMCSecretRef.Name,
	}, bmcSecret); err != nil {
		return ctrl.Result{}, err
	}
	if user.Status.EffectiveBMCSecretRef == nil {
		return r.handleMissingEffectiveBMCSecretRef(ctx, log, bmcClient, bmcSecret, user)
	}
	if user.Spec.BMCSecretRef.Name != user.Status.EffectiveBMCSecretRef.Name {
		log.Info("BMCSecret reference has changed, updating BMC account", "User", user.Name)
		if err := r.handleUpdatedSecretRef(ctx, log, user, bmcSecret, bmcClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to handle updated BMCSecret reference: %w", err)
		}
	}
	return r.handleRotatingPassword(ctx, log, user, bmcClient)
}

func (r *UserReconciler) handleRotatingPassword(ctx context.Context, log logr.Logger, user *metalv1alpha1.User, bmcClient bmc.BMC) (ctrl.Result, error) {
	forceRotation := false
	if user.GetAnnotations() != nil && user.GetAnnotations()[metalv1alpha1.OperationAnnotation] == metalv1alpha1.OperationAnnotationRotateCredentials {
		log.Info("User has rotation annotation set, triggering password rotation", "User", user.Name)
		forceRotation = true
	}
	if user.Spec.RotationPeriod == nil && !forceRotation {
		log.V(1).Info("No rotation period set for BMC user, skipping password rotation", "User", user.Name)
		return ctrl.Result{}, nil
	}
	if user.Status.LastRotation.Add(user.Spec.RotationPeriod.Duration).After(metav1.Now().Time) && !forceRotation {
		log.V(1).Info("BMC user password rotation is not needed yet", "User", user.Name)
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: user.Spec.RotationPeriod.Duration,
		}, nil
	}
	newPassword, err := GenerateRandomPassword(16)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to generate new password for BMC user %s: %w", user.Name, err)
	}
	newSecret, err := r.createSecret(ctx, log, user, string(newPassword))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMCSecret: %w", err)
	}
	if err = r.updatePassword(ctx, log, user, string(newPassword), newSecret, bmcClient); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update BMC user password: %w", err)
	}
	// Update the last rotation time
	userBase := user.DeepCopy()
	user.Status.LastRotation = &metav1.Time{Time: metav1.Now().Time}
	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch User status with last rotation time: %w", err)
	}
	log.Info("Updated last rotation time for BMC user", "User", user.Name)
	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: user.Spec.RotationPeriod.Duration,
	}, nil
}

func (r *UserReconciler) handleMissingBMCSecretRef(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, user *metalv1alpha1.User) error {
	log.Info("No BMCSecret reference set for User, creating a new one", "User", user.Name)
	newPassword, err := GenerateRandomPassword(16)
	if err != nil {
		return fmt.Errorf("failed to generate new password for BMC account %s: %w", user.Name, err)
	}
	newSecret, err := r.createSecret(ctx, log, user, string(newPassword))
	if err != nil {
		return fmt.Errorf("failed to create BMCSecret: %w", err)
	}
	log.Info("Creating BMC account with new password", "Account", user.Name)
	if err = r.updatePassword(ctx, log, user, string(newPassword), newSecret, bmcClient); err != nil {
		return fmt.Errorf("failed to update BMC account password: %w", err)
	}
	log.Info("BMC account created with new password", "Account", user.Name)

	return nil
}

func (r *UserReconciler) handleMissingEffectiveBMCSecretRef(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, bmcSecret *metalv1alpha1.BMCSecret, user *metalv1alpha1.User) (ctrl.Result, error) {
	log.Info("No effective BMCSecret reference set for User", "User", user.Name)
	_, password, err := bmcutils.GetBMCCredentialsFromSecret(bmcSecret)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get credentials from BMCSecret: %w", err)
	}
	if err = r.updatePassword(ctx, log, user, password, bmcSecret, bmcClient); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update BMC account password: %w", err)
	}
	log.Info("Created effective BMCSecret for User", "User", user.Name)
	if user.Spec.RotationPeriod == nil {
		// If no rotation period is set, we don't need to requeue
		return ctrl.Result{}, nil
	}
	log.Info("Requeuing for password rotation", "User", user.Name, "RotationPeriod", user.Spec.RotationPeriod.Duration)
	// If a rotation period is set, we requeue for the next rotation
	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: user.Spec.RotationPeriod.Duration,
	}, nil
}

func (r *UserReconciler) handleUpdatedSecretRef(ctx context.Context, log logr.Logger, user *metalv1alpha1.User, bmcSecret *metalv1alpha1.BMCSecret, bmcClient bmc.BMC) error {
	log.Info("BMCSecret credentials have changed, updating BMC user", "User", user.Name)
	_, password, err := bmcutils.GetBMCCredentialsFromSecret(bmcSecret)
	if err != nil {
		return fmt.Errorf("failed to get credentials from BMCSecret: %w", err)
	}
	if err = r.updatePassword(ctx, log, user, password, bmcSecret, bmcClient); err != nil {
		return fmt.Errorf("failed to update BMC account password: %w", err)
	}
	return nil
}

func (r *UserReconciler) updatePassword(ctx context.Context, log logr.Logger, user *metalv1alpha1.User, password string, secret *metalv1alpha1.BMCSecret, bmcClient bmc.BMC) error {
	if err := r.removeEffectiveSecret(ctx, log, user); err != nil {
		return fmt.Errorf("failed to remove effective BMCSecret: %w", err)
	}
	// Update the BMC account with the new password
	if err := bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, password, r.Insecure); err != nil {
		return fmt.Errorf("failed to create or update BMC account with new password: %w", err)
	}
	// Update the effective BMCSecret with the new password
	if err := r.setEffectiveSecretRef(ctx, log, user, secret); err != nil {
		return fmt.Errorf("failed to update effective BMCSecret: %w", err)
	}
	return nil
}

func (r *UserReconciler) removeEffectiveSecret(ctx context.Context, log logr.Logger, user *metalv1alpha1.User) error {
	log.Info("Removing effective BMCSecret for User", "User", user.Name)
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
		passwordBytes, err := GenerateRandomPassword(16)
		if err != nil {
			return nil, fmt.Errorf("failed to generate new password for BMC account %s: %w", user.Name, err)
		}
		password = string(passwordBytes)
	}
	secret := &metalv1alpha1.BMCSecret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: user.Name + "-bmcsecret-",
		},
		Data: map[string][]byte{
			metalv1alpha1.BMCSecretUsernameKeyName: []byte(user.Spec.UserName),
			metalv1alpha1.BMCSecretPasswordKeyName: []byte(password),
		},
		Immutable: &[]bool{true}[0], // Make the secret immutable
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
	log.V(1).Info("BMCSecret created or patched", "BMCSecret", secret.Name, "Operation", op)
	userBase := user.DeepCopy()
	user.Spec.BMCSecretRef = &v1.LocalObjectReference{Name: secret.Name}
	if err := r.Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return secret, fmt.Errorf("failed to patch User status with effective BMCSecretRef: %w", err)
	}
	return secret, nil
}

func (r *UserReconciler) setEffectiveSecretRef(ctx context.Context, log logr.Logger, user *metalv1alpha1.User, secret *metalv1alpha1.BMCSecret) error {
	log.Info("Creating effective BMCSecret for Account", "User", user.Name)
	userBase := user.DeepCopy()
	if user.Status.EffectiveBMCSecretRef == nil {
		if user.Status.EffectiveBMCSecretRef == nil {
			user.Status.EffectiveBMCSecretRef = &v1.LocalObjectReference{}
		}
		user.Status.EffectiveBMCSecretRef.Name = secret.Name
	}
	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return fmt.Errorf("failed to patch User status with effective BMCSecretRef: %w", err)
	}
	return nil
}

func (r *UserReconciler) getBMCClient(ctx context.Context, log logr.Logger, bmcObj *metalv1alpha1.BMC, user *metalv1alpha1.User) (bmcClient bmc.BMC, err error) {
	if bmcObj.Spec.AdminUserRef.Name == user.Name {
		if user.Spec.BMCSecretRef == nil {
			// if this user is the admin user, we cannot create a BMC client without a BMCSecretRef (password)
			return bmcClient, fmt.Errorf("BMC %s admin user %s does not have a BMCSecretRef set", bmcObj.Name, user.Name)
		}
		log.Info("User is the admin user for the BMC", "User", user.Name)
		protocolScheme := bmcutils.GetProtocolScheme(bmcObj.Spec.Protocol.Scheme, r.Insecure)
		address, err := bmcutils.GetBMCAddressForBMC(ctx, r.Client, bmcObj)
		if err != nil {
			return bmcClient, fmt.Errorf("failed to get BMC address: %w", err)
		}
		bmcSecret := &metalv1alpha1.BMCSecret{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: user.Namespace,
			Name:      user.Spec.BMCSecretRef.Name,
		}, bmcSecret); err != nil {
			return bmcClient, err
		}
		bmcClient, err = bmcutils.CreateBMCClient(ctx, r.Client, protocolScheme, bmcObj.Spec.Protocol.Name, address, bmcObj.Spec.Protocol.Port, bmcSecret, r.BMCOptions)
		if err != nil {
			return bmcClient, fmt.Errorf("failed to create BMC client: %w", err)
		}
	} else {
		bmcClient, err = bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCOptions)
		if err != nil {
			return bmcClient, fmt.Errorf("failed to create BMC client: %w", err)
		}
	}
	return
}

func (r *UserReconciler) delete(ctx context.Context, log logr.Logger, user *metalv1alpha1.User) (ctrl.Result, error) {
	log.V(1).Info("Deleting User", "User", user.Name)
	if user.Status.EffectiveBMCSecretRef != nil {
		log.V(1).Info("Removing effective BMCSecret reference from User", "User", user.Name)
		if err := r.removeEffectiveSecret(ctx, log, user); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove effective BMCSecret reference: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.User{}).
		Complete(r)
}
