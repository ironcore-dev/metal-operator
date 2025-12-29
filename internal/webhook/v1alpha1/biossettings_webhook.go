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
var settingsLog = logf.Log.WithName("biossettings-resource")

// SetupBIOSSettingsWebhookWithManager registers the webhook for BIOSSettings in the manager.
func SetupBIOSSettingsWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.BIOSSettings{}).
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

var _ webhook.CustomValidator = &BIOSSettingsCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BIOSSettings.
func (v *BIOSSettingsCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	settings, ok := obj.(*metalv1alpha1.BIOSSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSSettings object but got %T", obj)
	}
	settingsLog.Info("Validation for BIOSSettings upon creation", "name", settings.GetName())

	settingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := v.List(ctx, settingsList); err != nil {
		return nil, fmt.Errorf("failed to list BIOSSettings: %w", err)
	}

	return checkForDuplicateBIOSSettingsRefToServer(settingsList, settings)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BIOSSettings.
func (v *BIOSSettingsCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	settings, ok := newObj.(*metalv1alpha1.BIOSSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSSettings object for the newObj but got %T", newObj)
	}
	settingsLog.Info("Validation for BIOSSettings upon update", "name", settings.GetName())

	oldBIOSSettings, ok := oldObj.(*metalv1alpha1.BIOSSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSSettings object for the oldObj but got %T", oldObj)
	}
	// we do not allow Updates to BIOSSettings post server Maintenance creation
	if oldBIOSSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress &&
		!ShouldAllowForceUpdateInProgress(settings) && oldBIOSSettings.Spec.ServerMaintenanceRef != nil {
		err := fmt.Errorf("BIOSSettings (%v) is in progress, unable to update %v",
			oldBIOSSettings.Name,
			settings.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: settings.GroupVersionKind().Group, Kind: settings.Kind},
			settings.GetName(), field.ErrorList{field.Forbidden(field.NewPath("spec"), err.Error())})
	}

	settingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := v.List(ctx, settingsList); err != nil {
		return nil, fmt.Errorf("failed to list BIOSSettings: %w", err)
	}

	return checkForDuplicateBIOSSettingsRefToServer(settingsList, settings)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BIOSSettings.
func (v *BIOSSettingsCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	settings, ok := obj.(*metalv1alpha1.BIOSSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSSettings object but got %T", obj)
	}
	settingsLog.Info("Validation for BIOSSettings upon deletion", "name", settings.GetName())

	if settings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress && !ShouldAllowForceDeleteInProgress(settings) {
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
			err := fmt.Errorf("BMC (%v) referred in %v is duplicate of BMC (%v) referred in %v",
				settings.Spec.ServerRef.Name,
				settings.Name,
				bs.Spec.ServerRef.Name,
				bs.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: settings.GroupVersionKind().Group, Kind: settings.Kind},
				settings.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec", "serverRef"), err)})
		}
	}
	return nil, nil
}
