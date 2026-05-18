// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// -------------------------
// Setup webhook registration
// -------------------------

func SetupServerMaintenanceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalv1alpha1.ServerMaintenance{}).
		WithValidator(&ServerMaintenanceValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-servermaintenance,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=servermaintenances,verbs=create;update,versions=v1alpha1,name=vservermaintenance-v1alpha1.kb.io,admissionReviewVersions=v1

// -------------------------
// Validator struct
// -------------------------

type ServerMaintenanceValidator struct {
	Client client.Client
}

// -------------------------
// CREATE validation
// -------------------------

func (v *ServerMaintenanceValidator) ValidateCreate(
	ctx context.Context,
	obj *metalv1alpha1.ServerMaintenance,
) (admission.Warnings, error) {

	// 1. ServerRef must exist
	if obj.Spec.ServerRef == nil || obj.Spec.ServerRef.Name == "" {
		return nil, fmt.Errorf("serverRef.name must be set")
	}

	// 2. Check that referenced Server exists
	server := &metalv1alpha1.Server{}

	err := v.Client.Get(ctx, client.ObjectKey{
    Name: obj.Spec.ServerRef.Name,
	}, server)


	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("referenced Server %q does not exist", obj.Spec.ServerRef.Name)
		}
		return nil, err
	}

	return nil, nil
}

// -------------------------
// UPDATE validation
// -------------------------

func (v *ServerMaintenanceValidator) ValidateUpdate(
	ctx context.Context,
	oldObj, newObj *metalv1alpha1.ServerMaintenance,
) (admission.Warnings, error) {

	// reuse create logic
	return v.ValidateCreate(ctx, newObj)
}

// -------------------------
// DELETE validation
// -------------------------

func (v *ServerMaintenanceValidator) ValidateDelete(
	ctx context.Context,
	obj *metalv1alpha1.ServerMaintenance,
) (admission.Warnings, error) {

	// usually allowed
	return nil, nil
}
