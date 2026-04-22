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

// validateBMCUserExpirationSpec validates TTL and ExpiresAt fields.
// Returns error if validation fails.
func validateBMCUserExpirationSpec(spec *metalv1alpha1.BMCUserSpec) error {
	// Validate TTL and ExpiresAt are mutually exclusive
	if spec.TTL != nil && spec.ExpiresAt != nil {
		return fmt.Errorf("spec.ttl and spec.expiresAt are mutually exclusive")
	}

	// Validate ExpiresAt is in the future
	if spec.ExpiresAt != nil {
		if spec.ExpiresAt.Before(&metav1.Time{Time: time.Now()}) {
			return fmt.Errorf("spec.expiresAt must be in the future")
		}
	}

	// Validate TTL is reasonable (> 0, <= 1 week)
	if spec.TTL != nil {
		if spec.TTL.Duration <= 0 {
			return fmt.Errorf("spec.ttl must be positive")
		}
		if spec.TTL.Duration > 168*time.Hour { // 1 week
			return fmt.Errorf("spec.ttl exceeds maximum of 1 week")
		}
	}

	return nil
}

// ttlChanged returns true if TTL field changed between old and new objects.
func ttlChanged(oldTTL, newTTL *metav1.Duration) bool {
	if oldTTL == nil && newTTL == nil {
		return false
	}
	if oldTTL == nil || newTTL == nil {
		return true // one is nil, other isn't
	}
	return oldTTL.Duration != newTTL.Duration
}

// expiresAtChanged returns true if ExpiresAt field changed between old and new objects.
func expiresAtChanged(oldTime, newTime *metav1.Time) bool {
	if oldTime == nil && newTime == nil {
		return false
	}
	if oldTime == nil || newTime == nil {
		return true // one is nil, other isn't
	}
	return !oldTime.Equal(newTime)
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BMCUser.
func (v *BMCUserCustomValidator) ValidateCreate(_ context.Context, obj *metalv1alpha1.BMCUser) (admission.Warnings, error) {
	bmcuserlog.Info("Validation for BMCUser upon creation", "name", obj.GetName())
	return nil, validateBMCUserExpirationSpec(&obj.Spec)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCUser.
func (v *BMCUserCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *metalv1alpha1.BMCUser) (admission.Warnings, error) {
	bmcuserlog.Info("Validation for BMCUser upon update", "name", newObj.GetName())

	var warnings admission.Warnings

	// Apply same validation as create
	if err := validateBMCUserExpirationSpec(&newObj.Spec); err != nil {
		return warnings, err
	}

	// Warn if TTL/ExpiresAt changed after expiration was calculated
	if oldObj.Status.ExpiresAt != nil {
		if ttlChanged(oldObj.Spec.TTL, newObj.Spec.TTL) {
			warnings = append(warnings,
				"TTL was changed but expiration time is already set and will not be recalculated")
		}
		if expiresAtChanged(oldObj.Spec.ExpiresAt, newObj.Spec.ExpiresAt) {
			warnings = append(warnings,
				"ExpiresAt was changed but expiration time is already calculated and will not be updated")
		}
	}

	return warnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCUser.
func (v *BMCUserCustomValidator) ValidateDelete(_ context.Context, obj *metalv1alpha1.BMCUser) (admission.Warnings, error) {
	bmcuserlog.Info("Validation for BMCUser upon deletion", "name", obj.GetName())
	return nil, nil
}
