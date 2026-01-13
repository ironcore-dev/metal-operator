// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/common"
)

const (
	BMCUserFinalizer = "metal.ironcore.dev/bmcuser-finalizer"
)

// BMCUserReconciler reconciles a BMCUser object
type BMCUserReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Insecure   bool
	BMCOptions bmc.Options
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusers/finalizers,verbs=update

// Reconcile reconciles a BMCUser object
func (r *BMCUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	user := &metalv1alpha1.BMCUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, log, user)
}

func (r *BMCUserReconciler) reconcileExists(ctx context.Context, log logr.Logger, user *metalv1alpha1.BMCUser) (ctrl.Result, error) {
	if !user.DeletionTimestamp.IsZero() {
		log.Info("User is being deleted, handling deletion", "User", user.Name)
		return r.delete(ctx, log, user)
	}
	return r.reconcile(ctx, log, user)
}

func (r *BMCUserReconciler) reconcile(ctx context.Context, log logr.Logger, user *metalv1alpha1.BMCUser) (ctrl.Result, error) {
	if user.Spec.BMCRef == nil {
		log.Info("No BMC reference set for User, skipping reconciliation", "User", user.Name)
		return ctrl.Result{}, nil
	}
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, client.ObjectKey{
		Name: user.Spec.BMCRef.Name,
	}, bmcObj); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.updateEffectiveSecret(ctx, log, user, bmcObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update effective BMCSecret: %w", err)
	}
	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, user, BMCUserFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	bmcClient, err := r.getBMCClient(ctx, bmcObj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client: %w", err)
	}
	defer bmcClient.Logout()
	err = r.patchUserStatus(ctx, log, user, bmcClient)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update User status: %w", err)
	}

	if user.Spec.BMCSecretRef == nil {
		log.Info("No BMCSecret reference set for User, creating a new one", "User", user.Name)
		if err := r.handleMissingBMCSecretRef(ctx, log, bmcClient, user, bmcObj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to handle missing BMCSecret reference: %w", err)
		}
	}
	bmcSecret := &metalv1alpha1.BMCSecret{}
	if err := r.Get(ctx, client.ObjectKey{
		Name: user.Spec.BMCSecretRef.Name,
	}, bmcSecret); err != nil {
		return ctrl.Result{}, err
	}
	if user.Status.ID == "" {
		log.Info("No BMC account ID set in User status, creating or updating BMC account", "User", user.Name)
		_, password, err := bmcutils.GetBMCCredentialsFromSecret(bmcSecret)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get credentials from BMCSecret: %w", err)
		}
		if err = bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, password, r.Insecure); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create or update BMC account with new password: %w", err)
		}
		if err = r.patchUserStatus(ctx, log, user, bmcClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update User status after creating BMC account: %w", err)
		}
	}
	if user.Status.EffectiveBMCSecretRef != nil && user.Spec.BMCSecretRef.Name != user.Status.EffectiveBMCSecretRef.Name {
		log.Info("BMCSecret reference has changed, updating BMC account", "User", user.Name)
		if err := r.handleUpdatedSecretRef(ctx, log, user, bmcSecret, bmcClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to handle updated BMCSecret reference: %w", err)
		}
	}
	return r.handleRotatingPassword(ctx, log, user, bmcObj, bmcClient)
}

func (r *BMCUserReconciler) patchUserStatus(ctx context.Context, log logr.Logger, user *metalv1alpha1.BMCUser, bmcClient bmc.BMC) error {
	accounts, err := bmcClient.GetAccounts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get BMC accounts: %w", err)
	}
	for _, account := range accounts {
		if account.UserName == user.Spec.UserName {
			log.V(1).Info("BMC account already exists", "User", user.Name, "ID", account.ID)
			userBase := user.DeepCopy()
			user.Status.ID = account.ID
			exp, err := time.Parse(time.RFC3339, account.PasswordExpiration)
			if err == nil {
				user.Status.PasswordExpiration = &metav1.Time{Time: exp}
			} else {
				log.Error(err, "Failed to parse password expiration time from BMC account", "User", user.Name, "Expiration", account.PasswordExpiration)
			}
			if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
				return fmt.Errorf("failed to patch User status with BMC account ID: %w", err)
			}
			log.Info("Updated User status with BMC account ID", "User", user.Name, "AccountID", account.ID)
			return nil
		}
	}
	return nil
}

