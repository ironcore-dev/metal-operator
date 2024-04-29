/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/ironcore-dev/controller-utils/clientutils"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ServerBootConfigurationFinalizer = "metal.ironcore.dev/serverbootconfiguration"
)

// ServerBootConfigurationReconciler reconciles a ServerBootConfiguration object
type ServerBootConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerBootConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bootConfig := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, bootConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, bootConfig)
}

func (r *ServerBootConfigurationReconciler) reconcileExists(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	if !config.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, config)
	}
	return r.reconcile(ctx, log, config)
}

func (r *ServerBootConfigurationReconciler) delete(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	log.V(1).Info("Deleting ServerBootConfiguration")

	if err := r.removeServerBootConfigRef(ctx, config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove ServerBootConfigRef from server: %w", err)
	}
	log.V(1).Info("Ensured no server boot config is set on server")

	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, config, ServerBootConfigurationFinalizer); !apierrors.IsNotFound(err) || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured that the finalizer has been removed")

	log.V(1).Info("Deleted ServerBootConfiguration")
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationReconciler) reconcile(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerBootConfiguration")
	if config.Status.State == "" {
		if modified, err := r.patchState(ctx, config, metalv1alpha1.ServerBootConfigurationStatePending); err != nil || modified {
			return ctrl.Result{}, err
		}
	}
	log.V(1).Info("Patched state")

	if modified, err := clientutils.PatchEnsureFinalizer(ctx, r.Client, config, ServerBootConfigurationFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Ensured finalizer has been added")

	if err := r.patchServerBootConfigRef(ctx, config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch server boot config ref: %w", err)
	}
	log.V(1).Info("Patched server boot config ref")

	log.V(1).Info("Reconciled ServerBootConfiguration")
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationReconciler) patchState(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration, state metalv1alpha1.ServerBootConfigurationState) (bool, error) {
	if config.Status.State == state {
		return false, nil
	}
	configBase := config.DeepCopy()
	config.Status.State = state
	if err := r.Status().Patch(ctx, config, client.MergeFrom(configBase)); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ServerBootConfigurationReconciler) patchServerBootConfigRef(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) error {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return err
	}

	serverBase := server.DeepCopy()
	server.Spec.BootConfigurationRef = &v1.ObjectReference{
		Namespace:  config.Namespace,
		Name:       config.Name,
		UID:        config.UID,
		APIVersion: "metal.ironcore.dev/v1alpha1",
		Kind:       "ServerBootConfiguration",
	}
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return err
	}

	return nil
}

func (r *ServerBootConfigurationReconciler) removeServerBootConfigRef(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) error {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		if apierrors.IsNotFound(err) {
			// server is gone
			return nil
		}
		return err
	}

	serverBase := server.DeepCopy()
	server.Spec.BootConfigurationRef = nil
	if err := r.Patch(ctx, server, client.MergeFrom(serverBase)); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBootConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Watches(&metalv1alpha1.Server{}, r.enqueueServerBootConfigByServerRef()).
		Complete(r)
}

func (r *ServerBootConfigurationReconciler) enqueueServerBootConfigByServerRef() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		server := obj.(*metalv1alpha1.Server)
		if server.Spec.BootConfigurationRef != nil {
			return []ctrl.Request{
				{
					NamespacedName: types.NamespacedName{Namespace: server.Spec.BootConfigurationRef.Namespace, Name: server.Spec.BootConfigurationRef.Name},
				},
			}
		}
		return nil
	})
}
