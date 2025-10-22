// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"reflect"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/stmcginnis/gofish/redfish"
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

func resetBMCOfServer(
	ctx context.Context,
	log logr.Logger,
	kClient client.Client,
	server *metalv1alpha1.Server,
	bmcClient bmc.BMC,
) error {
	if server.Spec.BMCRef != nil {
		key := client.ObjectKey{Name: server.Spec.BMCRef.Name}
		BMC := &metalv1alpha1.BMC{}
		if err := kClient.Get(ctx, key, BMC); err != nil {
			log.V(1).Error(err, "failed to get referred server's Manager")
			return err
		}
		annotations := BMC.GetAnnotations()
		if annotations != nil {
			if op, ok := annotations[metalv1alpha1.OperationAnnotation]; ok {
				if op == metalv1alpha1.OperationAnnotationForceReset {
					log.V(1).Info("Waiting for BMC reset as annotation on BMC object is set")
					return nil
				} else {
					return fmt.Errorf("unknown annotation on BMC object for operation annotation %v", op)
				}
			}
		}
		log.V(1).Info("Setting annotation on BMC resource to trigger with BMC reset")

		BMCBase := BMC.DeepCopy()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.OperationAnnotationForceReset
		BMC.SetAnnotations(annotations)
		if err := kClient.Patch(ctx, BMC, client.MergeFrom(BMCBase)); err != nil {
			return err
		}
		return nil
	} else if server.Spec.BMC != nil {
		// no BMC ref, but BMC details are inline in server spec
		// we can directly reset BMC in this case, so just proceed
		// as we have the BMCclient, get the 1st manager and reset it
		bmc, err := bmcClient.GetManager("")
		if err != nil {
			return fmt.Errorf("failed to get manager to reset BMC: %w", err)
		}
		log.V(1).Info("Resetting through redfish to stabilize BMC of the server")
		err = bmcClient.ResetManager(ctx, bmc.ID, redfish.GracefulRestartResetType)
		if err != nil {
			return fmt.Errorf("failed to get manager to reset BMC: %w", err)
		}
		return nil
	}
	return fmt.Errorf("no BMC reference or inline BMC details found in server spec to reset BMC")
}
