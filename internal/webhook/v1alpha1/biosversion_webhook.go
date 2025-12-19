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
var versionLog = logf.Log.WithName("biosversion-resource")

// SetupBIOSVersionWebhookWithManager registers the webhook for BIOSVersion in the manager.
func SetupBIOSVersionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.BIOSVersion{}).
		WithValidator(&BIOSVersionCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-biosversion,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=biosversions,verbs=create;update;delete,versions=v1alpha1,name=vbiosversion-v1alpha1.kb.io,admissionReviewVersions=v1

// BIOSVersionCustomValidator struct is responsible for validating the BIOSVersion resource
// when it is created, updated, or deleted.
type BIOSVersionCustomValidator struct {
	client.Client
}

var _ webhook.CustomValidator = &BIOSVersionCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BIOSVersion.
func (v *BIOSVersionCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	version, ok := obj.(*metalv1alpha1.BIOSVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSVersion object but got %T", obj)
	}
	versionLog.Info("Validation for BIOSVersion upon creation", "name", version.GetName())

	versions := &metalv1alpha1.BIOSVersionList{}
	if err := v.List(ctx, versions); err != nil {
		return nil, fmt.Errorf("failed to list BIOSVersion: %w", err)
	}
	return checkForDuplicateBIOSVersionRefToServer(versions, version)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BIOSVersion.
func (v *BIOSVersionCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newVersion, ok := newObj.(*metalv1alpha1.BIOSVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSVersion object for the newObj but got %T", newObj)
	}
	versionLog.Info("Validation for BIOSVersion upon update", "name", newVersion.GetName())

	oldVersion, ok := oldObj.(*metalv1alpha1.BIOSVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSVersion object for the oldObj but got %T", oldObj)
	}
	if oldVersion.Status.State == metalv1alpha1.BIOSVersionStateInProgress &&
		!ShouldAllowForceUpdateInProgress(newVersion) && oldVersion.Spec.ServerMaintenanceRef != nil {
		err := fmt.Errorf("BIOSVersion (%v) is in progress, unable to update %v",
			oldVersion.Name,
			newVersion.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: newVersion.GroupVersionKind().Group, Kind: newVersion.Kind},
			newVersion.GetName(), field.ErrorList{field.Forbidden(field.NewPath("spec"), err.Error())})
	}

	versions := &metalv1alpha1.BIOSVersionList{}
	if err := v.List(ctx, versions); err != nil {
		return nil, fmt.Errorf("failed to list BIOSVersion: %w", err)
	}

	return checkForDuplicateBIOSVersionRefToServer(versions, newVersion)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BIOSVersion.
func (v *BIOSVersionCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	version, ok := obj.(*metalv1alpha1.BIOSVersion)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSVersion object but got %T", obj)
	}
	versionLog.Info("Validation for BIOSVersion upon deletion", "name", version.GetName())

	if version.Status.State == metalv1alpha1.BIOSVersionStateInProgress && !ShouldAllowForceDeleteInProgress(version) {
		return nil, apierrors.NewBadRequest("The BIOS version is in progress and cannot be deleted")
	}
	return nil, nil
}

func checkForDuplicateBIOSVersionRefToServer(versions *metalv1alpha1.BIOSVersionList, version *metalv1alpha1.BIOSVersion) (admission.Warnings, error) {
	if version.Spec.ServerRef == nil {
		return nil, nil
	}

	for _, bv := range versions.Items {
		if version.Name == bv.Name {
			continue
		}
		if bv.Spec.ServerRef == nil {
			continue
		}
		if version.Spec.ServerRef.Name == bv.Spec.ServerRef.Name {
			err := fmt.Errorf("server (%s) referred in %s is duplicate of Server (%s) referred in %s",
				version.Spec.ServerRef.Name,
				version.Name,
				bv.Spec.ServerRef.Name,
				bv.Name,
			)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: version.GroupVersionKind().Group, Kind: version.Kind},
				version.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec").Child("serverRef").Child("name"), err)})
		}
	}
	return nil, nil
}
