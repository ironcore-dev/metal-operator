// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"reflect"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var bmcsecretlog = logf.Log.WithName("bmcsecret-resource")

// SetupBMCSecretWebhookWithManager registers the webhook for BMCSecret in the manager.
func SetupBMCSecretWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalv1alpha1.BMCSecret{}).
		WithValidator(&BMCSecretCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-bmcsecret,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=bmcsecrets,verbs=create;update;delete,versions=v1alpha1,name=vbmcsecret-v1alpha1.kb.io,admissionReviewVersions=v1

type BMCSecretCustomValidator struct {
	Client client.Client
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BMCSecret.
func (v *BMCSecretCustomValidator) ValidateCreate(_ context.Context, obj *metalv1alpha1.BMCSecret) (admission.Warnings, error) {
	bmcsecretlog.Info("Validation for BMCSecret upon creation", "name", obj.GetName())

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCSecret.
func (v *BMCSecretCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *metalv1alpha1.BMCSecret) (admission.Warnings, error) {
	bmcsecretlog.Info("Validation for BMCSecret upon update", "name", newObj.GetName())

	if oldObj.Immutable != nil && *oldObj.Immutable {
		if newObj.Immutable == nil || !*newObj.Immutable {
			return nil, fmt.Errorf("immutable field cannot be changed from true to false")
		}
		if !reflect.DeepEqual(newObj.Data, oldObj.Data) {
			return nil, fmt.Errorf("data field is immutable and cannot be updated")
		}
		if !reflect.DeepEqual(newObj.StringData, oldObj.StringData) {
			return nil, fmt.Errorf("stringData field is immutable and cannot be updated")
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCSecret.
func (v *BMCSecretCustomValidator) ValidateDelete(_ context.Context, obj *metalv1alpha1.BMCSecret) (admission.Warnings, error) {
	bmcsecretlog.Info("Validation for BMCSecret upon deletion", "name", obj.GetName())

	return nil, nil
}
