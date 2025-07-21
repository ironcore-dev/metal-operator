// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"fmt"

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
var serverlog = logf.Log.WithName("server-resource")

// SetupServerWebhookWithManager registers the webhook for Server in the manager.
func SetupServerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&metalv1alpha1.Server{}).
		WithValidator(&ServerCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-metal-ironcore-dev-v1alpha1-server,mutating=false,failurePolicy=fail,sideEffects=None,groups=metal.ironcore.dev,resources=servers,verbs=delete,versions=v1alpha1,name=vserver-v1alpha1.kb.io,admissionReviewVersions=v1

// ServerCustomValidator struct is responsible for validating the Server resource
// when it is created, updated, or deleted.
type ServerCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &ServerCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Server.
func (v *ServerCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Server.
func (v *ServerCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Server.
func (v *ServerCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	server, ok := obj.(*metalv1alpha1.Server)
	if !ok {
		return nil, fmt.Errorf("expected a Server object but got %T", obj)
	}
	serverlog.Info("Validation for Server upon deletion", "name", server.GetName())

	if server.Status.State == metalv1alpha1.ServerStateMaintenance && !ShouldAllowForceDeleteInProgress(server) {
		return nil, fmt.Errorf("cannot delete Server %s in state %s", server.GetName(), server.Status.State)
	}

	return nil, nil
}
