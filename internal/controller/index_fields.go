package controller

import (
	"context"
	"errors"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// RegisterIndexFields registers index fields.
func RegisterIndexFields(ctx context.Context, mgr manager.Manager) (errs error) {
	indexer := mgr.GetFieldIndexer()
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
	return errs
}
