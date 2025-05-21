// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("ServerMaintenance Controller", func() {
	ns := SetupTest()

	var server *metalv1alpha1.Server

	BeforeEach(func(ctx SpecContext) {
		By("Ensuring clean state")
		var serverList metalv1alpha1.ServerList
		Eventually(ObjectList(&serverList)).Should(HaveField("Items", (BeEmpty())))
		var maintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&maintenanceList)).Should(HaveField("Items", (BeEmpty())))

		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
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
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		TransistionServerFromInitialToAvailableState(ctx, k8sClient, server, ns.Name)
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
	})

	It("Should force a Server into maintenance from Initial State", func(ctx SpecContext) {

		By("patching server to Initial State")
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
				ServerRef:   &v1.LocalObjectReference{Name: server.Name},
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
		}
		Expect(k8sClient.Create(ctx, serverMaintenance)).To(Succeed())
		Eventually(Object(serverMaintenance)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		))

		By("Checking the Server is in maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
		))
	})

	It("Should wait to put a Server into maintenance until approval", func(ctx SpecContext) {

		serverClaim := GetServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOff, "abc:abc")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

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
				ServerRef:   &v1.LocalObjectReference{Name: server.Name},
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
		}
		Expect(k8sClient.Create(ctx, serverMaintenance)).To(Succeed())
		Eventually(Object(serverMaintenance)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStatePending),
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
		bootConfig := &metalv1alpha1.ServerBootConfiguration{}

		Eventually(k8sClient.Get).WithArguments(ctx, types.NamespacedName{
			Name:      server.Spec.MaintenanceBootConfigurationRef.Name,
			Namespace: server.Spec.MaintenanceBootConfigurationRef.Namespace,
		}, bootConfig).Should(Succeed())

		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(bootConfig, func() {
			bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())

		By("Checking the Server is in maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
		))

		Eventually(Object(serverClaim)).Should(SatisfyAll(
			HaveField("ObjectMeta.Annotations", maintenanceLabels),
		))

		By("Checking the ServerMaintenance is in maintenance")
		Eventually(Object(serverMaintenance)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		))
		By("Deleting ServerMaintenance to finish the maintennce on the server")
		Eventually(k8sClient.Delete).WithArguments(ctx, serverMaintenance).Should(Succeed())

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
	})

	It("Should wait for other maintenance to complete before starting a new one", func(ctx SpecContext) {
		By("Creating an ServerMaintenance object")

		serverMaintenance01 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server-maintenance01",
				Namespace: ns.Name,
				Annotations: map[string]string{
					metalv1alpha1.ServerMaintenanceReasonAnnotationKey: "test-maintenance",
				},
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef:   &v1.LocalObjectReference{Name: server.Name},
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
				ServerRef:   &v1.LocalObjectReference{Name: server.Name},
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
		bootConfig := &metalv1alpha1.ServerBootConfiguration{}
		Eventually(k8sClient.Get).WithArguments(ctx, types.NamespacedName{
			Name:      server.Spec.MaintenanceBootConfigurationRef.Name,
			Namespace: server.Spec.MaintenanceBootConfigurationRef.Namespace,
		}, bootConfig).Should(Succeed())

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
		Eventually(k8sClient.Delete).WithArguments(ctx, serverMaintenance01).Should(Succeed())

		Eventually(Object(serverMaintenance02)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		))

		By("Checking the second ServerMaintenance is now in maintenance")
		Eventually(Object(serverMaintenance02)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		))
	})
})
