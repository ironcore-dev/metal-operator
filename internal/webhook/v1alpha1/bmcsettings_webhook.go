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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/controller"
)

// nolint:unused
// log is for logging in this package.
var bmcsettingslog = logf.Log.WithName("bmcsettings-resource")

// SetupBMCSettingsWebhookWithManager registers the webhook for BMCSettings in the manager.
func SetupBMCSettingsWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.BMCSettings{}).
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

var _ webhook.CustomValidator = &BMCSettingsCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmcSettings, ok := obj.(*metalv1alpha1.BMCSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSettings object but got %T", obj)
	}
	bmcsettingslog.Info("Validation for BMCSettings upon creation", "name", bmcSettings.GetName())

	bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
	if err := v.Client.List(ctx, bmcSettingsList); err != nil {
		return nil, fmt.Errorf("failed to list bmcSettingsList: %w", err)
	}
	return checkForDuplicateBMCSettingsRefToBMC(bmcSettingsList, bmcSettings)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	bmcSettings, ok := newObj.(*metalv1alpha1.BMCSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSettings object for the newObj but got %T", newObj)
	}
	bmcsettingslog.Info("Validation for BMCSettings upon update", "name", bmcSettings.GetName())

	oldBMCSettings, ok := oldObj.(*metalv1alpha1.BMCSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSettings object for the oldObj but got %T", oldObj)
	}
	if oldBMCSettings.Status.State == metalv1alpha1.BMCSettingsStateInProgress &&
		!ShouldAllowForceUpdateInProgress(bmcSettings) {
		err := fmt.Errorf("BMCSettings (%v) is in progress, unable to update %v",
			oldBMCSettings.Name,
			bmcSettings.Name)
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: bmcSettings.GroupVersionKind().Group, Kind: bmcSettings.Kind},
			bmcSettings.GetName(), field.ErrorList{field.Forbidden(field.NewPath("spec"), err.Error())})
	}

	bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
	if err := v.Client.List(ctx, bmcSettingsList); err != nil {
		return nil, fmt.Errorf("failed to list bmcSettingsList: %w", err)
	}
	return checkForDuplicateBMCSettingsRefToBMC(bmcSettingsList, bmcSettings)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmcsettings, ok := obj.(*metalv1alpha1.BMCSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSettings object but got %T", obj)
	}
	bmcsettingslog.Info("Validation for BMCSettings upon deletion", "name", bmcsettings.GetName())

	bs := &metalv1alpha1.BMCSettings{}
	err := v.Client.Get(ctx, client.ObjectKey{
		Name:      bmcsettings.GetName(),
		Namespace: bmcsettings.GetNamespace(),
	}, bs)
	if err != nil {
		return nil, fmt.Errorf("failed to get BMCSettings: %w", err)
	}

	if controllerutil.ContainsFinalizer(bs, controller.BMCSettingFinalizer) {
		if bs.Status.State == metalv1alpha1.BMCSettingsStateInProgress {
			return nil, apierrors.NewBadRequest("The BMC settings in progress, unable to delete")
		}
		if bs.Status.State == metalv1alpha1.BMCSettingsStateFailed {
			return nil, apierrors.NewBadRequest("The BMCSsettings has Failed, unable to delete. check server status and retry")
		}
	}

	return nil, nil
}

func checkForDuplicateBMCSettingsRefToBMC(
	bmcSettingsList *metalv1alpha1.BMCSettingsList,
	bmcSettings *metalv1alpha1.BMCSettings,
) (admission.Warnings, error) {
	for _, bs := range bmcSettingsList.Items {
		if bmcSettings.Name == bs.Name {
			continue
		}
		if bs.Spec.BMCRef.Name == bmcSettings.Spec.BMCRef.Name {
			err := fmt.Errorf("BMC (%v) referred in %v is duplicate of BMC (%v) referred in %v",
				bmcSettings.Spec.BMCRef.Name,
				bmcSettings.Name,
				bs.Spec.BMCRef.Name,
				bs.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: bmcSettings.GroupVersionKind().Group, Kind: bmcSettings.Kind},
				bmcSettings.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec", "BMCRef"), err)})
		}
	}
	return nil, nil
}
