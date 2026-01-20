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

// log is for logging in this package.
var bmcversionlog = logf.Log.WithName("bmcversion-resource")

// SetupBMCVersionWebhookWithManager registers the webhook for BMCVersion in the manager.
func SetupBMCVersionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalv1alpha1.BMCVersion{}).
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

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BMCVersion.
func (v *BMCVersionCustomValidator) ValidateCreate(ctx context.Context, obj *metalv1alpha1.BMCVersion) (admission.Warnings, error) {
	bmcversionlog.Info("Validation for BMCVersion upon creation", "name", obj.GetName())
	bmcVersionList := &metalv1alpha1.BMCVersionList{}
	if err := v.Client.List(ctx, bmcVersionList); err != nil {
		return nil, fmt.Errorf("failed to list BMCVersions: %w", err)
	}
	return checkForDuplicateBMCVersionsRefToBMC(bmcVersionList, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCVersion.
func (v *BMCVersionCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *metalv1alpha1.BMCVersion) (admission.Warnings, error) {
	bmcversionlog.Info("Validation for BMCVersion upon update", "name", newObj.GetName())

	if oldObj.Status.State == metalv1alpha1.BMCVersionStateInProgress &&
		!ShouldAllowForceUpdateInProgress(newObj) && oldObj.Spec.ServerMaintenanceRefs != nil {
		err := fmt.Errorf("BMCVersion (%v) is in progress, unable to update %v",
			oldObj.Name,
			newObj.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: newObj.GroupVersionKind().Group, Kind: newObj.Kind},
			newObj.GetName(), field.ErrorList{field.Forbidden(field.NewPath("spec"), err.Error())})
	}

	bmcVersionList := &metalv1alpha1.BMCVersionList{}
	if err := v.Client.List(ctx, bmcVersionList); err != nil {
		return nil, fmt.Errorf("failed to list BMCVersions: %w", err)
	}

	return checkForDuplicateBMCVersionsRefToBMC(bmcVersionList, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCVersion.
func (v *BMCVersionCustomValidator) ValidateDelete(ctx context.Context, obj *metalv1alpha1.BMCVersion) (admission.Warnings, error) {
	bmcversionlog.Info("Validation for BMCVersion upon deletion", "name", obj.GetName())

	bv := &metalv1alpha1.BMCVersion{}
	err := v.Client.Get(ctx, client.ObjectKey{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, bv)
	if err != nil {
		return nil, fmt.Errorf("failed to get BMCVersion: %w", err)
	}

	if bv.Status.State == metalv1alpha1.BMCVersionStateInProgress && !ShouldAllowForceDeleteInProgress(obj) {
		return nil, apierrors.NewBadRequest("Unable to delete BMCVersion as it is in progress")
	}

	return nil, nil
}

func checkForDuplicateBMCVersionsRefToBMC(versionList *metalv1alpha1.BMCVersionList, version *metalv1alpha1.BMCVersion) (admission.Warnings, error) {
	for _, v := range versionList.Items {
		if version.Name == v.Name {
			continue
		}
		if v.Spec.BMCRef.Name == version.Spec.BMCRef.Name {
			err := fmt.Errorf("BMC (%s) referred in %s is duplicate of BMC (%s) referred in %s",
				version.Spec.BMCRef.Name,
				version.Name,
				v.Spec.BMCRef.Name,
				v.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: version.GroupVersionKind().Group, Kind: version.Kind},
				version.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec").Child("bmcRef"), err)})
		}
	}
	return nil, nil
}
