// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var bmclog = logf.Log.WithName("bmc-resource")

// SetupBMCWebhookWithManager registers the webhook for BMC in the manager.
func SetupBMCWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.BMC{}).
		WithValidator(&BMCCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-bmc,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=bmcs,verbs=create;update,versions=v1alpha1,name=vbmc-v1alpha1.kb.io,admissionReviewVersions=v1

// BMCCustomValidator struct is responsible for validating the BMC resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type BMCCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &BMCCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BMC.
func (v *BMCCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmc, ok := obj.(*metalv1alpha1.BMC)
	if !ok {
		return nil, fmt.Errorf("expected a BMC object but got %T", obj)
	}
	bmclog.Info("Validation for BMC upon creation", "name", bmc.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMC.
func (v *BMCCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	bmc, ok := newObj.(*metalv1alpha1.BMC)
	if !ok {
		return nil, fmt.Errorf("expected a BMC object for the newObj but got %T", newObj)
	}
	bmclog.Info("Validation for BMC upon update", "name", bmc.GetName())
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMC.
func (v *BMCCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmc, ok := obj.(*metalv1alpha1.BMC)
	if !ok {
		return nil, fmt.Errorf("expected a BMC object but got %T", obj)
	}
	bmclog.Info("Validation for BMC upon deletion", "name", bmc.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
