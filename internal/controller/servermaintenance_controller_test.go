// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("ServerMaintenance Controller", func() {
	ns := SetupTest(nil)

	var server *metalv1alpha1.Server
	var bmcSecret *metalv1alpha1.BMCSecret

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
				UUID:       "38947555-7742-3448-3784-823347823834",
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Address: "127.0.0.1",
					BMCSecretRef: corev1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		EnsureCleanState()
	})

	It("Should force a Server into maintenance from Initial State", func(ctx SpecContext) {
		By("Patching server to Initial State")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateInitial
		})).Should(Succeed())

		By("Creating an ServerMaintenance object")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenanceTemplate: metalv1alpha1.ServerMaintenanceTemplate{
					Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
					ServerPower: metalv1alpha1.PowerOff,
					ServerBootConfigurationTemplate: &metalv1alpha1.ServerBootConfigurationTemplate{
						Name: "test-boot",
						Spec: metalv1alpha1.ServerBootConfigurationSpec{
							ServerRef: v1.LocalObjectReference{Name: server.Name},
							Image:     "some_image",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance)).To(Succeed())

		By("Checking the ServerMaintenance is in maintenance state")
		Eventually(Object(serverMaintenance)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		))

		By("Checking the Server is in maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
		))

		By("Deleting the ServerMaintenance to finish the maintenance on the server")
		Expect(k8sClient.Delete(ctx, serverMaintenance)).To(Succeed())

		By("Checking the Server is not in maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
		))
	})

	It("Should wait to put a Server into maintenance until approval", func(ctx SpecContext) {
		By("Patching server to Available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Creating an Ignition secret")
		ignitionSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ignitionSecret)

		By("Creating a ServerClaim object")
		serverClaim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:             powerOpOff,
				ServerRef:         &corev1.LocalObjectReference{Name: server.Name},
				IgnitionSecretRef: &corev1.LocalObjectReference{Name: ignitionSecret.Name},
				Image:             "foo:latest",
			},
		}
		Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())

		By("Ensuring that the Server is reserved by the ServerClaim")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
			HaveField("Spec.ServerClaimRef.Name", serverClaim.Name),
		))

		By("Creating an ServerMaintenance object")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenanceTemplate: metalv1alpha1.ServerMaintenanceTemplate{
					Policy:      metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
					ServerPower: metalv1alpha1.PowerOff,
					ServerBootConfigurationTemplate: &metalv1alpha1.ServerBootConfigurationTemplate{
						Name: "test-boot",
						Spec: metalv1alpha1.ServerBootConfigurationSpec{
							ServerRef: v1.LocalObjectReference{Name: server.Name},
							Image:     "some_image",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance)).To(Succeed())
		Eventually(Object(serverMaintenance)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending),
		))

		By("Ensuring that the ServerClaim has the maintenance needed annotation")
		Eventually(Object(serverClaim)).Should(SatisfyAll(
			HaveField("ObjectMeta.Annotations", HaveKeyWithValue(metalv1alpha1.ServerMaintenanceNeededLabelKey, "true")),
		))

		By("Checking the Server is not in maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		))

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, "true")
		})).Should(Succeed())

		maintenanceLabels := map[string]string{
			metalv1alpha1.ServerMaintenanceNeededLabelKey:      "true",
			metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-maintenance",
			metalv1alpha1.ServerMaintenanceApprovalKey:         "true",
		}
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef.Name", serverMaintenance.Name),
			HaveField("Spec.MaintenanceBootConfigurationRef", Not(BeNil())),
		))

		By("Ensuring that the ServerBootConfiguration is created")
		bootConfig := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      server.Spec.MaintenanceBootConfigurationRef.Name,
				Namespace: server.Spec.MaintenanceBootConfigurationRef.Namespace,
			},
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("Spec.ServerRef.Name", server.Name),
			HaveField("Spec.Image", "some_image"),
		))

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(bootConfig, func() {
			bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		By("Checking the Server is in maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
		))

		By("Checking the ServerClaim has the maintenance labels")
		Eventually(Object(serverClaim)).Should(SatisfyAll(
			HaveField("ObjectMeta.Annotations", maintenanceLabels),
		))

		By("Checking the ServerMaintenance is in maintenance")
		Eventually(Object(serverMaintenance)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		))

		By("Deleting ServerMaintenance to finish the maintennce on the server")
		Expect(k8sClient.Delete(ctx, serverMaintenance)).To(Succeed())

		By("Checking the Server is not in maintenance and cleaned up")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
			HaveField("Spec.MaintenanceBootConfigurationRef", BeNil()),
		))

		By("Checking the ServerClaim is cleaned up")
		Eventually(Object(serverClaim)).Should(SatisfyAll(
			HaveField("ObjectMeta.Annotations", Not(HaveKey(metalv1alpha1.ServerMaintenanceNeededLabelKey))),
		))

		By("Deleting the ServerClaim")
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
	})

	It("Should wait for other maintenance to complete before starting a new one", func(ctx SpecContext) {
		By("Creating an ServerMaintenance objects")
		serverMaintenance01 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance01",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenanceTemplate: metalv1alpha1.ServerMaintenanceTemplate{
					Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
					ServerPower: metalv1alpha1.PowerOff,
					ServerBootConfigurationTemplate: &metalv1alpha1.ServerBootConfigurationTemplate{
						Name: "test-boot",
						Spec: metalv1alpha1.ServerBootConfigurationSpec{
							ServerRef: v1.LocalObjectReference{Name: server.Name},
							Image:     "some_image",
						},
					},
				},
			},
		}
		serverMaintenance02 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance02",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenanceTemplate: metalv1alpha1.ServerMaintenanceTemplate{
					Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
					ServerPower: metalv1alpha1.PowerOff,
					ServerBootConfigurationTemplate: &metalv1alpha1.ServerBootConfigurationTemplate{
						Name: "test-boot",
						Spec: metalv1alpha1.ServerBootConfigurationSpec{
							ServerRef: v1.LocalObjectReference{Name: server.Name},
							Image:     "some_image",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance01)).To(Succeed())

		By("Checking the ServerMaintenanceRef")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", Not(BeNil())),
			HaveField("Spec.MaintenanceBootConfigurationRef", Not(BeNil())),
		))
		Eventually(Object(serverMaintenance01)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		))
		bootConfig := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      server.Spec.MaintenanceBootConfigurationRef.Name,
				Namespace: server.Spec.MaintenanceBootConfigurationRef.Namespace,
			},
		}
		Eventually(Get(bootConfig)).Should(Succeed())

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(bootConfig, func() {
			bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		By("Creating a second ServerMaintenance object")
		Expect(k8sClient.Create(ctx, serverMaintenance02)).To(Succeed())

		Eventually(Object(serverMaintenance02)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending),
		))

		By("Checking the Server is in maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
		))

		By("Checking the second ServerMaintenance is still pending")
		Eventually(Object(serverMaintenance02)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending),
		))

		By("Deleting first ServerMaintenance to finish the maintenance on the server")
		Expect(k8sClient.Delete(ctx, serverMaintenance01)).To(Succeed())

		By("Checking the second ServerMaintenance is now in maintenance")
		Eventually(Object(serverMaintenance02)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		))

		By("Setting the maintenance to completed")
		Eventually(UpdateStatus(serverMaintenance02, func() {
			serverMaintenance02.Status.State = metalv1alpha1.ServerMaintenanceStateCompleted
		})).Should(Succeed())

		By("Checking the Server is not in maintenance and cleaned up")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
			HaveField("Spec.MaintenanceBootConfigurationRef", BeNil()),
		))
	})
})
