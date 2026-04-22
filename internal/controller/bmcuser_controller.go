// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/ironcore-dev/controller-utils/clientutils"
	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"github.com/stmcginnis/gofish/schemas"
)

const (
	BMCUserFinalizer = "metal.ironcore.dev/bmcuser"
)

// BMCUserReconciler reconciles a BMCUser object
type BMCUserReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	DefaultProtocol    metalv1alpha1.ProtocolScheme
	SkipCertValidation bool
	BMCOptions         bmc.Options
	Conditions         *conditionutils.Accessor
}

// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile reconciles a BMCUser object
func (r *BMCUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	user := &metalv1alpha1.BMCUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, user)
}

func (r *BMCUserReconciler) reconcileExists(ctx context.Context, user *metalv1alpha1.BMCUser) (ctrl.Result, error) {
	if !user.DeletionTimestamp.IsZero() {
		return r.delete(ctx, user)
	}
	return r.reconcile(ctx, user)
}

func (r *BMCUserReconciler) reconcile(ctx context.Context, user *metalv1alpha1.BMCUser) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if user.Spec.BMCRef == nil {
		log.Info("No BMC reference set for User, skipping reconciliation")
		return ctrl.Result{}, nil
	}
	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, client.ObjectKey{Name: user.Spec.BMCRef.Name}, bmcObj); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.updateEffectiveSecret(ctx, user, bmcObj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update effective BMCSecret: %w", err)
	}
	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, user, BMCUserFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}

	// Calculate expiration time if needed (ONCE at creation)
	isTemporary, err := r.calculateExpirationTime(ctx, user)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if user has expired -> trigger deletion
	expired, err := r.checkExpiration(ctx, user)
	if err != nil {
		return ctrl.Result{}, err
	}
	if expired {
		// Deletion triggered, no requeue needed
		return ctrl.Result{}, nil
	}

	// Update expiration warning condition
	if isTemporary {
		if err := r.checkAndUpdateExpirationWarning(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	bmcClient, err := r.getBMCClient(ctx, bmcObj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client: %w", err)
	}
	defer bmcClient.Logout()
	if err = r.patchUserStatus(ctx, user, bmcClient); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update User status: %w", err)
	}

	if user.Spec.BMCSecretRef == nil {
		log.Info("No BMCSecret reference set for User, creating a new one")
		if err := r.ensureBMCSecretForUser(ctx, bmcClient, user, bmcObj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to handle missing BMCSecret reference: %w", err)
		}
	}
	bmcSecret := &metalv1alpha1.BMCSecret{}
	if err := r.Get(ctx, client.ObjectKey{Name: user.Spec.BMCSecretRef.Name}, bmcSecret); err != nil {
		return ctrl.Result{}, err
	}

	if user.Status.ID == "" {
		log.Info("No BMC account ID set in User status, creating or updating BMC account")
		_, password, err := bmcutils.GetBMCCredentialsFromSecret(bmcSecret)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get credentials from BMCSecret: %w", err)
		}
		if err = bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, password, true); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create or update BMC account with new password: %w", err)
		}
		if err = r.patchUserStatus(ctx, user, bmcClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update User status after creating BMC account: %w", err)
		}
	}
	if user.Status.EffectiveBMCSecretRef != nil && user.Spec.BMCSecretRef.Name != user.Status.EffectiveBMCSecretRef.Name {
		log.Info("BMCSecret reference has changed, updating BMC account")
		if err := r.handleUpdatedSecretRef(ctx, user, bmcSecret, bmcClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to handle updated BMCSecret reference: %w", err)
		}
	}

	// Handle password rotation and coordinate requeue
	rotationResult, err := r.handleRotatingPassword(ctx, user, bmcObj, bmcClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Coordinate requeue between rotation and expiration
	return r.coordinateRequeue(user, rotationResult), nil
}

func (r *BMCUserReconciler) patchUserStatus(ctx context.Context, user *metalv1alpha1.BMCUser, bmcClient bmc.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	accounts, err := bmcClient.GetAccounts()
	if err != nil {
		return fmt.Errorf("failed to get BMC accounts: %w", err)
	}
	for _, account := range accounts {
		if account.UserName == user.Spec.UserName {
			log.V(1).Info("BMC account already exists", "ID", account.ID)
			userBase := user.DeepCopy()
			user.Status.ID = account.ID
			exp, err := time.Parse(time.RFC3339, account.PasswordExpiration)
			if err == nil {
				user.Status.PasswordExpiration = &metav1.Time{Time: exp}
			} else {
				log.Error(err, "Failed to parse password expiration time from BMC account", "Expiration", account.PasswordExpiration)
			}
			if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
				return fmt.Errorf("failed to patch User status with BMC account ID: %w", err)
			}
			log.Info("Updated User status with BMC account ID", "AccountID", account.ID)
			return nil
		}
	}
	return nil
}

