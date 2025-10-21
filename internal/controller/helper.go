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

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const (
	fieldOwner = client.FieldOwner("metal.ironcore.dev/controller-manager")
)

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
	return val == metalv1alpha1.OperationAnnotationRetryFailed || val == metalv1alpha1.OperationAnnotationRetryFailedPropagated
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
	return valChildRetry == metalv1alpha1.OperationAnnotationRetryFailedPropagated
}

// GenerateRandomPassword generates a random password of the given length.
func GenerateRandomPassword(length int) ([]byte, error) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return nil, fmt.Errorf("failed to generate random password: %w", err)
		}
		result[i] = letters[n.Int64()]
	}
	return result, nil
}

func enqueFromChildObjUpdatesExceptAnnotation(e event.UpdateEvent) bool {
	isNil := func(arg any) bool {
		if v := reflect.ValueOf(arg); !v.IsValid() || ((v.Kind() == reflect.Ptr ||
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

func handleIgnoreAnnotationPropagation(
	ctx context.Context,
	log logr.Logger,
	kClient client.Client,
	parentObj client.Object,
	ownedObjects client.ObjectList,
) error {
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
		opResult, err := controllerutil.CreateOrPatch(ctx, kClient, childObj, func() error {
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

func handleRetryAnnotationPropagation(
	ctx context.Context,
	log logr.Logger,
	kClient client.Client,
	parentObj client.Object,
	ownedObjects client.ObjectList,
) error {
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
		log.V(1).Info("Child's annotations check", "ChildResource", childObj.GetName())

		opResult, err := controllerutil.CreateOrPatch(ctx, kClient, childObj, func() error {
			annotations := childObj.GetAnnotations()

			if !shouldChildRetryReconciliation(parentObj) && isChildRetryThroughSets(childObj) && annotations != nil {
				delete(annotations, metalv1alpha1.OperationAnnotation)
				childObj.SetAnnotations(annotations)
			}
			// should not overwrite the already present retry annotation on child
			// should not overwrite if the annotation already present on the child
			_, OperationAnnotationChildfound := annotations[metalv1alpha1.OperationAnnotation]
			if shouldChildRetryReconciliation(parentObj) && !OperationAnnotationChildfound {
				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.OperationAnnotationRetryFailedPropagated
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
