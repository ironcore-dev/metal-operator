// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var serverclaimlog = logf.Log.WithName("serverclaim-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *ServerClaim) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-serverclaim,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=serverclaims,verbs=create;update,versions=v1alpha1,name=vserverclaim.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &ServerClaim{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ServerClaim) ValidateCreate() (admission.Warnings, error) {
	serverclaimlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ServerClaim) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	serverclaimlog.Info("validate update", "name", r.Name)
	allErrs := field.ErrorList{}
	oldClaim := old.(*ServerClaim)

	allErrs = append(allErrs, ValidateUpdateSpecUpdate(r.Spec, oldClaim.Spec, field.NewPath("spec"))...)

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "metal.ironcore.dev", Kind: "ServerClaim"},
			r.Name, allErrs)
	}

	return nil, nil
}

func ValidateUpdateSpecUpdate(newSpec, oldSpec ServerClaimSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if oldSpec.ServerRef != nil {
		allErrs = append(allErrs, validation.ValidateImmutableField(newSpec.ServerRef, oldSpec.ServerRef, fldPath.Child("serverRef"))...)
	}
	if oldSpec.ServerSelector != nil {
		allErrs = append(allErrs, validation.ValidateImmutableField(newSpec.ServerSelector, oldSpec.ServerSelector, fldPath.Child("serverSelector"))...)
	}

	return allErrs
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ServerClaim) ValidateDelete() (admission.Warnings, error) {
	serverclaimlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