func (r *BMCUserReconciler) handleRotatingPassword(ctx context.Context, user *metalv1alpha1.BMCUser, bmcObj *metalv1alpha1.BMC, bmcClient bmc.BMC) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	forceRotation := false
	if user.GetAnnotations() != nil && user.GetAnnotations()[metalv1alpha1.OperationAnnotation] == metalv1alpha1.OperationAnnotationRotateCredentials {
		log.Info("User has rotation annotation set, triggering password rotation")
		forceRotation = true
		userBase := user.DeepCopy()
		delete(user.Annotations, metalv1alpha1.OperationAnnotation)
		if err := r.Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove rotation annotation from User: %w", err)
		}
	}
	if user.Status.PasswordExpiration != nil {
		if user.Status.PasswordExpiration.Time.Before(metav1.Now().Time) {
			log.Info("BMC user password has expired, rotating password")
			// If the password has expired, we need to rotate it
			forceRotation = true
		}
	}
	if user.Spec.RotationPeriod == nil && !forceRotation {
		log.V(1).Info("No rotation period set for BMC user, skipping password rotation")
		return ctrl.Result{}, nil
	}
	if user.Spec.RotationPeriod != nil &&
		user.Status.LastRotation != nil &&
		user.Status.LastRotation.Time.Add(user.Spec.RotationPeriod.Duration).After(time.Now()) &&
		!forceRotation {
		log.V(1).Info("BMC user password rotation is not needed yet")
		return ctrl.Result{RequeueAfter: user.Spec.RotationPeriod.Duration}, nil
	}
	log.Info("Rotating BMC user password")
	accountService, err := bmcClient.GetAccountService()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get account service: %w", err)
	}
	newPassword, err := bmc.GenerateSecurePassword(bmc.Manufacturer(bmcObj.Status.Manufacturer), int(accountService.MaxPasswordLength))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to generate new password for BMC user %s: %w", user.Name, err)
	}
	secret, err := r.createBMCSecretForUser(ctx, user, newPassword)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create BMCSecret: %w", err)
	}
	if err := r.setBMCUserSecretRef(ctx, user, secret); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set BMCSecret reference for User: %w", err)
	}
	if err := bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, newPassword, true); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create or update BMC account with new password: %w", err)
	}
	// Update the last rotation time
	userBase := user.DeepCopy()
	user.Status.LastRotation = &metav1.Time{Time: metav1.Now().Time}
	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch User status with last rotation time: %w", err)
	}
	log.Info("Updated last rotation time for BMC user")
	return ctrl.Result{}, nil
}

func (r *BMCUserReconciler) ensureBMCSecretForUser(ctx context.Context, bmcClient bmc.BMC, user *metalv1alpha1.BMCUser, bmcObj *metalv1alpha1.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("No BMCSecret reference set for User, creating a new one")
	accountService, err := bmcClient.GetAccountService()
	if err != nil {
		return fmt.Errorf("failed to get account service: %w", err)
	}
	newPassword, err := bmc.GenerateSecurePassword(bmc.Manufacturer(bmcObj.Status.Manufacturer), int(accountService.MaxPasswordLength))
	if err != nil {
		return fmt.Errorf("failed to generate new password for BMC account %s: %w", user.Name, err)
	}
	secret, err := r.createBMCSecretForUser(ctx, user, newPassword)
	if err != nil {
		return fmt.Errorf("failed to create BMCSecret: %w", err)
	}
	if err := r.setBMCUserSecretRef(ctx, user, secret); err != nil {
		return fmt.Errorf("failed to set BMCSecret reference for User: %w", err)
	}
	log.Info("Creating BMC account with new password", "Account", user.Name)
	if err := bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, newPassword, true); err != nil {
		return fmt.Errorf("failed to create or update BMC account with new password: %w", err)
	}
	log.Info("BMC account created with new password", "Account", user.Name)
	return nil
}

