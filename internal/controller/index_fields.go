// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ServerSystemUUIDIndexField = "spec.systemUUID"
	// ServerMaintenanceServerRefIndex is the index for the server reference in the ServerMaintenance resource.
	ServerMaintenanceServerRefIndex = "spec.serverRef"
)

// RegisterIndexFields registers index fields.
func RegisterIndexFields(ctx context.Context, indexer client.FieldIndexer) (errs error) {
	if err := indexer.IndexField(
		ctx,
		&metalv1alpha1.ServerMaintenance{},
		ServerMaintenanceServerRefIndex,
		func(rawObj client.Object) []string {
			maintenance := rawObj.(*metalv1alpha1.ServerMaintenance)
			if maintenance.Spec.ServerRef != nil {
				return []string{maintenance.Spec.ServerRef.Name}
			}
			return nil
		},
	); err != nil {
		errs = errors.Join(errs, err)
	}
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
		errs = errors.Join(errs, err)
	}
	return errs
}
