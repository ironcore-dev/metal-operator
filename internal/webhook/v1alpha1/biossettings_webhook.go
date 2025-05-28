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
var biossettingslog = logf.Log.WithName("biossettings-resource")

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
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type BIOSSettingsCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &BIOSSettingsCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BIOSSettings.
func (v *BIOSSettingsCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	biossettings, ok := obj.(*metalv1alpha1.BIOSSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSSettings object but got %T", obj)
	}
	biossettingslog.Info("Validation for BIOSSettings upon creation", "name", biossettings.GetName())

	biosSettingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := v.Client.List(ctx, biosSettingsList); err != nil {
		return nil, fmt.Errorf("failed to list BIOSSettingsList: %w", err)
	}

	return checkForDuplicateBiosSettingsRefToServer(biosSettingsList, biossettings)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BIOSSettings.
func (v *BIOSSettingsCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	biossettings, ok := newObj.(*metalv1alpha1.BIOSSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSSettings object for the newObj but got %T", newObj)
	}
	biossettingslog.Info("Validation for BIOSSettings upon update", "name", biossettings.GetName())

	biosSettingsList := &metalv1alpha1.BIOSSettingsList{}
	if err := v.Client.List(ctx, biosSettingsList); err != nil {
		return nil, fmt.Errorf("failed to list BIOSSettingsList: %w", err)
	}

	return checkForDuplicateBiosSettingsRefToServer(biosSettingsList, biossettings)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BIOSSettings.
func (v *BIOSSettingsCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	biossettings, ok := obj.(*metalv1alpha1.BIOSSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BIOSSettings object but got %T", obj)
	}
	biossettingslog.Info("Validation for BIOSSettings upon deletion", "name", biossettings.GetName())

	if biossettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
		return nil, apierrors.NewBadRequest("The bios settings in progress, unable to delete")
	}

	return nil, nil
}

func checkForDuplicateBiosSettingsRefToServer(
	biosSettingsList *metalv1alpha1.BIOSSettingsList,
	biosSettings *metalv1alpha1.BIOSSettings,
) (admission.Warnings, error) {

	for _, bs := range biosSettingsList.Items {
		if biosSettings.Name == bs.Name {
			continue
		}
		if biosSettings.Spec.ServerRef.Name == bs.Spec.ServerRef.Name && biosSettings.Name != bs.Name {
			err := fmt.Errorf("BMC (%v) referred in %v is duplicate of BMC (%v) referred in %v",
				biosSettings.Spec.ServerRef.Name,
				biosSettings.Name,
				bs.Spec.ServerRef.Name,
				bs.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: biosSettings.GroupVersionKind().Group, Kind: biosSettings.Kind},
				biosSettings.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec", "ServerRef"), err)})
		}
	}
	return nil, nil
}