func (r *BMCUserReconciler) handleUpdatedSecretRef(ctx context.Context, user *metalv1alpha1.BMCUser, bmcSecret *metalv1alpha1.BMCSecret, bmcClient bmc.BMC) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("BMCSecret credentials have changed, updating BMC user")
	_, password, err := bmcutils.GetBMCCredentialsFromSecret(bmcSecret)
	if err != nil {
		return fmt.Errorf("failed to get credentials from BMCSecret: %w", err)
	}
	// Update the BMC account with the new password
	if err := bmcClient.CreateOrUpdateAccount(ctx, user.Spec.UserName, user.Spec.RoleID, password, true); err != nil {
		return fmt.Errorf("failed to create or update BMC account with new password: %w", err)
	}
	return nil
}

func (r *BMCUserReconciler) createBMCSecretForUser(ctx context.Context, user *metalv1alpha1.BMCUser, password string) (*metalv1alpha1.BMCSecret, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Creating BMCSecret for User")
	secret := &metalv1alpha1.BMCSecret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: user.Name,
		},
		Data: map[string][]byte{
			metalv1alpha1.BMCSecretUsernameKeyName: []byte(user.Spec.UserName),
			metalv1alpha1.BMCSecretPasswordKeyName: []byte(password),
		},
		Immutable: ptr.To(true), // Make the secret immutable
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
	return secret, nil
}

func (r *BMCUserReconciler) setBMCUserSecretRef(ctx context.Context, user *metalv1alpha1.BMCUser, secret *metalv1alpha1.BMCSecret) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Setting BMCSecret reference for User", "User", user.Name)
	userBase := user.DeepCopy()
	user.Spec.BMCSecretRef = &v1.LocalObjectReference{Name: secret.Name}
	if err := r.Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return fmt.Errorf("failed to patch User with BMCSecretRef: %w", err)
	}
	return nil
}

func (r *BMCUserReconciler) setEffectiveSecretRef(ctx context.Context, user *metalv1alpha1.BMCUser, secret *metalv1alpha1.BMCSecret) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Setting effective BMCSecret")
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
	bmcClient, err := bmcutils.GetBMCClientFromBMC(ctx, r.Client, bmcObj, r.DefaultProtocol, r.SkipCertValidation, r.BMCOptions)
	if err != nil {
		return bmcClient, fmt.Errorf("failed to create BMC client: %w", err)
	}

	return bmcClient, nil
}

func (r *BMCUserReconciler) updateEffectiveSecret(ctx context.Context, user *metalv1alpha1.BMCUser, bmcObj *metalv1alpha1.BMC) error {
	log := ctrl.LoggerFrom(ctx)
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
		log.Info("New BMCSecret is invalid, will not update effective BMCSecret", "NewBMCSecret", secret.Name)
		return nil
	}
	if user.Status.EffectiveBMCSecretRef == nil {
		if err := r.setEffectiveSecretRef(ctx, user, secret); err != nil {
			return fmt.Errorf("failed to update effective BMCSecret: %w", err)
		}
		log.Info("Set effective BMCSecret for User")
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
		log.Info("Effective BMCSecret is invalid", "EffectiveBMCSecret", effSecret.Name, "NewBMCSecret", secret.Name)
		if err := r.setEffectiveSecretRef(ctx, user, secret); err != nil {
			return fmt.Errorf("failed to update effective BMCSecret: %w", err)
		}
		log.Info("Updated effective BMCSecret for User")
	}
	return nil
}

func (r *BMCUserReconciler) bmcConnectionTest(ctx context.Context, secret *metalv1alpha1.BMCSecret, bmcObj *metalv1alpha1.BMC) (bool, error) {
	protocolScheme := bmcutils.GetProtocolScheme(bmcObj.Spec.Protocol.Scheme, r.DefaultProtocol)
	address, err := bmcutils.GetBMCAddressForBMC(ctx, r.Client, bmcObj)
	if err != nil {
		return false, fmt.Errorf("failed to get BMC address: %w", err)
	}
	bmcClient, err := bmcutils.CreateBMCClient(ctx, r.Client, protocolScheme, bmcObj.Spec.Protocol.Name, address, bmcObj.Spec.Protocol.Port, secret, r.BMCOptions, r.SkipCertValidation)
	if err != nil {
		var httpErr *schemas.Error
		if errors.As(err, &httpErr) {
			if httpErr.HTTPReturnedStatusCode == 401 || httpErr.HTTPReturnedStatusCode == 403 {
				return true, nil
			}
		}
		return false, fmt.Errorf("failed to create BMC client: %w", err)
	}
	defer bmcClient.Logout()
	return false, nil
}

