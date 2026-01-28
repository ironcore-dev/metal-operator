// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var bmcsecretlog = logf.Log.WithName("bmcsecret-resource")

// SetupBMCSecretWebhookWithManager registers the webhook for BMCSecret in the manager.
func SetupBMCSecretWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.BMCSecret{}).
		WithValidator(&BMCSecretCustomValidator{}).
		Complete()
}

type BMCSecretCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &BMCSecretCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type BMCSecret.
func (v *BMCSecretCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmcsecret, ok := obj.(*metalv1alpha1.BMCSecret)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSecret object but got %T", obj)
	}
	bmcsecretlog.Info("Validation for BMCSecret upon creation", "name", bmcsecret.GetName())

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type BMCSecret.
func (v *BMCSecretCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	bmcsecret, ok := newObj.(*metalv1alpha1.BMCSecret)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSecret object for the newObj but got %T", newObj)
	}
	oldSecret, ok := oldObj.(*metalv1alpha1.BMCSecret)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSecret object for the oldObj but got %T", oldObj)
	}
	bmcsecretlog.Info("Validation for BMCSecret upon update", "name", bmcsecret.GetName())

	if oldSecret.Immutable != nil && *oldSecret.Immutable {
		if bmcsecret.Immutable == nil || !*bmcsecret.Immutable {
			return nil, fmt.Errorf("immutable field cannot be changed from true to false")
		}
		if !reflect.DeepEqual(bmcsecret.Data, oldSecret.Data) {
			return nil, fmt.Errorf("data field is immutable and cannot be updated")
		}
		if !reflect.DeepEqual(bmcsecret.StringData, oldSecret.StringData) {
			return nil, fmt.Errorf("stringData field is immutable and cannot be updated")
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type BMCSecret.
func (v *BMCSecretCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bmcsecret, ok := obj.(*metalv1alpha1.BMCSecret)
	if !ok {
		return nil, fmt.Errorf("expected a BMCSecret object but got %T", obj)
	}
	bmcsecretlog.Info("Validation for BMCSecret upon deletion", "name", bmcsecret.GetName())

	return nil, nil
}
