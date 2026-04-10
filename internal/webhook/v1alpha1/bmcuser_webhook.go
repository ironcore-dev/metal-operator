// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var bmcuserlog = logf.Log.WithName("bmcuser-resource")

// SetupBMCUserWebhookWithManager registers the webhook for BMCUser in the manager.
func SetupBMCUserWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalv1alpha1.BMCUser{}).
		WithValidator(&BMCUserCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-bmcuser,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=bmcusers,verbs=create;update,versions=v1alpha1,name=vbmcuser-v1alpha1.kb.io,admissionReviewVersions=v1

type BMCUserCustomValidator struct {
	Client client.Client
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BMCUser.
func (v *BMCUserCustomValidator) ValidateCreate(_ context.Context, obj *metalv1alpha1.BMCUser) (admission.Warnings, error) {
	bmcuserlog.Info("Validation for BMCUser upon creation", "name", obj.GetName())

	// Validate TTL and ExpiresAt are mutually exclusive
	if obj.Spec.TTL != nil && obj.Spec.ExpiresAt != nil {
		return nil, fmt.Errorf("spec.ttl and spec.expiresAt are mutually exclusive")
	}

	// Validate ExpiresAt is in the future
	if obj.Spec.ExpiresAt != nil {
		if obj.Spec.ExpiresAt.Before(&metav1.Time{Time: time.Now()}) {
			return nil, fmt.Errorf("spec.expiresAt must be in the future")
		}
	}

	// Validate TTL is reasonable (> 0, < 10 years)
	if obj.Spec.TTL != nil {
		if obj.Spec.TTL.Duration <= 0 {
			return nil, fmt.Errorf("spec.ttl must be positive")
		}
		if obj.Spec.TTL.Duration > 87600*time.Hour { // 10 years
			return nil, fmt.Errorf("spec.ttl exceeds maximum of 10 years")
		}
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCUser.
func (v *BMCUserCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *metalv1alpha1.BMCUser) (admission.Warnings, error) {
	bmcuserlog.Info("Validation for BMCUser upon update", "name", newObj.GetName())

	var warnings admission.Warnings

	// Validate TTL and ExpiresAt are mutually exclusive
	if newObj.Spec.TTL != nil && newObj.Spec.ExpiresAt != nil {
		return warnings, fmt.Errorf("spec.ttl and spec.expiresAt are mutually exclusive")
	}

	// Warn if TTL/ExpiresAt changed after expiration was calculated
	if oldObj.Status.ExpiresAt != nil {
		if oldObj.Spec.TTL != nil && newObj.Spec.TTL != nil {
			if oldObj.Spec.TTL.Duration != newObj.Spec.TTL.Duration {
				warnings = append(warnings,
					"TTL was changed but expiration time is already set and will not be recalculated")
			}
		}
		if oldObj.Spec.ExpiresAt != nil && newObj.Spec.ExpiresAt != nil {
			if !oldObj.Spec.ExpiresAt.Equal(newObj.Spec.ExpiresAt) {
				warnings = append(warnings,
					"ExpiresAt was changed but expiration time is already calculated and will not be updated")
			}
		}
	}

	return warnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCUser.
func (v *BMCUserCustomValidator) ValidateDelete(_ context.Context, obj *metalv1alpha1.BMCUser) (admission.Warnings, error) {
	bmcuserlog.Info("Validation for BMCUser upon deletion", "name", obj.GetName())
	return nil, nil
}