func (r *BMCUserReconciler) delete(ctx context.Context, user *metalv1alpha1.BMCUser) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if user.Spec.BMCRef == nil {
		log.Info("No BMC reference set for User, removing finalizer")
		if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, user, BMCUserFinalizer); err != nil || modified {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	bmcObj := &metalv1alpha1.BMC{}
	if err := r.Get(ctx, client.ObjectKey{Name: user.Spec.BMCRef.Name}, bmcObj); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Info("BMC not found, removing finalizer")
			if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, user, BMCUserFinalizer); err != nil || modified {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	bmcClient, err := r.getBMCClient(ctx, bmcObj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get BMC client: %w", err)
	}
	defer bmcClient.Logout()

	log.Info("Deleting BMC account for User")
	if err := bmcClient.DeleteAccount(ctx, user.Spec.UserName, user.Status.ID); err != nil {
		var httpErr *schemas.Error
		if errors.As(err, &httpErr) && httpErr.HTTPReturnedStatusCode == 404 {
			log.Info("BMC account not found, continuing finalizer removal")
		} else {
			return ctrl.Result{}, fmt.Errorf("failed to delete BMC account: %w", err)
		}
	}
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, user, BMCUserFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.Info("Successfully deleted BMC account and removed finalizer for User")
	return ctrl.Result{}, nil
}

// calculateExpirationTime sets status.ExpiresAt based on spec.TTL or spec.ExpiresAt.
// Only called once - when user is first created (status.ExpiresAt is nil).
// Returns whether expiration was set and any error.
func (r *BMCUserReconciler) calculateExpirationTime(ctx context.Context, user *metalv1alpha1.BMCUser) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// Already calculated - don't recalculate
	if user.Status.ExpiresAt != nil {
		return true, nil
	}

	// No TTL or ExpiresAt - permanent user
	if user.Spec.TTL == nil && user.Spec.ExpiresAt == nil {
		return false, nil
	}

	userBase := user.DeepCopy()

	if user.Spec.TTL != nil {
		// Calculate from creation time + TTL
		expiresAt := user.CreationTimestamp.Add(user.Spec.TTL.Duration)
		user.Status.ExpiresAt = &metav1.Time{Time: expiresAt}
		log.Info("Calculated expiration time from TTL",
			"TTL", user.Spec.TTL.Duration,
			"ExpiresAt", expiresAt)
	} else if user.Spec.ExpiresAt != nil {
		// Use absolute expiration time
		user.Status.ExpiresAt = user.Spec.ExpiresAt
		log.Info("Using absolute expiration time",
			"ExpiresAt", user.Spec.ExpiresAt.Time)
	}

	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return false, fmt.Errorf("failed to update status with expiration time: %w", err)
	}

	return true, nil
}

// checkExpiration checks if the user has expired and triggers deletion if needed.
// Returns true if expired (deletion triggered), false otherwise.
func (r *BMCUserReconciler) checkExpiration(ctx context.Context, user *metalv1alpha1.BMCUser) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// Not a temporary user
	if user.Status.ExpiresAt == nil {
		return false, nil
	}

	now := metav1.Now()
	if now.After(user.Status.ExpiresAt.Time) {
		log.Info("BMCUser has expired, triggering deletion",
			"ExpiresAt", user.Status.ExpiresAt.Time,
			"Now", now.Time)

		// Update condition before deletion
		if err := r.updateActiveCondition(ctx, user, false,
			metalv1alpha1.BMCUserReasonExpired,
			fmt.Sprintf("User expired at %s", user.Status.ExpiresAt.Format(time.RFC3339))); err != nil {
			return false, err
		}

		// Delete the user
		if err := r.Delete(ctx, user); err != nil {
			return false, fmt.Errorf("failed to delete expired user: %w", err)
		}

		return true, nil
	}

	return false, nil
}

