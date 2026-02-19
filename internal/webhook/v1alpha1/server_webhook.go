// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

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
		WithValidator(&ServerCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-server,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=servers,verbs=delete,versions=v1alpha1,name=vserver-v1alpha1.kb.io,admissionReviewVersions=v1

// ServerCustomValidator struct is responsible for validating the Server resource
// when it is created, updated, or deleted.
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
	serverlog.Info("Validation for Server upon deletion", "name", obj.GetName())

	if obj.Status.State == metalv1alpha1.ServerStateMaintenance && !ShouldAllowForceDeleteInProgress(obj) {
		return nil, fmt.Errorf("cannot delete Server %s in state %s", obj.GetName(), obj.Status.State)
	}
	return nil, nil
}
