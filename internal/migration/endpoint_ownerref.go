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

// EndpointOwnerReferenceMigration removes legacy Endpoint owner references from all BMCs and BMCSecrets.
type EndpointOwnerReferenceMigration struct {
	client.Client
}

func (m *EndpointOwnerReferenceMigration) Migrate(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	bmcList := &metalv1alpha1.BMCList{}
	if err := m.List(ctx, bmcList); err != nil {
		return fmt.Errorf("failed to list BMCs: %w", err)
	}
	bmcSecretList := &metalv1alpha1.BMCSecretList{}
	if err := m.List(ctx, bmcSecretList); err != nil {
		return fmt.Errorf("failed to list BMCSecrets: %w", err)
	}

	objs := make([]client.Object, 0, len(bmcList.Items)+len(bmcSecretList.Items))
	for i := range bmcList.Items {
		objs = append(objs, &bmcList.Items[i])
	}
	for i := range bmcSecretList.Items {
		objs = append(objs, &bmcSecretList.Items[i])
	}

	var errs []error
	migrated := 0
	for _, obj := range objs {
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := m.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
				return err
			}
			base := obj.DeepCopyObject().(client.Object)
			obj.SetOwnerReferences(slices.DeleteFunc(obj.GetOwnerReferences(), func(ref metav1.OwnerReference) bool {
				return ref.APIVersion == metalv1alpha1.GroupVersion.String() && ref.Kind == "Endpoint"
			}))
			if len(obj.GetOwnerReferences()) == len(base.GetOwnerReferences()) {
				return nil
			}
			if err := m.Patch(ctx, obj, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
				return err
			}
			migrated++
			return nil
		}); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove Endpoint owner reference from %T %s: %w", obj, obj.GetName(), err))
		}
	}

	log.Info("Removed legacy Endpoint owner references from BMCs and BMCSecrets", "Migrated", migrated, "Total", len(objs))
	return errors.Join(errs...)
}
