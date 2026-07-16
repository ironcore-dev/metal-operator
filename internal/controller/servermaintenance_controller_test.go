// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("ServerMaintenance Controller", func() {
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
	})

	AfterEach(func(ctx SpecContext) {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, server))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, bmcSecret))).To(Succeed())
		EnsureCleanState()
	})

	It("should force a Server into maintenance from Initial State", func(ctx SpecContext) {
		By("Patching server to Initial State")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateInitial
		})).Should(Succeed())

		By("Creating a ServerMaintenance object")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
				ServerBootConfigurationTemplate: &metalv1alpha1.ServerBootConfigurationTemplate{
					Name: "test-boot",
					Spec: metalv1alpha1.ServerBootConfigurationSpec{
						ServerRef: corev1.LocalObjectReference{Name: server.Name},
						Image:     "some_image",
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

	It("should wait to put a Server into maintenance until approval", func(ctx SpecContext) {
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

		By("Creating a ServerMaintenance object")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				ServerPower: metalv1alpha1.PowerOff,
				ServerBootConfigurationTemplate: &metalv1alpha1.ServerBootConfigurationTemplate{
					Name: "test-boot",
					Spec: metalv1alpha1.ServerBootConfigurationSpec{
						ServerRef: corev1.LocalObjectReference{Name: server.Name},
						Image:     "foo:latest",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance)).To(Succeed())
		Eventually(Object(serverMaintenance)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending),
		))

		By("Ensuring that the ServerClaim has the maintenance needed label and annotation")
		Eventually(Object(serverClaim)).Should(SatisfyAll(
			HaveField("ObjectMeta.Labels", HaveKeyWithValue(metalv1alpha1.ServerMaintenanceNeededLabelKey, trueValue)),
		))

		By("Checking the Server is not in maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		))

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetLabel(serverClaim, metalv1alpha1.ServerMaintenanceApprovedLabelKey, trueValue)
		})).Should(Succeed())

		maintenanceLabels := map[string]string{
			metalv1alpha1.ServerMaintenanceNeededLabelKey:   trueValue,
			metalv1alpha1.ServerMaintenanceApprovedLabelKey: trueValue,
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
		Eventually(Get(bootConfig)).Should(Succeed())

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(bootConfig, func() {
			bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		By("Checking the Server is in maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
		))

		By("Checking the ServerClaim has the maintenance labels and annotations")
		Eventually(Object(serverClaim)).Should(SatisfyAll(
			HaveField("ObjectMeta.Labels", maintenanceLabels),
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
			HaveField("ObjectMeta.Labels", Not(HaveKey(metalv1alpha1.ServerMaintenanceNeededLabelKey))),
		))

		By("Deleting the ServerClaim")
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
	})

	It("should wait for other maintenance to complete before starting a new one", func(ctx SpecContext) {
		By("Creating ServerMaintenance objects")
		serverMaintenance01 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance01",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
				ServerBootConfigurationTemplate: &metalv1alpha1.ServerBootConfigurationTemplate{
					Name: "test-boot",
					Spec: metalv1alpha1.ServerBootConfigurationSpec{
						ServerRef: corev1.LocalObjectReference{Name: server.Name},
						Image:     "some_image",
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
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
				ServerBootConfigurationTemplate: &metalv1alpha1.ServerBootConfigurationTemplate{
					Name: "test-boot",
					Spec: metalv1alpha1.ServerBootConfigurationSpec{
						ServerRef: corev1.LocalObjectReference{Name: server.Name},
						Image:     "some_image",
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

		By("Deleting the second ServerMaintenance to finish the maintenance on the server")
		Expect(k8sClient.Delete(ctx, serverMaintenance02)).To(Succeed())

		By("Ensuring that the Server is in discovery state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateDiscovery)))
	})

	It("should prioritize higher-priority maintenance for the same server", func(ctx SpecContext) {
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

		By("Creating low and high priority ServerMaintenance objects")
		lowPriorityMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-low-priority-maintenance",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				Priority:    10,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		highPriorityMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-high-priority-maintenance",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				Priority:    100,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		Expect(k8sClient.Create(ctx, lowPriorityMaintenance)).To(Succeed())
		Expect(k8sClient.Create(ctx, highPriorityMaintenance)).To(Succeed())

		By("Ensuring both ServerMaintenances are pending")
		Eventually(Object(lowPriorityMaintenance)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending))
		Eventually(Object(highPriorityMaintenance)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending))

		By("Approving maintenance on the ServerClaim")
		Eventually(Update(serverClaim, func() {
			metautils.SetLabel(serverClaim, metalv1alpha1.ServerMaintenanceApprovedLabelKey, trueValue)
		})).Should(Succeed())

		By("Ensuring high-priority maintenance starts first")
		Eventually(Object(server)).Should(HaveField("Spec.ServerMaintenanceRef.Name", highPriorityMaintenance.Name))
		Eventually(Object(highPriorityMaintenance)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance))
		Consistently(Object(lowPriorityMaintenance)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending))

		By("Deleting high-priority maintenance")
		Expect(k8sClient.Delete(ctx, highPriorityMaintenance)).To(Succeed())
		// check that the high-priority maintenance is deleted before checking the low-priority maintenance
		Eventually(Get(highPriorityMaintenance)).Should(Satisfy(apierrors.IsNotFound))
		By("Ensuring low-priority maintenance can proceed with the existing approval")
		Eventually(Object(lowPriorityMaintenance)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance))

		By("Deleting low-priority maintenance")
		Expect(k8sClient.Delete(ctx, lowPriorityMaintenance)).To(Succeed())
		Eventually(Get(lowPriorityMaintenance)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
	})

	It("should treat unset priority as zero", func(ctx SpecContext) {
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

		By("Creating maintenances with unset and set priority")
		unsetPriorityMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-unset-priority-maintenance",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		setPriorityMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-set-priority-maintenance",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				Priority:    1,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		Expect(k8sClient.Create(ctx, unsetPriorityMaintenance)).To(Succeed())
		Expect(k8sClient.Create(ctx, setPriorityMaintenance)).To(Succeed())

		By("Approving maintenance on the ServerClaim")
		Eventually(Update(serverClaim, func() {
			metautils.SetLabel(serverClaim, metalv1alpha1.ServerMaintenanceApprovedLabelKey, trueValue)
		})).Should(Succeed())

		By("Ensuring maintenance with explicit priority runs before unset priority")
		Eventually(Object(server)).Should(HaveField("Spec.ServerMaintenanceRef.Name", setPriorityMaintenance.Name))
		Eventually(Object(setPriorityMaintenance)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance))
		Consistently(Object(unsetPriorityMaintenance)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending))

		By("Deleting set-priority maintenance")
		Expect(k8sClient.Delete(ctx, setPriorityMaintenance)).To(Succeed())
		Eventually(Get(setPriorityMaintenance)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring unset-priority maintenance can proceed with the existing approval")
		Eventually(Object(unsetPriorityMaintenance)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance))

		By("Deleting unset-priority maintenance")
		Expect(k8sClient.Delete(ctx, unsetPriorityMaintenance)).To(Succeed())
		Eventually(Get(unsetPriorityMaintenance)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
	})

	It("should transition ServerMaintenance to Pending state when its referenced Server no longer exists", func(ctx SpecContext) {
		By("Creating a ServerMaintenance object")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance-orphan",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &corev1.LocalObjectReference{Name: server.Name},
				Policy:    metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance)).To(Succeed())
		DeferCleanup(k8sClient.Delete, serverMaintenance)

		By("Deleting the Server")
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Eventually(Get(server)).ShouldNot(Succeed())

		By("Expecting the ServerMaintenance to transition to Pending state")
		Eventually(Object(serverMaintenance)).Should(HaveField("Status.State", Equal(metalv1alpha1.ServerMaintenanceStatePending)))
	})

	It("should complete deletion when the referenced Server is already gone", func(ctx SpecContext) {
		By("Creating a ServerMaintenance object")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance-server-gone",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance)).To(Succeed())

		By("Waiting for the ServerMaintenance to reach InMaintenance state")
		Eventually(Object(serverMaintenance)).Should(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		)

		By("Deleting the Server before deleting the ServerMaintenance")
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Eventually(Get(server)).Should(Satisfy(apierrors.IsNotFound))

		By("Deleting the ServerMaintenance")
		Expect(k8sClient.Delete(ctx, serverMaintenance)).To(Succeed())

		By("Ensuring the ServerMaintenance is fully deleted despite the Server being gone")
		Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should not allow an Enforced maintenance to steal the ref from an already-active maintenance", func(ctx SpecContext) {
		By("Creating first ServerMaintenance with Enforced policy")
		serverMaintenance01 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-enforced-maintenance-active",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "first-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance01)).To(Succeed())

		By("Waiting for the first ServerMaintenance to reach InMaintenance state")
		Eventually(Object(serverMaintenance01)).Should(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		)
		Consistently(Object(serverMaintenance01)).Should(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		)

		By("Verifying the Server's ServerMaintenanceRef points to the first maintenance")
		Eventually(Object(server)).Should(
			HaveField("Spec.ServerMaintenanceRef.Name", serverMaintenance01.Name),
		)
		Consistently(Object(server)).Should(
			HaveField("Spec.ServerMaintenanceRef.Name", serverMaintenance01.Name),
		)

		By("Creating second Enforced ServerMaintenance for the same server")
		serverMaintenance02 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-enforced-maintenance-challenger",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "second-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance02)).To(Succeed())

		By("Ensuring the second Enforced maintenance stays Pending and does not steal the ref")
		Eventually(Object(serverMaintenance02)).Should(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending),
		)
		Consistently(Object(serverMaintenance02)).Should(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending),
		)

		By("Verifying the first maintenance remains InMaintenance (not evicted to Pending)")
		Consistently(Object(serverMaintenance01)).Should(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		)

		By("Verifying the Server's ServerMaintenanceRef is still held by the first maintenance")
		Consistently(Object(server)).Should(
			HaveField("Spec.ServerMaintenanceRef.Name", serverMaintenance01.Name),
		)

		By("Deleting the first ServerMaintenance to release the server")
		Expect(k8sClient.Delete(ctx, serverMaintenance01)).To(Succeed())
		Eventually(Get(serverMaintenance01)).ShouldNot(Succeed())

		By("Verifying the second maintenance can now proceed to InMaintenance")
		Eventually(Object(serverMaintenance02)).Should(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		)

		By("Deleting the second ServerMaintenance")
		Expect(k8sClient.Delete(ctx, serverMaintenance02)).To(Succeed())
		Eventually(Get(serverMaintenance02)).ShouldNot(Succeed())
	})

	It("should keep server in Maintenance throughout all queued Enforced maintenances without state bounce", func(ctx SpecContext) {
		By("Creating two Enforced ServerMaintenance objects")
		maintenance01 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-no-bounce-enforced-01",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		maintenance02 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-no-bounce-enforced-02",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		Expect(k8sClient.Create(ctx, maintenance01)).To(Succeed())
		Expect(k8sClient.Create(ctx, maintenance02)).To(Succeed())

		By("Waiting for the first maintenance to be active and server to be in Maintenance")
		Eventually(Object(server)).Should(HaveField("Spec.ServerMaintenanceRef.Name", maintenance01.Name))
		Eventually(Object(maintenance01)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance))
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateMaintenance))

		By("Ensuring the second maintenance is pending while first is active")
		Eventually(Object(maintenance02)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending))

		By("Completing the first maintenance")
		Expect(k8sClient.Delete(ctx, maintenance01)).To(Succeed())

		By("Verifying server stays in Maintenance while second maintenance takes over (no state bounce)")
		Consistently(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateMaintenance))
		Eventually(Object(server)).Should(HaveField("Spec.ServerMaintenanceRef.Name", maintenance02.Name))
		Eventually(Object(maintenance02)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance))

		By("Completing the second maintenance")
		Expect(k8sClient.Delete(ctx, maintenance02)).To(Succeed())
		Eventually(Get(maintenance02)).Should(Satisfy(apierrors.IsNotFound))

		By("Verifying server exits Maintenance only after all maintenances are done")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))
	})

	It("should keep reserved server in Maintenance throughout all queued OwnerApproval maintenances and return to Reserved only after all are done", func(ctx SpecContext) {
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
			Data: map[string][]byte{"foo": []byte("bar")},
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ignitionSecret)

		By("Creating a ServerClaim to reserve the server")
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
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
			HaveField("Spec.ServerClaimRef.Name", serverClaim.Name),
		))

		By("Creating two OwnerApproval ServerMaintenance objects")
		maintenance01 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-no-bounce-approval-01",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				Priority:    10,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		maintenance02 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-no-bounce-approval-02",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				Priority:    5,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		Expect(k8sClient.Create(ctx, maintenance01)).To(Succeed())
		Expect(k8sClient.Create(ctx, maintenance02)).To(Succeed())
		Eventually(Object(maintenance01)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending))
		Eventually(Object(maintenance02)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending))

		By("Approving maintenance on the ServerClaim (single approval covers all queued maintenances)")
		Eventually(Update(serverClaim, func() {
			metautils.SetLabel(serverClaim, metalv1alpha1.ServerMaintenanceApprovedLabelKey, trueValue)
		})).Should(Succeed())

		By("Ensuring the higher-priority maintenance starts first")
		Eventually(Object(server)).Should(HaveField("Spec.ServerMaintenanceRef.Name", maintenance01.Name))
		Eventually(Object(maintenance01)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance))
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateMaintenance))
		Consistently(Object(maintenance02)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending))

		By("Completing the first maintenance")
		Expect(k8sClient.Delete(ctx, maintenance01)).To(Succeed())
		Eventually(Get(maintenance01)).Should(Satisfy(apierrors.IsNotFound))

		By("Verifying server stays in Maintenance while second maintenance takes over (no bounce to Reserved)")
		Consistently(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateMaintenance))
		Eventually(Object(server)).Should(HaveField("Spec.ServerMaintenanceRef.Name", maintenance02.Name))
		Eventually(Object(maintenance02)).Should(HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance))

		By("Completing the second maintenance")
		Expect(k8sClient.Delete(ctx, maintenance02)).To(Succeed())
		Eventually(Get(maintenance02)).Should(Satisfy(apierrors.IsNotFound))

		By("Verifying server returns to Reserved only after all maintenances are done")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		By("Verifying approval and maintenance-needed labels are cleaned up on the ServerClaim")
		Eventually(Object(serverClaim)).Should(SatisfyAll(
			HaveField("ObjectMeta.Labels", Not(HaveKey(metalv1alpha1.ServerMaintenanceApprovedLabelKey))),
			HaveField("ObjectMeta.Labels", Not(HaveKey(metalv1alpha1.ServerMaintenanceNeededLabelKey))),
		))

		By("Deleting the ServerClaim")
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
	})

	It("should skip cleanup and remove finalizer when no finalizer is present on deletion", func(ctx SpecContext) {
		By("Creating a ServerMaintenance object without going through reconciliation (no finalizer)")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance-no-finalizer",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &corev1.LocalObjectReference{Name: server.Name},
				Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerPower: metalv1alpha1.PowerOff,
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance)).To(Succeed())

		By("Waiting for the finalizer to be added by the reconciler")
		Eventually(Object(serverMaintenance)).Should(
			HaveField("Finalizers", ContainElement(serverMaintenanceFinalizer)),
		)

		By("Setting ignore-reconciliation annotation to prevent the reconciler from re-adding the finalizer")
		Eventually(Update(serverMaintenance, func() {
			metav1.SetMetaDataAnnotation(&serverMaintenance.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationIgnore)
		})).Should(Succeed())

		By("Manually removing the finalizer to simulate a no-finalizer state")
		Eventually(Update(serverMaintenance, func() {
			serverMaintenance.Finalizers = nil
		})).Should(Succeed())

		By("Ensuring finalizers are empty before delete")
		Expect(serverMaintenance.Finalizers).To(BeEmpty())

		By("Deleting the ServerMaintenance")
		Expect(k8sClient.Delete(ctx, serverMaintenance)).To(Succeed())

		By("Ensuring the ServerMaintenance is deleted immediately without cleanup side-effects")
		Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should set the LocatorLED on maintenance start and turn it off when maintenance ends", func(ctx SpecContext) {
		By("Patching server to Available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Creating a ServerMaintenance with LocatorLED Lit")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance-led",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-led-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:  &corev1.LocalObjectReference{Name: server.Name},
				Policy:     metalv1alpha1.ServerMaintenancePolicyEnforced,
				LocatorLED: metalv1alpha1.LitIndicatorLED,
			},
		}
		Expect(k8sClient.Create(ctx, serverMaintenance)).To(Succeed())

		By("Checking the ServerMaintenance transitions to InMaintenance")
		Eventually(Object(serverMaintenance)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		))

		By("Checking that the server LocatorLED is set to Lit")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.IndicatorLED", metalv1alpha1.LitIndicatorLED),
		))

		By("Deleting the ServerMaintenance to end maintenance")
		Expect(k8sClient.Delete(ctx, serverMaintenance)).To(Succeed())

		By("Checking that the server LocatorLED is cleared to Off")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.IndicatorLED", metalv1alpha1.OffIndicatorLED),
		))

		By("Waiting for ServerMaintenance to be fully removed")
		Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))
	})
})
