// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ServerSystemUUIDIndexField = "spec.systemUUID"
)

func RegisterIndexFields(ctx context.Context, indexer client.FieldIndexer) error {
	if err := indexer.IndexField(ctx, &metalv1alpha1.Server{}, ServerSystemUUIDIndexField, func(rawObj client.Object) []string {
		server, ok := rawObj.(*metalv1alpha1.Server)
		if !ok {
			return nil
		}
		if server.Spec.SystemUUID == "" {
			return nil
		}
		return []string{server.Spec.SystemUUID}
	}); err != nil {
		return err
	}
	return nil
}
