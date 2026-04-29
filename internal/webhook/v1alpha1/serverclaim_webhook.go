// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var serverclaimlog = logf.Log.WithName("serverclaim-resource")

// SetupServerClaimWebhookWithManager registers the webhook for ServerClaim in the manager.
func SetupServerClaimWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalv1alpha1.ServerClaim{}).
		WithValidator(&ServerClaimCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-serverclaim,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=serverclaims,verbs=create;update,versions=v1alpha1,name=vserverclaim-v1alpha1.kb.io,admissionReviewVersions=v1

// ServerClaimCustomValidator validates ServerClaim resources on create and update.
type ServerClaimCustomValidator struct {
	Client client.Client
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ServerClaim.
func (v *ServerClaimCustomValidator) ValidateCreate(ctx context.Context, obj *metalv1alpha1.ServerClaim) (admission.Warnings, error) {
	serverclaimlog.Info("Validation for ServerClaim upon creation", "name", obj.GetName())
	return nil, validateNoDeprecatedApprovalKey(obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ServerClaim.
func (v *ServerClaimCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *metalv1alpha1.ServerClaim) (admission.Warnings, error) {
	serverclaimlog.Info("Validation for ServerClaim upon update", "name", newObj.GetName())
	return nil, validateNoDeprecatedApprovalKey(newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ServerClaim.
func (v *ServerClaimCustomValidator) ValidateDelete(_ context.Context, _ *metalv1alpha1.ServerClaim) (admission.Warnings, error) {
	return nil, nil
}

// validateNoDeprecatedApprovalKey rejects any ServerClaim that carries the deprecated
// maintenance-approval key in its annotations or labels. Consumers must use
// ServerMaintenanceApprovedLabelKey ("metal.ironcore.dev/maintenance-approved") instead.
func validateNoDeprecatedApprovalKey(obj *metalv1alpha1.ServerClaim) error {
	deprecatedMsg := fmt.Sprintf(
		"deprecated maintenance approval key %q; please migrate to %q",
		metalv1alpha1.ServerMaintenanceApprovalKey,
		metalv1alpha1.ServerMaintenanceApprovedLabelKey,
	)

	var errs field.ErrorList
	if _, ok := obj.Annotations[metalv1alpha1.ServerMaintenanceApprovalKey]; ok {
		errs = append(errs, field.Forbidden(
			field.NewPath("metadata", "annotations").Key(metalv1alpha1.ServerMaintenanceApprovalKey),
			deprecatedMsg,
		))
	}
	if _, ok := obj.Labels[metalv1alpha1.ServerMaintenanceApprovalKey]; ok {
		errs = append(errs, field.Forbidden(
			field.NewPath("metadata", "labels").Key(metalv1alpha1.ServerMaintenanceApprovalKey),
			deprecatedMsg,
		))
	}
	if len(errs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: obj.GroupVersionKind().Group, Kind: obj.Kind},
		obj.GetName(),
		errs,
	)
}
