// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var bmcversionlog = logf.Log.WithName("bmcversion-resource")

// SetupBMCVersionWebhookWithManager registers the webhook for BMCVersion in the manager.
func SetupBMCVersionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.BMCVersion{}).
		WithValidator(&BMCVersionCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-bmcversion,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=bmcversions,verbs=create;update;delete,versions=v1alpha1,name=vbmcversion-v1alpha1.kb.io,admissionReviewVersions=v1

// BMCVersionCustomValidator struct is responsible for validating the BMCVersion resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type BMCVersionCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &BMCVersionCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BMCVersion.
func (v *BMCVersionCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmcversion, ok := obj.(*metalv1alpha1.BMCVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BMCVersion object but got %T", obj)
	}
	bmcversionlog.Info("Validation for BMCVersion upon creation", "name", bmcversion.GetName())
	bmcVersionList := &metalv1alpha1.BMCVersionList{}
	if err := v.Client.List(ctx, bmcVersionList); err != nil {
		return nil, fmt.Errorf("failed to list BMCVersionList: %w", err)
	}
	return checkForDuplicateBMCVersionsRefToBMC(bmcVersionList, bmcversion)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCVersion.
func (v *BMCVersionCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	bmcversion, ok := newObj.(*metalv1alpha1.BMCVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BMCVersion object for the newObj but got %T", newObj)
	}
	bmcversionlog.Info("Validation for BMCVersion upon update", "name", bmcversion.GetName())

	bmcVersionList := &metalv1alpha1.BMCVersionList{}
	if err := v.Client.List(ctx, bmcVersionList); err != nil {
		return nil, fmt.Errorf("failed to list BMCVersionList: %w", err)
	}
	return checkForDuplicateBMCVersionsRefToBMC(bmcVersionList, bmcversion)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCVersion.
func (v *BMCVersionCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmcversion, ok := obj.(*metalv1alpha1.BMCVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BMCVersion object but got %T", obj)
	}
	bmcversionlog.Info("Validation for BMCVersion upon deletion", "name", bmcversion.GetName())

	if bmcversion.Status.State == metalv1alpha1.BMCVersionStateInProgress {
		return nil, apierrors.NewBadRequest("The BMC settings in progress, unable to delete")
	}

	return nil, nil
}

func checkForDuplicateBMCVersionsRefToBMC(
	bmcVersionList *metalv1alpha1.BMCVersionList,
	bmcVersion *metalv1alpha1.BMCVersion,
) (admission.Warnings, error) {
	for _, bv := range bmcVersionList.Items {
		if bmcVersion.Name == bv.Name {
			continue
		}
		if bv.Spec.BMCRef.Name == bmcVersion.Spec.BMCRef.Name && bmcVersion.Name != bv.Name {
			err := fmt.Errorf("BMC (%v) referred in %v is duplicate of BMC (%v) referred in %v",
				bmcVersion.Spec.BMCRef.Name,
				bmcVersion.Name,
				bv.Spec.BMCRef.Name,
				bv.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: bmcVersion.GroupVersionKind().Group, Kind: bmcVersion.Kind},
				bmcVersion.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec", "BMCRef"), err)})
		}
	}
	return nil, nil
}
