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
	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/stmcginnis/gofish/redfish"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const (
	fieldOwner = client.FieldOwner("metal.ironcore.dev/controller-manager")
)

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

// GetCondition finds a condition in a condition slice.
func GetCondition(acc *conditionutils.Accessor, conditions []metav1.Condition, conditionType string) (*metav1.Condition, error) {
	condition := &metav1.Condition{}
	condFound, err := acc.FindSlice(conditions, conditionType, condition)

	if err != nil {
		return nil, fmt.Errorf("failed to find Condition %v. error: %v", conditionType, err)
	}
	if !condFound {
		condition.Type = conditionType
		if err := acc.Update(
			condition,
			conditionutils.UpdateStatus(corev1.ConditionFalse),
		); err != nil {
			return condition, fmt.Errorf("failed to create/update new Condition %v. error: %v", conditionType, err)
		}
	}

	return condition, nil
}

// GetServerByName returns a Server object by its name or an error in case the object can not be found.
func GetServerByName(ctx context.Context, c client.Client, serverName string) (*metalv1alpha1.Server, error) {
	server := &metalv1alpha1.Server{}
	if err := c.Get(ctx, client.ObjectKey{Name: serverName}, server); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("server not found")
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

// truncateString truncates a string to the specified maximum length.
func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength]
}

func enqueueFromChildObjUpdatesExceptAnnotation(e event.UpdateEvent) bool {
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
				if op == metalv1alpha1.GracefulRestartBMC {
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
		annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.GracefulRestartBMC
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

func handleIgnoreAnnotationPropagation(ctx context.Context, log logr.Logger, c client.Client, parentObj client.Object, ownedObjects client.ObjectList) error {
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

func handleRetryAnnotationPropagation(ctx context.Context, log logr.Logger, c client.Client, parentObj client.Object, ownedObjects client.ObjectList) error {
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

		opResult, err := controllerutil.CreateOrPatch(ctx, c, childObj, func() error {
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