func (r *BMCUserReconciler) handleRotatingPassword(ctx context.Context, log logr.Logger, user *metalv1alpha1.BMCUser, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) (ctrl.Result, error) {
	forceRotation := false
	if user.GetAnnotations() != nil && user.GetAnnotations()[metalv1alpha1.OperationAnnotation] == metalv1alpha1.OperationAnnotationRotateCredentials {
		log.Info("User has rotation annotation set, triggering password rotation", "User", user.Name)
		forceRotation = true
		userBase := user.DeepCopy()
		delete(user.Annotations, metalv1alpha1.OperationAnnotation)
		if err := r.Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove rotation annotation from User: %w", err)
		}
	}
	if user.Status.PasswordExpiration != nil {
		if user.Status.PasswordExpiration.Time.Before(metav1.Now().Time) {
			log.Info("BMC user password has expired, rotating password", "User", user.Name)
			// If the password has expired, we need to rotate it
			forceRotation = true
		}
	}
	if user.Spec.RotationPeriod == nil && !forceRotation {
		log.V(1).Info("No rotation period set for BMC user, skipping password rotation", "User", user.Name)
		return ctrl.Result{}, nil
	}
	if user.Status.LastRotation != nil && user.Status.LastRotation.Time.Add(user.Spec.RotationPeriod.Duration).After(time.Now()) && !forceRotation {
		log.V(1).Info("BMC user password rotation is not needed yet", "User", user.Name)
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: user.Spec.RotationPeriod.Duration,
		}, nil
	}
	log.Info("Rotating BMC user password", "User", user.Name)
	accountService, err := bmcClient.GetAccountService(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get account service: %w", err)
	}
	newPassword, err := bmc.GenerateSecurePassword(bmc.Manufacturer(bmcObj.Status.Manufacturer), accountService.MaxPasswordLength)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to generate new password for BMC user %s: %w", user.Name, err)
	}
	if err := r.createSecret(ctx, log, user, newPassword); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMCSecret: %w", err)
	}
	if err := bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, newPassword, r.Insecure); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create or update BMC account with new password: %w", err)
	}
	// Update the last rotation time
	userBase := user.DeepCopy()
	user.Status.LastRotation = &metav1.Time{Time: metav1.Now().Time}
	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch User status with last rotation time: %w", err)
	}
	log.Info("Updated last rotation time for BMC user", "User", user.Name)
	return ctrl.Result{}, nil
}

func (r *BMCUserReconciler) handleMissingBMCSecretRef(ctx context.Context, log logr.Logger, bmcClient bmc.BMC, user *metalv1alpha1.BMCUser, bmcObj *metalv1alpha1.BMC) error {
	log.Info("No BMCSecret reference set for User, creating a new one", "User", user.Name)
	accountService, err := bmcClient.GetAccountService(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account service: %w", err)
	}
	newPassword, err := bmc.GenerateSecurePassword(bmc.Manufacturer(bmcObj.Status.Manufacturer), accountService.MaxPasswordLength)
	if err != nil {
		return fmt.Errorf("failed to generate new password for BMC account %s: %w", user.Name, err)
	}
	if err := r.createSecret(ctx, log, user, newPassword); err != nil {
		return fmt.Errorf("failed to create BMCSecret: %w", err)
	}
	log.Info("Creating BMC account with new password", "Account", user.Name)
	if err := bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, newPassword, r.Insecure); err != nil {
		return fmt.Errorf("failed to create or update BMC account with new password: %w", err)
	}
	log.Info("BMC account created with new password", "Account", user.Name)
	return nil
}

func (r *BMCUserReconciler) handleUpdatedSecretRef(ctx context.Context, log logr.Logger, user *metalv1alpha1.BMCUser, bmcSecret *metalv1alpha1.BMCSecret, bmcClient bmc.BMC) error {
	log.Info("BMCSecret credentials have changed, updating BMC user", "User", user.Name)
	_, password, err := bmcutils.GetBMCCredentialsFromSecret(bmcSecret)
	if err != nil {
		return fmt.Errorf("failed to get credentials from BMCSecret: %w", err)
	}
	// Update the BMC account with the new password
	if err := bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, password, r.Insecure); err != nil {
		return fmt.Errorf("failed to create or update BMC account with new password: %w", err)
	}
	return nil
}

func (r *BMCUserReconciler) createSecret(ctx context.Context, log logr.Logger, user *metalv1alpha1.BMCUser, password string) error {
	log.Info("Creating BMCSecret for User", "User", user.Name)
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
		return fmt.Errorf("failed to create or patch BMCSecret: %w", err)
	}
	log.V(1).Info("BMCSecret created or patched", "BMCSecret", secret.Name, "Operation", op)
	userBase := user.DeepCopy()
	user.Spec.BMCSecretRef = &v1.LocalObjectReference{Name: secret.Name}
	if err := r.Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return fmt.Errorf("failed to patch User status with effective BMCSecretRef: %w", err)
	}
	return nil
}

func (r *BMCUserReconciler) setEffectiveSecretRef(ctx context.Context, log logr.Logger, user *metalv1alpha1.BMCUser, secret *metalv1alpha1.BMCSecret) error {
	log.Info("Setting effective BMCSecret", "User", user.Name)
	userBase := user.DeepCopy()
	if user.Status.EffectiveBMCSecretRef == nil {
		user.Status.EffectiveBMCSecretRef = &v1.LocalObjectReference{}
	}
	user.Status.EffectiveBMCSecretRef.Name = secret.Name
	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return fmt.Errorf("failed to patch User status with effective BMCSecretRef: %w", err)
	}
	return nil
}

