// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsAnyServerMaintenanceActive returns true if any of the referenced ServerMaintenance objects
// is currently in the InMaintenance state.
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
