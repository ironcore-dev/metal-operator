// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/controller-utils/clientutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/api/macdb"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	EndpointFinalizer = "metal.ironcore.dev/endpoint"
)

// EndpointReconciler reconciles a Endpoints object
type EndpointReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	MACPrefixes *macdb.MacPrefixes
	Insecure    bool
	BMCOptions  bmc.Options
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

	sanitizedMACAddress := strings.ReplaceAll(endpoint.Spec.MACAddress, ":", "")
	for _, m := range r.MACPrefixes.MacPrefixes {
		if strings.HasPrefix(sanitizedMACAddress, m.MacPrefix) && m.Type == metalv1alpha1.BMCType {
			log.V(1).Info("Found a BMC adapter for endpoint", "Type", m.Type, "Protocol", m.Protocol)
			if len(m.DefaultCredentials) == 0 {
				return ctrl.Result{}, fmt.Errorf("no default credentials present for BMC %s", endpoint.Spec.MACAddress)
			}

			bmcOptions := bmc.Options{
				BasicAuth: true,
				Username:  m.DefaultCredentials[0].Username,
				Password:  m.DefaultCredentials[0].Password,
			}

			protocolScheme := bmcutils.GetProtocolScheme(m.ProtocolScheme, r.Insecure)
			bmcOptions.Endpoint = fmt.Sprintf("%s://%s", protocolScheme, net.JoinHostPort(endpoint.Spec.IP.String(), fmt.Sprintf("%d", m.Port)))

			switch m.Protocol {
			case metalv1alpha1.ProtocolRedfish:
				log.V(1).Info("Creating client for BMC", "Address", bmcOptions.Endpoint)
				bmcClient, err := bmc.NewRedfishBMCClient(ctx, bmcOptions)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
				}
				defer bmcClient.Logout()

				// TODO: ensure that BMC has the correct MACAddress

				var bmcSecret *metalv1alpha1.BMCSecret
				if bmcSecret, err = r.applyBMCSecret(ctx, log, endpoint, m); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to apply BMCSecret: %w", err)
				}
				log.V(1).Info("Applied BMC secret for endpoint")

				if err := r.applyBMC(ctx, log, endpoint, bmcSecret, m); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to apply BMC object: %w", err)
				}
				log.V(1).Info("Applied BMC object for endpoint")
			case metalv1alpha1.ProtocolRedfishLocal:
				log.V(1).Info("Creating client for a local test BMC", "Address", bmcOptions.Endpoint)
				bmcClient, err := bmc.NewRedfishLocalBMCClient(ctx, bmcOptions)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
				}
				defer bmcClient.Logout()

				var bmcSecret *metalv1alpha1.BMCSecret
				if bmcSecret, err = r.applyBMCSecret(ctx, log, endpoint, m); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to apply BMCSecret: %w", err)
				}
				log.V(1).Info("Applied local test BMC secret for endpoint")

				if err := r.applyBMC(ctx, log, endpoint, bmcSecret, m); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to apply BMC object: %w", err)
				}
				log.V(1).Info("Applied BMC object for Endpoint")
			case metalv1alpha1.ProtocolRedfishKube:
				log.V(1).Info("Creating client for a kube test BMC", "Address", bmcOptions.Endpoint)
				bmcClient, err := bmc.NewRedfishKubeBMCClient(ctx, bmcOptions, r.Client, bmcutils.DefaultKubeNamespace)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to create BMC client: %w", err)
				}
				defer bmcClient.Logout()

				var bmcSecret *metalv1alpha1.BMCSecret
				if bmcSecret, err = r.applyBMCSecret(ctx, log, endpoint, m); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to apply BMCSecret: %w", err)
				}
				log.V(1).Info("Applied kube test BMC secret for endpoint")

				if err := r.applyBMC(ctx, log, endpoint, bmcSecret, m); err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to apply BMC object: %w", err)
				}
				log.V(1).Info("Applied BMC object for Endpoint")

			}
			// TODO: other types like Switches can be handled here later
		}
	}
	log.V(1).Info("Reconciled endpoint")

	return ctrl.Result{}, nil
}

func (r *EndpointReconciler) applyBMC(ctx context.Context, log logr.Logger, endpoint *metalv1alpha1.Endpoint, secret *metalv1alpha1.BMCSecret, m macdb.MacPrefix) error {
	bmcObj := &metalv1alpha1.BMC{}
	bmcObj.Name = endpoint.Name
	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, bmcObj, func() error {
		spec := &bmcObj.Spec
		spec.EndpointRef = &corev1.LocalObjectReference{Name: endpoint.Name}
		spec.BMCSecretRef = corev1.LocalObjectReference{Name: secret.Name}
		spec.Protocol = metalv1alpha1.Protocol{
			Name:   metalv1alpha1.ProtocolName(m.Protocol),
			Port:   m.Port,
			Scheme: m.ProtocolScheme,
		}
		spec.ConsoleProtocol = &metalv1alpha1.ConsoleProtocol{
			Name: metalv1alpha1.ConsoleProtocolName(m.Console.Type),
			Port: m.Console.Port,
		}
		return controllerutil.SetControllerReference(endpoint, bmcObj, r.Client.Scheme())
	})
	if err != nil {
		return fmt.Errorf("failed to create or patch BMC: %w", err)
	}
	log.V(1).Info("Created or patched BMC", "BMC", bmcObj.Name, "Operation", opResult)

	return nil
}

func (r *EndpointReconciler) applyBMCSecret(ctx context.Context, log logr.Logger, endpoint *metalv1alpha1.Endpoint, m macdb.MacPrefix) (*metalv1alpha1.BMCSecret, error) {
	bmcSecret := &metalv1alpha1.BMCSecret{}
	bmcSecret.Name = endpoint.Name
	opResult, err := controllerutil.CreateOrPatch(ctx, r.Client, bmcSecret, func() error {
		bmcSecret.Data = map[string][]byte{
			metalv1alpha1.BMCSecretUsernameKeyName: []byte(m.DefaultCredentials[0].Username),
			metalv1alpha1.BMCSecretPasswordKeyName: []byte(m.DefaultCredentials[0].Password),
		}
		return controllerutil.SetControllerReference(endpoint, bmcSecret, r.Client.Scheme())
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create or patch BMCSecret: %w", err)
	}
	log.V(1).Info("Created or patched BMSecret", "BMCSecret", bmcSecret.Name, "Operation", opResult)

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
