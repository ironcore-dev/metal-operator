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
var bmcsettingslog = logf.Log.WithName("bmcsettings-resource")

// SetupBMCSettingsWebhookWithManager registers the webhook for BMCSettings in the manager.
func SetupBMCSettingsWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalv1alpha1.BMCSettings{}).
		WithValidator(&BMCSettingsCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-bmcsettings,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=bmcsettings,verbs=create;update;delete,versions=v1alpha1,name=vbmcsettings-v1alpha1.kb.io,admissionReviewVersions=v1

// BMCSettingsCustomValidator struct is responsible for validating the BMCSettings resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type BMCSettingsCustomValidator struct {
	Client client.Client
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateCreate(ctx context.Context, obj *metalv1alpha1.BMCSettings) (admission.Warnings, error) {
	bmcsettingslog.Info("Validation for BMCSettings upon creation", "name", obj.GetName())

	bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
	if err := v.Client.List(ctx, bmcSettingsList); err != nil {
		return nil, fmt.Errorf("failed to list BMCSettings: %w", err)
	}
	return checkForDuplicateBMCSettingsRefToBMC(bmcSettingsList, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *metalv1alpha1.BMCSettings) (admission.Warnings, error) {
	bmcsettingslog.Info("Validation for BMCSettings upon update", "name", newObj.GetName())

	if oldObj.Status.State == metalv1alpha1.BMCSettingsStateInProgress &&
		!ShouldAllowForceUpdateInProgress(newObj) && oldObj.Spec.ServerMaintenanceRefs != nil {
		err := fmt.Errorf("BMCSettings (%s) is in progress, unable to update %s",
			oldObj.Name,
			newObj.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: newObj.GroupVersionKind().Group, Kind: oldObj.Kind},
			newObj.GetName(), field.ErrorList{field.Forbidden(field.NewPath("spec"), err.Error())})
	}

	bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
	if err := v.Client.List(ctx, bmcSettingsList); err != nil {
		return nil, fmt.Errorf("failed to list BMCSettings: %w", err)
	}
	return checkForDuplicateBMCSettingsRefToBMC(bmcSettingsList, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateDelete(ctx context.Context, obj *metalv1alpha1.BMCSettings) (admission.Warnings, error) {
	bmcsettingslog.Info("Validation for BMCSettings upon deletion", "name", obj.GetName())

	if obj.Status.State == metalv1alpha1.BMCSettingsStateInProgress && !ShouldAllowForceDeleteInProgress(obj) {
		return nil, apierrors.NewBadRequest("The BMC settings in progress, unable to delete")
	}

	return nil, nil
}

func checkForDuplicateBMCSettingsRefToBMC(settingsList *metalv1alpha1.BMCSettingsList, settings *metalv1alpha1.BMCSettings) (admission.Warnings, error) {
	for _, bs := range settingsList.Items {
		if settings.Name == bs.Name {
			continue
		}
		if bs.Spec.BMCRef.Name == settings.Spec.BMCRef.Name {
			err := fmt.Errorf("BMC (%s) referred in %s is duplicate of BMC (%s) referred in %s",
				settings.Spec.BMCRef.Name,
				settings.Name,
				bs.Spec.BMCRef.Name,
				bs.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: settings.GroupVersionKind().Group, Kind: settings.Kind},
				settings.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec").Child("bmcRef"), err)})
		}
	}
	return nil, nil
}
