// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

// Deprecated: These tests cover the migration behaviour of the ServerMaintenance controller
// in the old metal.ironcore.dev group. The canonical controller now lives in maintenance-operator
// under servermaintenance.metal.ironcore.dev. Only the following workflows are still supported:
//   - Objects already InMaintenance continue to be fully served (power, boot config, LED, cleanup).
//   - Deletion / finalizer cleanup runs unchanged.
//   - Pending objects are deleted so their owner recreates them in the new group.
var _ = Describe("ServerMaintenance Controller (deprecated group)", func() {
	ns := SetupTest(nil)

	var (
		server    *metalv1alpha1.Server
		bmcSecret *metalv1alpha1.BMCSecret
	)

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a Server")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-maintenance-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: MockServerPort,
					},
					Address: MockServerIP,
					BMCSecretRef: corev1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Waiting for server to reach a non-Initial state")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateDiscovery))
	})

	AfterEach(func(ctx SpecContext) {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, server))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, bmcSecret))).To(Succeed())
		EnsureCleanState()
	})

	// createInMaintenance simulates an in-flight ServerMaintenance from before the migration.
	// Uses OperationAnnotationIgnore so the reconciler skips the object during setup, manually
	// sets finalizer, status and server.Spec.ServerMaintenanceRef, then removes the annotation
	// so the reconciler handles it as an active InMaintenance object going forward.
	// A DeferCleanup is registered to force-remove the finalizer and delete the object if the
	// test itself doesn't delete it (e.g. on failure), so AfterEach can clean up cleanly.
	createInMaintenance := func(ctx SpecContext, name, namespace string, srv *metalv1alpha1.Server, spec metalv1alpha1.ServerMaintenanceSpec) *metalv1alpha1.ServerMaintenance {
		maintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Annotations: map[string]string{
					metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationIgnore,
				},
			},
			Spec: spec,
		}
		Expect(k8sClient.Create(ctx, maintenance)).To(Succeed())

		DeferCleanup(func(ctx SpecContext) {
			m := &metalv1alpha1.ServerMaintenance{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(maintenance), m); apierrors.IsNotFound(err) {
				return
			}
			// Strip finalizer so the object can be deleted even if something went wrong.
			Eventually(Update(m, func() { m.Finalizers = nil })).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, m))).To(Succeed())
		})

		By("Adding finalizer and setting ServerMaintenanceRef on Server (bypassing reconciler)")
		Eventually(Update(maintenance, func() {
			maintenance.Finalizers = []string{serverMaintenanceFinalizer}
		})).Should(Succeed())

		Eventually(UpdateStatus(maintenance, func() {
			maintenance.Status.State = metalv1alpha1.ServerMaintenanceStateInMaintenance
		})).Should(Succeed())

		Eventually(Update(srv, func() {
			srv.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{
				Name:      maintenance.Name,
				Namespace: maintenance.Namespace,
			}
		})).Should(Succeed())

		By("Removing ignore annotation so reconciler handles it as active InMaintenance")
		Eventually(Update(maintenance, func() {
			delete(maintenance.Annotations, metalv1alpha1.OperationAnnotation)
		})).Should(Succeed())

		return maintenance
	}

	It("should honor an existing InMaintenance object: serve LED and clean up on delete", func(ctx SpecContext) {
		maintenance := createInMaintenance(ctx, "test-in-maintenance", ns.Name, server,
			metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
				LocatorLED:  metalv1alpha1.LitIndicatorLED,
			},
		)

		By("Confirming the server controller transitions Server to Maintenance state")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateMaintenance))

		By("Confirming the SM controller applies LocatorLED")
		Eventually(Object(server)).Should(HaveField("Spec.IndicatorLED", metalv1alpha1.LitIndicatorLED))

		By("Deleting the ServerMaintenance to end maintenance")
		Expect(k8sClient.Delete(ctx, maintenance)).To(Succeed())

		By("Confirming finalizer cleanup clears ServerMaintenanceRef and LocatorLED")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
			HaveField("Spec.IndicatorLED", metalv1alpha1.OffIndicatorLED),
		))

		By("Confirming the ServerMaintenance is fully deleted")
		Eventually(Get(maintenance)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should run finalizer cleanup when the referenced Server is already gone", func(ctx SpecContext) {
		maintenance := createInMaintenance(ctx, "test-server-gone", ns.Name, server,
			metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &corev1.LocalObjectReference{Name: server.Name},
				Policy:    metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		)

		By("Confirming server is in Maintenance")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateMaintenance))

		By("Clearing ServerMaintenanceRef and finalizer on Server so it can be force-deleted")
		Eventually(Update(server, func() {
			server.Spec.ServerMaintenanceRef = nil
			server.Finalizers = nil
		})).Should(Succeed())

		By("Deleting the Server")
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Eventually(Get(server)).Should(Satisfy(apierrors.IsNotFound))

		By("Deleting the ServerMaintenance — finalizer should complete despite Server being gone")
		Expect(k8sClient.Delete(ctx, maintenance)).To(Succeed())
		Eventually(Get(maintenance)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should delete a Pending object so its owner recreates it in the new group", func(ctx SpecContext) {
		By("Creating a ServerMaintenance in Pending state (as a consumer controller would)")
		maintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pending-deprecated",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &corev1.LocalObjectReference{Name: server.Name},
				Policy:    metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, maintenance)).To(Succeed())

		By("Confirming the controller deletes Pending objects in the deprecated group")
		Eventually(Get(maintenance)).Should(Satisfy(apierrors.IsNotFound))

		By("Confirming the Server is not affected")
		Consistently(Object(server)).Should(HaveField("Spec.ServerMaintenanceRef", BeNil()))
	})
})