// updateActiveCondition updates the Active condition based on expiration status.
func (r *BMCUserReconciler) updateActiveCondition(ctx context.Context, user *metalv1alpha1.BMCUser,
	active bool, reason, message string) error {

	log := ctrl.LoggerFrom(ctx)

	status := metav1.ConditionTrue
	if !active {
		status = metav1.ConditionFalse
	}

	condition := &metav1.Condition{}
	found, err := r.Conditions.FindSlice(user.Status.Conditions,
		metalv1alpha1.BMCUserConditionTypeActive, condition)
	if err != nil {
		return fmt.Errorf("failed to find Active condition: %w", err)
	}

	// Check if condition needs updating
	needsUpdate := !found ||
		condition.Status != status ||
		condition.Reason != reason ||
		condition.Message != message

	if !needsUpdate {
		return nil
	}

	userBase := user.DeepCopy()

	if err := r.Conditions.UpdateSlice(
		&user.Status.Conditions,
		metalv1alpha1.BMCUserConditionTypeActive,
		conditionutils.UpdateStatus(status),
		conditionutils.UpdateReason(reason),
		conditionutils.UpdateMessage(message),
	); err != nil {
		return fmt.Errorf("failed to update Active condition: %w", err)
	}

	if err := r.Status().Patch(ctx, user, client.MergeFrom(userBase)); err != nil {
		return fmt.Errorf("failed to patch BMCUser status with condition: %w", err)
	}

	log.V(1).Info("Updated Active condition",
		"Status", status, "Reason", reason)

	return nil
}

// checkAndUpdateExpirationWarning checks if user is in warning period and updates condition.
// Warning period is the smaller of: 10% of TTL or 1 hour before expiration.
func (r *BMCUserReconciler) checkAndUpdateExpirationWarning(ctx context.Context, user *metalv1alpha1.BMCUser) error {
	// Not a temporary user
	if user.Status.ExpiresAt == nil {
		// Ensure Active=True for permanent users
		return r.updateActiveCondition(ctx, user, true,
			metalv1alpha1.BMCUserReasonActive,
			"User is active")
	}

	now := metav1.Now()
	timeUntilExpiration := user.Status.ExpiresAt.Sub(now.Time)

	// Calculate warning threshold
	var warningThreshold time.Duration
	if user.Spec.TTL != nil {
		// 10% of TTL, capped at 1 hour
		warningThreshold = min(time.Duration(float64(user.Spec.TTL.Duration)*0.1), time.Hour)
	} else {
		// For absolute ExpiresAt, use 1 hour
		warningThreshold = time.Hour
	}

	if timeUntilExpiration <= warningThreshold && timeUntilExpiration > 0 {
		return r.updateActiveCondition(ctx, user, true,
			metalv1alpha1.BMCUserReasonExpiringSoon,
			fmt.Sprintf("User will expire in %s at %s",
				timeUntilExpiration.Round(time.Minute),
				user.Status.ExpiresAt.Format(time.RFC3339)))
	}

	// Normal active state
	return r.updateActiveCondition(ctx, user, true,
		metalv1alpha1.BMCUserReasonActive,
		fmt.Sprintf("User is active, expires at %s",
			user.Status.ExpiresAt.Format(time.RFC3339)))
}

// coordinateRequeue determines the next requeue time considering both
// password rotation and expiration checking.
func (r *BMCUserReconciler) coordinateRequeue(user *metalv1alpha1.BMCUser, rotationResult ctrl.Result) ctrl.Result {
	// Not a temporary user - use rotation result as-is
	if user.Status.ExpiresAt == nil {
		return rotationResult
	}

	now := metav1.Now()
	timeUntilExpiration := user.Status.ExpiresAt.Sub(now.Time)

	// Already expired - no requeue
	if timeUntilExpiration <= 0 {
		return ctrl.Result{}
	}

	// Calculate when we need to check next for expiration/warning
	var expirationRequeue time.Duration

	// Calculate warning threshold
	var warningThreshold time.Duration
	if user.Spec.TTL != nil {
		warningThreshold = min(time.Duration(float64(user.Spec.TTL.Duration)*0.1), time.Hour)
	} else {
		warningThreshold = time.Hour
	}

	if timeUntilExpiration > warningThreshold {
		// Not in warning period yet - requeue when warning starts
		expirationRequeue = timeUntilExpiration - warningThreshold
	} else {
		// In warning period - check frequently, but not past expiration
		expirationRequeue = min(5*time.Minute, timeUntilExpiration)
	}

	// Choose the earlier of rotation or expiration requeue
	if rotationResult.RequeueAfter > 0 && rotationResult.RequeueAfter < expirationRequeue {
		return rotationResult
	}

	return ctrl.Result{RequeueAfter: expirationRequeue}
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BMCUser{}).
		Owns(&metalv1alpha1.BMCSecret{}).
		Named("bmcuser").
		Complete(r)
}
