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
var settingsLog = logf.Log.WithName("biossettings-resource")

// SetupBIOSSettingsWebhookWithManager registers the webhook for BIOSSettings in the manager.
func SetupBIOSSettingsWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalv1alpha1.BIOSSettings{}).
		WithValidator(&BIOSSettingsCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-biossettings,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=biossettings,verbs=create;update;delete,versions=v1alpha1,name=vbiossettings-v1alpha1.kb.io,admissionReviewVersions=v1

// BIOSSettingsCustomValidator struct is responsible for validating the BIOSSettings resource
// when it is created, updated, or deleted.
type BIOSSettingsCustomValidator struct {
	client.Client
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BIOSSettings.
func (v *BIOSSettingsCustomValidator) ValidateCreate(ctx context.Context, obj *metalv1alpha1.BIOSSettings) (admission.Warnings, error) {
	settingsLog.Info("Validation for BIOSSettings upon creation", "name", obj.GetName())

	settingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := v.List(ctx, settingsList); err != nil {
		return nil, fmt.Errorf("failed to list BIOSSettings: %w", err)
	}

	return checkForDuplicateBIOSSettingsRefToServer(settingsList, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BIOSSettings.
func (v *BIOSSettingsCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *metalv1alpha1.BIOSSettings) (admission.Warnings, error) {
	settingsLog.Info("Validation for BIOSSettings upon update", "name", newObj.GetName())

	// we do not allow Updates to BIOSSettings post server Maintenance creation
	if oldObj.Status.State == metalv1alpha1.BIOSSettingsStateInProgress &&
		!ShouldAllowForceUpdateInProgress(newObj) && oldObj.Spec.ServerMaintenanceRef != nil {
		err := fmt.Errorf("BIOSSettings (%s) is in progress, unable to update %s", oldObj.Name, newObj.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: newObj.GroupVersionKind().Group, Kind: newObj.Kind},
			newObj.GetName(), field.ErrorList{field.Forbidden(field.NewPath("spec"), err.Error())})
	}

	settingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := v.List(ctx, settingsList); err != nil {
		return nil, fmt.Errorf("failed to list BIOSSettings: %w", err)
	}

	return checkForDuplicateBIOSSettingsRefToServer(settingsList, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BIOSSettings.
func (v *BIOSSettingsCustomValidator) ValidateDelete(_ context.Context, obj *metalv1alpha1.BIOSSettings) (admission.Warnings, error) {
	settingsLog.Info("Validation for BIOSSettings upon deletion", "name", obj.GetName())

	if obj.Status.State == metalv1alpha1.BIOSSettingsStateInProgress && !ShouldAllowForceDeleteInProgress(obj) {
		return nil, apierrors.NewBadRequest("The bios settings in progress, unable to delete")
	}

	return nil, nil
}

func checkForDuplicateBIOSSettingsRefToServer(settingsList *metalv1alpha1.BIOSSettingsList, settings *metalv1alpha1.BIOSSettings) (admission.Warnings, error) {
	for _, bs := range settingsList.Items {
		if settings.Name == bs.Name {
			continue
		}
		if settings.Spec.ServerRef.Name == bs.Spec.ServerRef.Name {
			err := fmt.Errorf("server (%s) referred in %s is duplicate of server (%s) referred in %s",
				settings.Spec.ServerRef.Name,
				settings.Name,
				bs.Spec.ServerRef.Name,
				bs.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: settings.GroupVersionKind().Group, Kind: settings.Kind},
				settings.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec").Child("serverRef"), err)})
		}
	}
	return nil, nil
}
