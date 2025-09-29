// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"reflect"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	return val == metalv1alpha1.OperationAnnotationIgnore || val == metalv1alpha1.OperationAnnotationIgnoreChildAndSelf
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
	valPropagated, found := annotations[metalv1alpha1.PropagatedOperationAnnotation]
	if !found {
		return false
	}
	valChildIgnore, found := annotations[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return valChildIgnore == metalv1alpha1.OperationAnnotationIgnore && valPropagated == metalv1alpha1.OperationAnnotationIgnoreChild
}

// shouldRetryReconciliation checks if the object should retry reconciliation from failed state.
func shouldRetryReconciliation(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationRetry
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

// truncateString truncates a string to the specified maximum length.
func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength]
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
