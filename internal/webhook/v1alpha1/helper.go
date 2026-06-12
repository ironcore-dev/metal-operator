// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsAnyServerMaintenanceActive returns true if any of the referenced ServerMaintenance
// objects has State == InMaintenance. References whose objects are not found or are
// being deleted are skipped (treated as inactive).
func IsAnyServerMaintenanceActive(ctx context.Context, c client.Client, refs []metalv1alpha1.ObjectReference) (bool, error) {
	for _, ref := range refs {
		sm := &metalv1alpha1.ServerMaintenance{}
		if err := c.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}, sm); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, fmt.Errorf("failed to get ServerMaintenance %s/%s: %w", ref.Namespace, ref.Name, err)
		}
		if !sm.DeletionTimestamp.IsZero() {
			continue
		}
		if sm.Status.State == metalv1alpha1.ServerMaintenanceStateInMaintenance {
			return true, nil
		}
	}
	return false, nil
}

// ShouldAllowForceUpdateInProgress checks if the object should force allow update.
func ShouldAllowForceUpdateInProgress(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationForceUpdateInProgress || val == metalv1alpha1.OperationAnnotationForceUpdateOrDeleteInProgress
}

// ShouldAllowForceDeleteInProgress checks if the object be allowed to be force deleted.
func ShouldAllowForceDeleteInProgress(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationForceUpdateOrDeleteInProgress
}
