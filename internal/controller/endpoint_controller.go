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
	"encoding/base64"
	"fmt"
	"strings"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	"github.com/afritzler/metal-operator/bmc"
	"github.com/afritzler/metal-operator/internal/api/macdb"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	BMCType           = "bmc"
	ProtocolRedfish   = "Redfish"
	EndpointFinalizer = "metal.ironcore.dev/endpoint"
)

// EndpointReconciler reconciles a Endpoints object
type EndpointReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	MACPrefixes *macdb.MacPrefixes
	Insecure    bool
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bmcsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=endpoints/finalizers,verbs=update

func (r *EndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	endpoint := &metalv1alpha1.Endpoint{}
	if err := r.Get(ctx, req.NamespacedName, endpoint); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, endpoint)
}

func (r *EndpointReconciler) reconcileExists(ctx context.Context, log logr.Logger, endpoint *metalv1alpha1.Endpoint) (ctrl.Result, error) {
	if !endpoint.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, endpoint)
	}
	return r.reconcile(ctx, log, endpoint)
}

func (r *EndpointReconciler) delete(ctx context.Context, log logr.Logger, endpoint *metalv1alpha1.Endpoint) (ctrl.Result, error) {
	log.V(1).Info("Deleting Endpoint")
	// TODO: cleanup endpoint
	if modified, err := clientutils.PatchEnsureNoFinalizer(ctx, r.Client, endpoint, EndpointFinalizer); err != nil || modified {
		return ctrl.Result{}, err
	}
	log.V(1).Info("Deleted Endpoint")
	return ctrl.Result{}, nil
}

func (r *EndpointReconciler) reconcile(ctx context.Context, log logr.Logger, endpoint *metalv1alpha1.Endpoint) (ctrl.Result, error) {
	log.V(1).Info("Reconciling endpoint")
	if shouldIgnoreReconciliation(endpoint) {
		log.V(1).Info("Skipped Endpoint reconciliation")
		return ctrl.Result{}, nil
	}

	sanitizedMACAddress := strings.Replace(endpoint.Spec.MACAddress, ":", "", -1)
	for _, m := range r.MACPrefixes.MacPrefixes {
		if strings.HasPrefix(sanitizedMACAddress, m.MacPrefix) && m.Type == BMCType {
			log.V(1).Info("Found a BMC adapter for endpoint", "Type", m.Type, "Protocol", m.Protocol)
			if len(m.DefaultCredentials) == 0 {
				return ctrl.Result{}, fmt.Errorf("no default credentials present for BMC %s", endpoint.Spec.MACAddress)
			}
			switch m.Protocol {
			case ProtocolRedfish:
				log.V(1).Info("Creating client for BMC")
				bmcAddress := fmt.Sprintf("%s://%s:%d", r.getProtocol(), endpoint.Spec.IP, m.Port)
				bmcClient, err := bmc.NewRedfishBMCClient(ctx, bmcAddress, m.DefaultCredentials[0].Username, m.DefaultCredentials[0].Password, true)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
				}
				defer bmcClient.Logout()

				// TODO: ensure that BMC has the correct MACAddress

				var bmcSecret *metalv1alpha1.BMCSecret
				if bmcSecret, err = r.applyBMCSecret(ctx, endpoint, m); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to apply BMCSecret: %w", err)
				}
				log.V(1).Info("Applied BMC secret for endpoint")

				if err := r.applyBMC(ctx, endpoint, bmcSecret, m); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to apply BMC object: %w", err)
				}
				log.V(1).Info("Applied BMC object for endpoint")
			}
			// TODO: other types like Switches can be handled here later
		}
	}
	log.V(1).Info("Reconciled endpoint")

	return ctrl.Result{}, nil
}

func (r *EndpointReconciler) getProtocol() string {
	protocol := "https"
	if r.Insecure {
		protocol = "http"
	}
	return protocol
}

func (r *EndpointReconciler) applyBMC(ctx context.Context, endpoint *metalv1alpha1.Endpoint, secret *metalv1alpha1.BMCSecret, m macdb.MacPrefix) error {
	bmcObj := &metalv1alpha1.BMC{
		TypeMeta: metav1.TypeMeta{
			APIVersion: metalv1alpha1.GroupVersion.String(),
			Kind:       "BMC",
		},
		ObjectMeta: metav1.ObjectMeta{
			// TODO: make name prefix configurable
			Name: fmt.Sprintf("bmc-%s", endpoint.Name),
		},
		Spec: metalv1alpha1.BMCSpec{
			EndpointRef: corev1.LocalObjectReference{
				Name: endpoint.Name,
			},
			BMCSecretRef: corev1.LocalObjectReference{
				Name: secret.Name,
			},
			Protocol: metalv1alpha1.Protocol{
				Name: metalv1alpha1.ProtocolName(m.Protocol),
				Port: m.Port,
			},
			ConsoleProtocol: &metalv1alpha1.ConsoleProtocol{
				Name: metalv1alpha1.ConsoleProtocolName(m.Console.Type),
				Port: m.Console.Port,
			},
		},
	}

	if err := controllerutil.SetControllerReference(endpoint, bmcObj, r.Client.Scheme()); err != nil {
		return err
	}

	if err := r.Patch(ctx, bmcObj, client.Apply, fieldOwner); err != nil {
		return err
	}

	return nil
}

func (r *EndpointReconciler) applyBMCSecret(ctx context.Context, endpoint *metalv1alpha1.Endpoint, m macdb.MacPrefix) (*metalv1alpha1.BMCSecret, error) {
	bmcSecret := &metalv1alpha1.BMCSecret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: metalv1alpha1.GroupVersion.String(),
			Kind:       "BMCSecret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: GetBMCSecretNameFromEndpoint(endpoint),
		},
		Data: map[string][]byte{
			"username": []byte(base64.StdEncoding.EncodeToString([]byte(m.DefaultCredentials[0].Username))),
			"password": []byte(base64.StdEncoding.EncodeToString([]byte(m.DefaultCredentials[0].Password))),
		},
	}

	if err := controllerutil.SetControllerReference(endpoint, bmcSecret, r.Client.Scheme()); err != nil {
		return nil, err
	}

	if err := r.Patch(ctx, bmcSecret, client.Apply, fieldOwner); err != nil {
		return nil, err
	}

	return bmcSecret, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.Endpoint{}).
		Owns(&metalv1alpha1.BMCSecret{}).
		Owns(&metalv1alpha1.BMC{}).
		Complete(r)
}
