// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"slices"

	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	fieldOwner = client.FieldOwner("metal.ironcore.dev/controller-manager")
)

type BMCTaskFetchFailedError struct {
	TaskURI  string
	Resource string
	Err      error
}

func (e BMCTaskFetchFailedError) Error() string {
	return e.Err.Error()
}

type MultiErrorTracker struct {
	Identifier string
	Err        error
}

func (e MultiErrorTracker) Error() string {
	return e.Err.Error()
}

// GetServerMaintenanceForObjectReference returns a ServerMaintenance object for a given reference.
func GetServerMaintenanceForObjectReference(ctx context.Context, c client.Client, ref *metalv1alpha1.ObjectReference) (*metalv1alpha1.ServerMaintenance, error) {
	if ref == nil {
		return nil, fmt.Errorf("got nil reference")
	}
	maintenance := &metalv1alpha1.ServerMaintenance{}
	if err := c.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, maintenance); err != nil {
		return nil, fmt.Errorf("failed to get ServerMaintenance: %w", err)
	}

	return maintenance, nil
}

// shouldProceedWithDeletion returns true when obj should proceed with deletion.
// isProgressing is called only when the object has the finalizer; it returns true
// when deletion should be postponed (e.g. actively progressing under maintenance).
// Callers own all state/ref/owner logic inside isProgressing.
func shouldProceedWithDeletion(
	ctx context.Context,
	obj client.Object,
	finalizer string,
	isProgressing func() (bool, error),
) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	if obj.GetDeletionTimestamp().IsZero() {
		return false, nil
	}
	if controllerutil.ContainsFinalizer(obj, finalizer) {
		progressing, err := isProgressing()
		if err != nil {
			return false, err
		}
		if progressing {
			log.V(1).Info("Postponing deletion: resource is progressing under active maintenance")
			return false, nil
		}
	}
	log.V(1).Info("Proceeding with deletion")
	return true, nil
}

// GetCondition finds a condition in a condition slice.
// If the condition is not found, a new one with Status=False is returned.
func GetCondition(acc *conditionutils.Accessor, conditions []metav1.Condition, conditionType string) (*metav1.Condition, error) {
	condition := &metav1.Condition{}
	condFound, err := acc.FindSlice(conditions, conditionType, condition)

	if err != nil {
		return nil, fmt.Errorf("failed to find Condition %v. error: %w", conditionType, err)
	}
	if !condFound {
		condition.Type = conditionType
		if err := acc.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
		); err != nil {
			return condition, fmt.Errorf("failed to create/update new Condition %v. error: %w", conditionType, err)
		}
	}

	return condition, nil
}

// GetServerByName returns a Server object by its name or an error in case the object can not be found.
func GetServerByName(ctx context.Context, c client.Client, serverName string) (*metalv1alpha1.Server, error) {
	server := &metalv1alpha1.Server{}
	if err := c.Get(ctx, client.ObjectKey{Name: serverName}, server); err != nil {
		return nil, err
	}
	return server, nil
}

// shouldIgnoreReconciliation checks if the object should be ignored during reconciliation.
func shouldIgnoreReconciliation(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return slices.Contains([]string{
		metalv1alpha1.OperationAnnotationIgnore,
	}, val)
}

func isServerParked(server *metalv1alpha1.Server) bool {
	_, ok := server.GetAnnotations()[metalv1alpha1.ParkedAnnotation]
	return ok
}

func isServerParkingOrParked(server *metalv1alpha1.Server) bool {
	if isServerParked(server) {
		return true
	}
	return server.GetAnnotations()[metalv1alpha1.OperationAnnotation] == metalv1alpha1.OperationAnnotationPark
}

func isResetAnnotation(operation string) bool {
	_, ok := metalv1alpha1.AnnotationToRedfishMapping[operation]
	return ok
}

func isParkableState(state metalv1alpha1.ServerState) bool {
	switch state {
	case metalv1alpha1.ServerStateAvailable, metalv1alpha1.ServerStateReserved:
		return true
	}
	return false
}

// shouldRetryReconciliation checks if the object should retry reconciliation from failed state.
func shouldRetryReconciliation(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationRetry || val == metalv1alpha1.OperationAnnotationRetryPropagated
}

// GenerateRandomPassword generates a random password of the given length.
func GenerateRandomPassword(length int) ([]byte, error) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range length {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return nil, fmt.Errorf("failed to generate random password: %w", err)
		}
		result[i] = letters[n.Int64()]
	}
	return result, nil
}

func GetImageCredentialsForSecretRef(ctx context.Context, c client.Client, secretRef *corev1.SecretReference) (string, string, error) {
	if secretRef == nil {
		return "", "", fmt.Errorf("got nil secretRef")
	}
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: secretRef.Namespace, Name: secretRef.Name}, secret); err != nil {
		return "", "", err
	}

	username, ok := secret.Data[metalv1alpha1.BMCSecretUsernameKeyName]
	if !ok {
		return "", "", fmt.Errorf("no username found in secret")
	}
	password, ok := secret.Data[metalv1alpha1.BMCSecretPasswordKeyName]
	if !ok {
		return "", "", fmt.Errorf("no password found in secret")
	}

	return string(username), string(password), nil
}
