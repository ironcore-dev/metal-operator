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
	metalutil "github.com/ironcore-dev/metal-operator/internal/util"
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
	if errs := validateDriftPolicy(obj, obj.Spec.DriftPolicy); len(errs) > 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: obj.GroupVersionKind().Group, Kind: obj.Kind}, obj.GetName(), errs)
	}
	list := &metalv1alpha1.BMCSettingsList{}
	if err := v.Client.List(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to list BMCSettings: %w", err)
	}
	return nil, checkDuplicateBMCSettings(list, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *metalv1alpha1.BMCSettings) (admission.Warnings, error) {
	bmcsettingslog.Info("Validation for BMCSettings upon update", "name", newObj.GetName())

	// Block updates while any referenced ServerMaintenance is InMaintenance.
	if !ShouldAllowForceUpdateInProgress(newObj) && len(oldObj.Spec.ServerMaintenanceRefs) > 0 {
		refs := make([]metalv1alpha1.ObjectReference, 0, len(oldObj.Spec.ServerMaintenanceRefs))
		for _, item := range oldObj.Spec.ServerMaintenanceRefs {
			if item.ServerMaintenanceRef != nil {
				refs = append(refs, *item.ServerMaintenanceRef)
			}
		}
		active, err := metalutil.IsAnyServerMaintenanceActive(ctx, v.Client, refs)
		if err != nil {
			return nil, fmt.Errorf("failed to check maintenance state: %w", err)
		}
		if active {
			msg := fmt.Errorf("BMCSettings %s is under active maintenance, unable to update", oldObj.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: newObj.GroupVersionKind().Group, Kind: oldObj.Kind},
				newObj.GetName(), field.ErrorList{field.Forbidden(field.NewPath("spec"), msg.Error())})
		}
	}

	if errs := validateDriftPolicy(newObj, newObj.Spec.DriftPolicy); len(errs) > 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: newObj.GroupVersionKind().Group, Kind: newObj.Kind}, newObj.GetName(), errs)
	}

	list := &metalv1alpha1.BMCSettingsList{}
	if err := v.Client.List(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to list BMCSettings: %w", err)
	}
	return nil, checkDuplicateBMCSettings(list, newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateDelete(ctx context.Context, obj *metalv1alpha1.BMCSettings) (admission.Warnings, error) {
	bmcsettingslog.Info("Validation for BMCSettings upon deletion", "name", obj.GetName())

	// Block deletion while any referenced ServerMaintenance is InMaintenance.
	if !ShouldAllowForceDeleteInProgress(obj) && len(obj.Spec.ServerMaintenanceRefs) > 0 {
		refs := make([]metalv1alpha1.ObjectReference, 0, len(obj.Spec.ServerMaintenanceRefs))
		for _, item := range obj.Spec.ServerMaintenanceRefs {
			if item.ServerMaintenanceRef != nil {
				refs = append(refs, *item.ServerMaintenanceRef)
			}
		}
		active, err := metalutil.IsAnyServerMaintenanceActive(ctx, v.Client, refs)
		if err != nil {
			return nil, fmt.Errorf("failed to check maintenance state: %w", err)
		}
		if active {
			return nil, apierrors.NewBadRequest("BMCSettings is under active maintenance, unable to delete")
		}
	}

	return nil, nil
}
