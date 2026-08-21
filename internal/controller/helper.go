// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"slices"

	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
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
		metalv1alpha1.OperationAnnotationIgnoreChildAndSelf,
		metalv1alpha1.OperationAnnotationIgnorePropagated,
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

// shouldChildIgnoreReconciliation checks if the object Child should ignore reconciliation.
// if Parent has OperationAnnotation set to ignore-child, Child should also ignore reconciliation.
func shouldChildIgnoreReconciliation(parentObj client.Object) bool {
	val, found := parentObj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationIgnoreChild || val == metalv1alpha1.OperationAnnotationIgnoreChildAndSelf
}

// isChildIgnoredThroughSets checks if the object's child is marked ignore operation through parent.
func isChildIgnoredThroughSets(childObj client.Object) bool {
	annotations := childObj.GetAnnotations()
	valChildIgnore, found := annotations[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return valChildIgnore == metalv1alpha1.OperationAnnotationIgnorePropagated
}

// shouldRetryReconciliation checks if the object should retry reconciliation from failed state.
func shouldRetryReconciliation(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationRetry || val == metalv1alpha1.OperationAnnotationRetryPropagated
}

// shouldChildRetryReconciliation checks if the object Child should retry reconciliation.
// if Parent has OperationAnnotation set to retry-child, Child should also retry reconciliation.
func shouldChildRetryReconciliation(parentObj client.Object) bool {
	val, found := parentObj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationRetryChild || val == metalv1alpha1.OperationAnnotationRetryChildAndSelf
}

// isChildRetryThroughSets checks if the object's child is marked retry operation through parent.
func isChildRetryThroughSets(childObj client.Object) bool {
	annotations := childObj.GetAnnotations()
	valChildRetry, found := annotations[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return valChildRetry == metalv1alpha1.OperationAnnotationRetryPropagated
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

func enqueueFromChildObjUpdatesExceptAnnotation(e event.UpdateEvent) bool {
	isNil := func(arg any) bool {
		if v := reflect.ValueOf(arg); !v.IsValid() || ((v.Kind() == reflect.Pointer ||
			v.Kind() == reflect.Interface ||
			v.Kind() == reflect.Slice ||
			v.Kind() == reflect.Map ||
			v.Kind() == reflect.Chan ||
			v.Kind() == reflect.Func) && v.IsNil()) {
			return true
		}
		return false
	}

	if isNil(e.ObjectOld) {
		return false
	}
	if isNil(e.ObjectNew) {
		return false
	}

	newAnnotations := isChildIgnoredThroughSets(e.ObjectNew)
	oldAnnotations := isChildIgnoredThroughSets(e.ObjectOld)

	// when the changes are to only the annotations used for propagation, we should not enqueue
	// becase this is going to blast set reconcile as the children's changed
	if newAnnotations != oldAnnotations {
		// check if all other fields are same, except the annotations
		oldCopy := e.ObjectOld.DeepCopyObject().(client.Object)
		oldCopy.SetAnnotations(e.ObjectNew.GetAnnotations())
		return !reflect.DeepEqual(oldCopy, e.ObjectNew)
	}
	return true
}

func handleIgnoreAnnotationPropagation(ctx context.Context, c client.Client, parentObj client.Object, ownedObjects client.ObjectList) error {
	log := ctrl.LoggerFrom(ctx)
	var errs []error
	_ = meta.EachListItem(ownedObjects, func(obj runtime.Object) error {
		childObj, ok := obj.(client.Object)
		if !ok {
			errs = append(errs, fmt.Errorf("item in list is not a client.Object: %T", obj))
			return nil
		}
		// if the child is being deleted, we don't need to propagate
		if !childObj.GetDeletionTimestamp().IsZero() {
			return nil
		}
		opResult, err := controllerutil.CreateOrPatch(ctx, c, childObj, func() error {
			annotations := childObj.GetAnnotations()

			if !shouldChildIgnoreReconciliation(parentObj) && isChildIgnoredThroughSets(childObj) && annotations != nil {
				delete(annotations, metalv1alpha1.OperationAnnotation)
				childObj.SetAnnotations(annotations)
			}
			// should not overwrite the already ignored annotation on child
			// should not overwrite if the annotation already present on the child
			_, OperationAnnotationChildfound := annotations[metalv1alpha1.OperationAnnotation]
			if shouldChildIgnoreReconciliation(parentObj) && !OperationAnnotationChildfound {
				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.OperationAnnotationIgnorePropagated
				childObj.SetAnnotations(annotations)
			}
			return nil
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to propagate ignore annotation to child %s: %w", childObj.GetName(), err))
		}
		if opResult != controllerutil.OperationResultNone {
			log.V(1).Info("Patched Child's annotations for ignore operation", "ChildResource", childObj.GetName(), "Operation", opResult)
		}
		return nil
	})
	return errors.Join(errs...)
}

func handleRetryAnnotationPropagation(ctx context.Context, c client.Client, parentObj client.Object, ownedObjects client.ObjectList) error {
	log := ctrl.LoggerFrom(ctx)
	var errs []error
	_ = meta.EachListItem(ownedObjects, func(obj runtime.Object) error {
		cObj, ok := obj.(client.Object)
		if !ok {
			errs = append(errs, fmt.Errorf("item in list is not a client.Object: %T", obj))
			return nil
		}
		// Always fetch the latest version from the API server
		childObj := cObj.DeepCopyObject().(client.Object)
		err := c.Get(ctx, client.ObjectKeyFromObject(cObj), childObj)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to fetch latest child %s: %w", cObj.GetName(), err))
			return nil
		}
		// if the child is being deleted, we don't need to propagate
		if !childObj.GetDeletionTimestamp().IsZero() {
			return nil
		}
		log.V(1).Info("Child's annotations check", "ChildResource", childObj.GetName())

		opResult, err := controllerutil.CreateOrPatch(ctx, c, childObj, func() error {
			annotations := childObj.GetAnnotations()

			if !shouldChildRetryReconciliation(parentObj) && isChildRetryThroughSets(childObj) && annotations != nil {
				delete(annotations, metalv1alpha1.OperationAnnotation)
				childObj.SetAnnotations(annotations)
			}
			// Use reflection to access the Status.Conditions field, assuming given child objects have it.
			v := reflect.ValueOf(childObj).Elem()
			statusField := v.FieldByName("Status")
			if statusField.IsValid() {
				// If there's no Status field, we can't check conditions we continue.
				conditionsField := statusField.FieldByName("Conditions")
				if conditionsField.IsValid() {
					// Same as above, if there's no Conditions field, we can't check this.
					conditions, ok := conditionsField.Interface().([]metav1.Condition)
					if ok {
						acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
						retriedCondition, err := GetCondition(acc, conditions, ConditionRetryOfFailedResourceIssued)

						if err == nil && retriedCondition != nil &&
							retriedCondition.Status == metav1.ConditionTrue &&
							retriedCondition.Message == metalv1alpha1.OperationAnnotationRetryPropagated {
							// retry was already propagated to child, we can skip re-propagation to avoid infinite loop
							return nil
						}
					}
				}
			}
			// should not overwrite the already present retry annotation on child
			// should not overwrite if the annotation already present on the child
			_, OperationAnnotationChildfound := annotations[metalv1alpha1.OperationAnnotation]
			if shouldChildRetryReconciliation(parentObj) && !OperationAnnotationChildfound {
				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.OperationAnnotationRetryPropagated
				childObj.SetAnnotations(annotations)
			}
			return nil
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to propagate retry annotation to child %s: %w", childObj.GetName(), err))
		}
		if opResult != controllerutil.OperationResultNone {
			log.V(1).Info("Patched Child's annotations to retry annotation", "ChildResource", childObj.GetName(), "Operation", opResult)
		}
		return nil
	})
	return errors.Join(errs...)
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