func (r *BMCUserReconciler) getBMCClient(ctx context.Context, bmcObj *metalv1alpha1.BMC) (bmc.BMC, error) {
	/* will be needed when supporting admin user management
	if bmcObj.Spec.AdminUserRef != nil && bmcObj.Spec.AdminUserRef.Name == user.Name {
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
	*/
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.Insecure, r.BMCOptions)
	if err != nil {
		return bmcClient, fmt.Errorf("failed to create BMC client: %w", err)
	}

	return bmcClient, nil
}

func (r *BMCUserReconciler) updateEffectiveSecret(ctx context.Context, log logr.Logger, user *metalv1alpha1.BMCUser, bmcObj *metalv1alpha1.BMC) error {
	if user.Spec.BMCSecretRef == nil || user.Status.ID == "" {
		return nil
	}
	secret := &metalv1alpha1.BMCSecret{}
	if err := r.Get(ctx, client.ObjectKey{
		Name: user.Spec.BMCSecretRef.Name,
	}, secret); err != nil {
		return fmt.Errorf("failed to get BMCSecret %s: %w", user.Spec.BMCSecretRef.Name, err)
	}

	invalidCredentials, err := r.bmcConnectionTest(ctx, secret, bmcObj)
	if err != nil {
		return fmt.Errorf("failed to test BMC connection with BMCSecret %s: %w", secret.Name, err)
	}
	if invalidCredentials {
		log.Info("New BMCSecret is invalid, will not update effective BMCSecret", "User", user.Name, "NewBMCSecret", secret.Name)
		return nil
	}
	if user.Status.EffectiveBMCSecretRef == nil {
		if err := r.setEffectiveSecretRef(ctx, log, user, secret); err != nil {
			return fmt.Errorf("failed to update effective BMCSecret: %w", err)
		}
		log.Info("Set effective BMCSecret for User", "User", user.Name)
		return nil
	}

	effSecret := &metalv1alpha1.BMCSecret{}
	if err := r.Get(ctx, client.ObjectKey{
		Name: user.Status.EffectiveBMCSecretRef.Name,
	}, effSecret); err != nil {
		return fmt.Errorf("failed to get effective BMCSecret %s: %w", user.Status.EffectiveBMCSecretRef.Name, err)
	}

	invalidCredentials, err = r.bmcConnectionTest(ctx, effSecret, bmcObj)
	if err != nil {
		return fmt.Errorf("failed to test BMC connection with effectiveSecret %s: %w", effSecret.Name, err)
	}
	if invalidCredentials {
		log.Info("Effective BMCSecret is invalid", "User", user.Name, "EffectiveBMCSecret", effSecret.Name, "NewBMCSecret", secret.Name)
		if err := r.setEffectiveSecretRef(ctx, log, user, secret); err != nil {
			return fmt.Errorf("failed to update effective BMCSecret: %w", err)
		}
		log.Info("Updated effective BMCSecret for User", "User", user.Name)
	}
	return nil
}

func (r *BMCUserReconciler) bmcConnectionTest(ctx context.Context, secret *metalv1alpha1.BMCSecret, bmcObj *metalv1alpha1.BMC) (bool, error) {
	protocolScheme := bmcutils.GetProtocolScheme(bmcObj.Spec.Protocol.Scheme, r.Insecure)
	address, err := bmcutils.GetBMCAddressForBMC(ctx, r.Client, bmcObj)
	if err != nil {
		return false, fmt.Errorf("failed to get BMC address: %w", err)
	}
	_, err = bmcutils.CreateBMCClient(ctx, r.Client, protocolScheme, bmcObj.Spec.Protocol.Name, address, bmcObj.Spec.Protocol.Port, secret, r.BMCOptions)
	if err != nil {
		if httpErr, ok := err.(*common.Error); ok {
			if httpErr.HTTPReturnedStatusCode == 401 || httpErr.HTTPReturnedStatusCode == 403 {
				return true, nil
			}
		}
		return false, fmt.Errorf("failed to create BMC client: %w", err)
	}
	return false, nil
}

func (r *BMCUserReconciler) delete(ctx context.Context, log logr.Logger, user *metalv1alpha1.BMCUser) (ctrl.Result, error) {
	if user.Spec.BMCRef == nil {
		log.Info("No BMC reference set for User, skipping deletion", "User", user.Name)
		return ctrl.Result{}, nil
	}
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, client.ObjectKey{
		Name: user.Spec.BMCRef.Name,
	}, bmcObj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	bmcClient, err := r.getBMCClient(ctx, bmcObj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client: %w", err)
	}
	defer bmcClient.Logout()
	log.Info("Deleting BMC account for User", "User", user.Name)
	if err := bmcClient.DeleteAccount(ctx, user.Spec.UserName, user.Status.ID); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete BMC account: %w", err)
	}
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, user, BMCUserFinalizer); err != nil || modified {
		log.Info("Removed finalizer for User", "User", user.Name)
		return ctrl.Result{}, err
	}
	log.Info("Successfully deleted BMC account and removed finalizer for User", "User", user.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMCUser{}).
		Owns(&metalv1alpha1.BMCSecret{}).
		Named("bmcuser").
		Complete(r)
}
