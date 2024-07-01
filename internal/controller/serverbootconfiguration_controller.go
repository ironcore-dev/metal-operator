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

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBootConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Complete(r)
}
