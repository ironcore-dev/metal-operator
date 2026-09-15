// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// log is for logging in this package.
var serverlog = logf.Log.WithName("server-resource")

// SetupServerWebhookWithManager registers the webhook for Server in the manager.
func SetupServerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &metalv1alpha1.Server{}).
<<<<<<< HEAD
		WithValidator(&ServerValidator{}).
=======
		WithValidator(&ServerCustomValidator{Client: mgr.GetClient()}).
>>>>>>> tmp-original-15-09-26-00-47
		Complete()
}

// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-server,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=servers,verbs=delete,versions=v1alpha1,name=vserver-v1alpha1.kb.io,admissionReviewVersions=v1

// ServerValidator struct is responsible for validating the Server resource
// when it is created, updated, or deleted.
<<<<<<< HEAD
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ServerValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type Server.
func (v *ServerValidator) ValidateCreate(_ context.Context, obj *metalv1alpha1.Server) (admission.Warnings, error) {
	serverlog.Info("Validation for Server upon creation", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements admission.Validator so a webhook will be registered for the type Server.
func (v *ServerValidator) ValidateUpdate(_ context.Context, oldObj, newObj *metalv1alpha1.Server) (admission.Warnings, error) {
	serverlog.Info("Validation for Server upon update", "name", newObj.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements admission.Validator so a webhook will be registered for the type Server.
func (v *ServerValidator) ValidateDelete(_ context.Context, obj *metalv1alpha1.Server) (admission.Warnings, error) {
=======
type ServerCustomValidator struct {
	Client client.Client
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Server.
func (v *ServerCustomValidator) ValidateCreate(ctx context.Context, obj *metalv1alpha1.Server) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Server.
func (v *ServerCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *metalv1alpha1.Server) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Server.
func (v *ServerCustomValidator) ValidateDelete(ctx context.Context, obj *metalv1alpha1.Server) (admission.Warnings, error) {
>>>>>>> tmp-original-15-09-26-00-47
	serverlog.Info("Validation for Server upon deletion", "name", obj.GetName())
	return nil, nil
}
