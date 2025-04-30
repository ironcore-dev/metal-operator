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
var bmcsettingslog = logf.Log.WithName("bmcsettings-resource")

// SetupBMCSettingsWebhookWithManager registers the webhook for BMCSettings in the manager.
func SetupBMCSettingsWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.BMCSettings{}).
		WithValidator(&BMCSettingsCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-bmcsettings,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=bmcsettings,verbs=create;update,versions=v1alpha1,name=vbmcsettings-v1alpha1.kb.io,admissionReviewVersions=v1

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

	if bmcSettings.Spec.BMCRef == nil && bmcSettings.Spec.ServerRefList == nil {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: bmcSettings.GroupVersionKind().Group, Kind: bmcSettings.Kind},
			bmcSettings.GetName(), field.ErrorList{field.Required(field.NewPath("spec"), "Spec.BMCRef or Spec.ServerRefList is required")})
	}

	bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
	if err := v.Client.List(ctx, bmcSettingsList); err != nil {
		return nil, fmt.Errorf("failed to list bmcSettingsList: %w", err)
	}

	// this make one API call rather than multiple when trying to find duplicates
	serversList := &metalv1alpha1.ServerList{}
	if err := v.Client.List(ctx, serversList); err != nil {
		return nil, fmt.Errorf("failed to list serversList: %w", err)
	}
	serversMap := make(map[string]*metalv1alpha1.Server, len(serversList.Items))
	for _, server := range serversList.Items {
		serversMap[server.Name] = &server
	}

	var bmcSettingsBMCName string
	var path string
	var bsBMCName string
	var err error
	// get the intended BMC
	if bmcSettings.Spec.BMCRef != nil {
		bmcSettingsBMCName = bmcSettings.Spec.BMCRef.Name
		path = "Spec.BMCRef"
	} else {
		bmcSettingsBMCName, err = getBMCNameFromServerRef(serversMap, bmcSettings)
		if err != nil {
			return nil, err
		}
		path = "Spec.ServerRefList"
	}

	bmcsettingslog.Info("TEMP:bmcSettings", "bmcSettings name", bmcSettings.Name, "bmcSettings BMCRef", bmcSettings.Spec.BMCRef, "bmcSettings ServerList", bmcSettings.Spec.ServerRefList)

	for _, bs := range bmcSettingsList.Items {
		bmcsettingslog.Info("TEMP:bs ", "bs name", bs.Name, "bs BMCRef", bs.Spec.BMCRef, "bs ServerList", bs.Spec.ServerRefList)
		if bs.Spec.BMCRef != nil {
			bsBMCName = bs.Spec.BMCRef.Name
		} else {
			bsBMCName, err = getBMCNameFromServerRef(serversMap, &bs)
			if err != nil {
				bmcsettingslog.Info("Skipping as no referred BMC was found", "BMCSettings", bs.Name, "error", err)
				continue
			}
		}
		if bsBMCName == bmcSettingsBMCName {
			err = fmt.Errorf("BMC (%v) referred in %v is duplicate of BMC (%v) referred in %v", bmcSettingsBMCName, bmcSettings.Name, bsBMCName, bs.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: bmcSettings.GroupVersionKind().Group, Kind: bmcSettings.Kind},
				bmcSettings.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec", path), err)})
		}
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	bmcSettings, ok := newObj.(*metalv1alpha1.BMCSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSettings object for the newObj but got %T", newObj)
	}
	bmcsettingslog.Info("Validation for BMCSettings upon update", "name", bmcSettings.GetName())

	if bmcSettings.Spec.BMCRef == nil && bmcSettings.Spec.ServerRefList == nil {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: bmcSettings.GroupVersionKind().Group, Kind: bmcSettings.Kind},
			bmcSettings.GetName(), field.ErrorList{field.Required(field.NewPath("spec"), "Spec.BMCRef or Spec.ServerRefList is required")})
	}

	bmcSettingsList := &metalv1alpha1.BMCSettingsList{}
	if err := v.Client.List(ctx, bmcSettingsList); err != nil {
		return nil, fmt.Errorf("failed to list bmcSettingsList: %w", err)
	}

	// this make one API call rather than multiple when trying to find duplicates
	serversList := &metalv1alpha1.ServerList{}
	if err := v.Client.List(ctx, serversList); err != nil {
		return nil, fmt.Errorf("failed to list serversList: %w", err)
	}
	serversMap := make(map[string]*metalv1alpha1.Server, len(serversList.Items))
	for _, server := range serversList.Items {
		serversMap[server.Name] = &server
	}

	var bmcSettingsBMCName string
	var path string
	var bsBMCName string
	var err error

	// get the intended BMC
	if bmcSettings.Spec.BMCRef != nil {
		bmcSettingsBMCName = bmcSettings.Spec.BMCRef.Name
		path = "Spec.BMCRef"
	} else {
		bmcSettingsBMCName, err = getBMCNameFromServerRef(serversMap, bmcSettings)
		if err != nil {
			return nil, err
		}
		path = "Spec.ServerRefList"
	}

	bmcsettingslog.Info("TEMP:bmcSettings", "bmcSettings name", bmcSettings.Name, "bmcSettings BMCRef", bmcSettings.Spec.BMCRef, "bmcSettings ServerList", bmcSettings.Spec.ServerRefList)

	for _, bs := range bmcSettingsList.Items {
		if bmcSettings.Name == bs.Name {
			continue
		}
		bmcsettingslog.Info("TEMP: bs", "bs name", bs.Name, "bs BMCRef", bs.Spec.BMCRef, "bs ServerList", bs.Spec.ServerRefList)
		if bs.Spec.BMCRef != nil {
			bsBMCName = bs.Spec.BMCRef.Name
		} else {
			bsBMCName, err = getBMCNameFromServerRef(serversMap, &bs)
			if err != nil {
				bmcsettingslog.Info("Skipping as no referred BMC was found", "BMCSettings", bs.Name, "error", err)
				continue
			}
		}
		if bsBMCName == bmcSettingsBMCName {
			err = fmt.Errorf("BMC (%v) referred in %v is duplicate of BMC (%v) referred in %v", bmcSettingsBMCName, bmcSettings.Name, bsBMCName, bs.Name)
			return nil, apierrors.NewInvalid(
				schema.GroupKind{Group: bmcSettings.GroupVersionKind().Group, Kind: bmcSettings.Kind},
				bmcSettings.GetName(), field.ErrorList{field.Duplicate(field.NewPath("spec", path), err)})
		}
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCSettings.
func (v *BMCSettingsCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmcsettings, ok := obj.(*metalv1alpha1.BMCSettings)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSettings object but got %T", obj)
	}
	bmcsettingslog.Info("Validation for BMCSettings upon deletion", "name", bmcsettings.GetName())

	return nil, nil
}

func getBMCNameFromServerRef(serversMap map[string]*metalv1alpha1.Server, bmcSettings *metalv1alpha1.BMCSettings) (string, error) {
	for _, serverRef := range bmcSettings.Spec.ServerRefList {
		bmcsettingslog.Info("TEMP: Validation ", "serverRef", serverRef, "serversMap[serverRef.Name]", serversMap)
		if server, ok := serversMap[serverRef.Name]; ok && server != nil && server.Spec.BMCRef != nil {
			return server.Spec.BMCRef.Name, nil
		}
	}
	return "", fmt.Errorf("no servers found with reference to BMC in given 'ServerRefList' %v", bmcSettings.Spec.ServerRefList)
}
