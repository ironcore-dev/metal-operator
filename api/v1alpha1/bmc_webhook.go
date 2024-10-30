// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var bmclog = logf.Log.WithName("bmc-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *BMC) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-bmc,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=bmcs,verbs=create;update,versions=v1alpha1,name=vbmc.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &BMC{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *BMC) ValidateCreate() (admission.Warnings, error) {
	bmclog.Info("validate create", "name", r.Name)

	allErrs := field.ErrorList{}
	allErrs = append(allErrs, ValidateCreateBMCSpec(r.Spec, field.NewPath("spec"))...)

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "metal.ironcore.dev", Kind: "BMC"},
			r.Name, allErrs)
	}

	return nil, nil
}

func ValidateCreateBMCSpec(spec BMCSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if spec.EndpointRef != nil && spec.Endpoint != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("endpointRef"), spec.EndpointRef, "only one of 'endpointRef' or 'endpoint' should be specified"))
		allErrs = append(allErrs, field.Invalid(fldPath.Child("endpoint"), spec.Endpoint, "only one of 'endpointRef' or 'endpoint' should be specified"))
	}
	if spec.EndpointRef == nil && spec.Endpoint == nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("endpointRef"), spec.EndpointRef, "either 'endpointRef' or 'endpoint' must be specified"))
		allErrs = append(allErrs, field.Invalid(fldPath.Child("endpoint"), spec.Endpoint, "either 'endpointRef' or 'endpoint' must be specified"))
	}

	return allErrs
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *BMC) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	bmclog.Info("validate update", "name", r.Name)
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, ValidateCreateBMCSpec(r.Spec, field.NewPath("spec"))...)

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "metal.ironcore.dev", Kind: "BMC"},
			r.Name, allErrs)
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *BMC) ValidateDelete() (admission.Warnings, error) {
	bmclog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
