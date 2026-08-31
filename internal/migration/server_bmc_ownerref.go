// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"context"
	"errors"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// ServerBMCOwnerReferenceMigration removes legacy BMC owner references from all Servers.
type ServerBMCOwnerReferenceMigration struct {
	client.Client
}

func (m *ServerBMCOwnerReferenceMigration) Migrate(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	serverList := &metalv1alpha1.ServerList{}
	if err := m.List(ctx, serverList); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	var errs []error
	migrated := 0
	for i := range serverList.Items {
		server := &serverList.Items[i]
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := m.Get(ctx, client.ObjectKeyFromObject(server), server); err != nil {
				return err
			}
			base := server.DeepCopy()
			server.OwnerReferences = slices.DeleteFunc(server.OwnerReferences, func(ref metav1.OwnerReference) bool {
				return ref.APIVersion == metalv1alpha1.GroupVersion.String() && ref.Kind == "BMC"
			})
			if len(server.OwnerReferences) == len(base.OwnerReferences) {
				return nil
			}
			if err := m.Patch(ctx, server, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
				return err
			}
			migrated++
			return nil
		}); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove BMC owner reference from server %s: %w", server.Name, err))
		}
	}

	log.Info("Removed legacy BMC owner references from Servers", "Migrated", migrated, "Total", len(serverList.Items))
	return errors.Join(errs...)
}
