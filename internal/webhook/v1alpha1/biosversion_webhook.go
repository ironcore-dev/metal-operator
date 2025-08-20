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
var biosversionlog = logf.Log.WithName("biosversion-resource")

// SetupBIOSVersionWebhookWithManager registers the webhook for BIOSVersion in the manager.
func SetupBIOSVersionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.BIOSVersion{}).
		WithValidator(&BIOSVersionCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-biosversion,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=biosversions,verbs=create;update;delete,versions=v1alpha1,name=vbiosversion-v1alpha1.kb.io,admissionReviewVersions=v1

// BIOSVersionCustomValidator struct is responsible for validating the BIOSVersion resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type BIOSVersionCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &BIOSVersionCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BIOSVersion.
func (v *BIOSVersionCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	biosversion, ok := obj.(*metalv1alpha1.BIOSVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSVersion object but got %T", obj)
	}
	biosversionlog.Info("Validation for BIOSVersion upon creation", "name", biosversion.GetName())

	biosVersionList := &metalv1alpha1.BIOSVersionList{}
	if err := v.Client.List(ctx, biosVersionList); err != nil {
		return nil, fmt.Errorf("failed to list BIOSVersionList: %w", err)
	}
	return checkForDuplicateBIOSVersionRefToServer(biosVersionList, biosversion)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BIOSVersion.
func (v *BIOSVersionCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	biosversion, ok := newObj.(*metalv1alpha1.BIOSVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSVersion object for the newObj but got %T", newObj)
	}
	biosversionlog.Info("Validation for BIOSVersion upon update", "name", biosversion.GetName())

	oldBIOSVersion, ok := oldObj.(*metalv1alpha1.BIOSVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSVersion object for the oldObj but got %T", oldObj)
	}
	if oldBIOSVersion.Status.State == metalv1alpha1.BIOSVersionStateInProgress &&
		!ShouldAllowForceUpdateInProgress(biosversion) && oldBIOSVersion.Spec.ServerMaintenanceRef != nil {
		err := fmt.Errorf("BIOSVersion (%v) is in progress, unable to update %v",
			oldBIOSVersion.Name,
			biosversion.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: biosversion.GroupVersionKind().Group, Kind: biosversion.Kind},
			biosversion.GetName(), field.ErrorList{field.Forbidden(field.NewPath("spec"), err.Error())})
	}

	biosVersionList := &metalv1alpha1.BIOSVersionList{}
	if err := v.Client.List(ctx, biosVersionList); err != nil {
		return nil, fmt.Errorf("failed to list BIOSVersionList: %w", err)
	}

	return checkForDuplicateBIOSVersionRefToServer(biosVersionList, biosversion)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BIOSVersion.
func (v *BIOSVersionCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	biosversion, ok := obj.(*metalv1alpha1.BIOSVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSVersion object but got %T", obj)
	}
	biosversionlog.Info("Validation for BIOSVersion upon deletion", "name", biosversion.GetName())

	if biosversion.Status.State == metalv1alpha1.BIOSVersionStateInProgress && !ShouldAllowForceDeleteInProgress(biosversion) {
		return nil, apierrors.NewBadRequest("The bios version in progress, unable to delete")
	}
	return nil, nil
}

func checkForDuplicateBIOSVersionRefToServer(
	biosVersionList *metalv1alpha1.BIOSVersionList,
	biosVersion *metalv1alpha1.BIOSVersion,
) (admission.Warnings, error) {
	for _, bv := range biosVersionList.Items {
		if biosVersion.Name == bv.Name {
			continue
		}
		if biosVersion.Spec.ServerRef.Name == bv.Spec.ServerRef.Name {
			err := fmt.Errorf("server (%v) referred in %v is duplicate of Server (%v) referred in %v",
				biosVersion.Spec.ServerRef,
				biosVersion.Name,
				bv.Spec.ServerRef,
				bv.Name,
			)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: biosVersion.GroupVersionKind().Group, Kind: biosVersion.Kind},
				biosVersion.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec").Child("ServerRef").Child("Name"), err)})
		}
	}
	return nil, nil
}
