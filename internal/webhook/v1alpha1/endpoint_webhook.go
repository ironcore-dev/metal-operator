// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var endpointlog = logf.Log.WithName("endpoint-resource")

// SetupEndpointWebhookWithManager registers the webhook for Endpoint in the manager.
func SetupEndpointWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.Endpoint{}).
		WithValidator(&EndpointCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-endpoint,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=endpoints,verbs=create;update,versions=v1alpha1,name=vendpoint-v1alpha1.kb.io,admissionReviewVersions=v1

// EndpointCustomValidator struct is responsible for validating the Endpoint resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type EndpointCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &EndpointCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Endpoint.
func (v *EndpointCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	allErrs := field.ErrorList{}

	endpoint, ok := obj.(*metalv1alpha1.Endpoint)
	if !ok {
		return nil, fmt.Errorf("expected an Endpoint object but got %T", obj)
	}
	endpointlog.Info("Validation for Endpoint upon creation", "name", endpoint.GetName())

	allErrs = append(allErrs, ValidateMACAddressCreate(ctx, v.Client, endpoint.Spec, field.NewPath("spec"))...)

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "metal.ironcore.dev", Kind: "Endpoint"},
			endpoint.GetName(), allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Endpoint.
func (v *EndpointCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	allErrs := field.ErrorList{}

	endpoint, ok := newObj.(*metalv1alpha1.Endpoint)
	if !ok {
		return nil, fmt.Errorf("expected an Endpoint object for the newObj but got %T", newObj)
	}
	endpointlog.Info("Validation for Endpoint upon update", "name", endpoint.GetName())

	allErrs = append(allErrs, ValidateMACAddressUpdate(ctx, v.Client, endpoint, field.NewPath("spec"))...)

	if len(allErrs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "metal.ironcore.dev", Kind: "Endpoint"},
			endpoint.GetName(), allErrs)
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Endpoint.
func (v *EndpointCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	endpoint, ok := obj.(*metalv1alpha1.Endpoint)
	if !ok {
		return nil, fmt.Errorf("expected an Endpoint object but got %T", obj)
	}
	endpointlog.Info("Validation for Endpoint upon deletion", "name", endpoint.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}

func ValidateMACAddressCreate(ctx context.Context, c client.Client, spec metalv1alpha1.EndpointSpec, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	endpoints := &metalv1alpha1.EndpointList{}
	if err := c.List(ctx, endpoints); err != nil {
		allErrs = append(allErrs, field.InternalError(path, fmt.Errorf("failed to list Endpoints: %w", err)))
	}

	for _, e := range endpoints.Items {
		if e.Spec.MACAddress == spec.MACAddress {
			allErrs = append(allErrs, field.Duplicate(field.NewPath("spec").Child("MACAddress"), e.Spec.MACAddress))
		}
	}

	return allErrs
}

func ValidateMACAddressUpdate(ctx context.Context, c client.Client, updatedEndpoint *metalv1alpha1.Endpoint, path *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	endpoints := &metalv1alpha1.EndpointList{}
	if err := c.List(ctx, endpoints); err != nil {
		allErrs = append(allErrs, field.InternalError(path, fmt.Errorf("failed to list Endpoints: %w", err)))
	}

	for _, e := range endpoints.Items {
		if e.Spec.MACAddress == updatedEndpoint.Spec.MACAddress && e.Name != updatedEndpoint.Name {
			allErrs = append(allErrs, field.Duplicate(field.NewPath("spec").Child("MACAddress"), e.Spec.MACAddress))
		}
	}

	return allErrs
}
