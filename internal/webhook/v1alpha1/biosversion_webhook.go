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

// nolint:unused
// log is for logging in this package.
var versionLog = logf.Log.WithName("biosversion-resource")

// SetupBIOSVersionWebhookWithManager registers the webhook for BIOSVersion in the manager.
func SetupBIOSVersionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalv1alpha1.BIOSVersion{}).
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

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BIOSVersion.
func (v *BIOSVersionCustomValidator) ValidateCreate(ctx context.Context, obj *metalv1alpha1.BIOSVersion) (admission.Warnings, error) {
	versionLog.Info("Validation for BIOSVersion upon creation", "name", obj.GetName())

	versions := &metalv1alpha1.BIOSVersionList{}
	if err := v.List(ctx, versions); err != nil {
		return nil, fmt.Errorf("failed to list BIOSVersion: %w", err)
	}
	return checkForDuplicateBIOSVersionRefToServer(versions, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BIOSVersion.
func (v *BIOSVersionCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *metalv1alpha1.BIOSVersion) (admission.Warnings, error) {
	versionLog.Info("Validation for BIOSVersion upon update", "name", newObj.GetName())

	if oldObj.Status.State == metalv1alpha1.BIOSVersionStateInProgress &&
		!ShouldAllowForceUpdateInProgress(newObj) && oldObj.Spec.ServerMaintenanceRef != nil {
		err := fmt.Errorf("BIOSVersion (%v) is in progress, unable to update %v",
			oldObj.Name,
			newObj.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: newObj.GroupVersionKind().Group, Kind: newObj.Kind},
			newObj.GetName(), field.ErrorList{field.Forbidden(field.NewPath("spec"), err.Error())})
	}

	versions := &metalv1alpha1.BIOSVersionList{}
	if err := v.List(ctx, versions); err != nil {
		return nil, fmt.Errorf("failed to list BIOSVersion: %w", err)
	}

	return checkForDuplicateBIOSVersionRefToServer(versions, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BIOSVersion.
func (v *BIOSVersionCustomValidator) ValidateDelete(ctx context.Context, obj *metalv1alpha1.BIOSVersion) (admission.Warnings, error) {
	versionLog.Info("Validation for BIOSVersion upon deletion", "name", obj.GetName())

	if obj.Status.State == metalv1alpha1.BIOSVersionStateInProgress && !ShouldAllowForceDeleteInProgress(obj) {
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
			err := fmt.Errorf("server (%s) referred in %s is duplicate of server (%s) referred in %s",
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
