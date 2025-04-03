// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ServerBIOS Controller", func() {
	ns := SetupTest()
	ns.Name = "default"

	var server *metalv1alpha1.Server

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
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
				Namespace:    ns.Name,
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
		Expect(k8sClient.Create(ctx, server)).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
	})

	It("should successfully patch its reference to referred server", func(ctx SpecContext) {
		BIOSSetting := make(map[string]string)

		By("Ensuring that the server has Initial state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateInitial),
		))

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Ensuring that the server has the correct state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))

		By("Creating a BIOSSetting")
		serverBIOS := &metalv1alpha1.ServerBIOS{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerBIOSSpec{
				BIOS:                    metalv1alpha1.BIOSSettings{Version: "P79 v1.45 (12/06/2017)", Settings: BIOSSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, serverBIOS)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", Not(BeNil())),
			HaveField("Spec.BIOSSettingsRef.Name", serverBIOS.Name),
		))

		Eventually(Object(serverBIOS)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSMaintenanceStateSynced),
		))
	})

	It("should move to completed if no bios setting changes to referred server", func(ctx SpecContext) {
		BIOSSetting := make(map[string]string)

		By("Ensuring that the server has Initial state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateInitial),
		))

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Ensuring that the server has the correct state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))

		By("Creating a BIOSSetting")
		serverBIOS := &metalv1alpha1.ServerBIOS{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerBIOSSpec{
				BIOS:                    metalv1alpha1.BIOSSettings{Version: "P79 v1.45 (12/06/2017)", Settings: BIOSSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, serverBIOS)).To(Succeed())

		By("Ensuring that the Server has the bios setting ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", Not(BeNil())),
			HaveField("Spec.BIOSSettingsRef.Name", serverBIOS.Name),
		))

		Eventually(Object(serverBIOS)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSMaintenanceStateSynced),
		))

		By("Deleting the ServerBIOS")
		Expect(k8sClient.Delete(ctx, serverBIOS)).To(Succeed())

		By("Ensuring that the Server BIOS ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})

	It("should move to through upgrade state if no bios version is not right", func(ctx SpecContext) {
		BIOSSetting := make(map[string]string)

		By("Ensuring that the server has Initial state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateInitial),
		))

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Ensuring that the server has the correct state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))

		By("Creating a BIOSSetting")
		serverBIOS := &metalv1alpha1.ServerBIOS{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerBIOSSpec{
				BIOS:                    metalv1alpha1.BIOSSettings{Version: "P79 v2.0 (12/06/2017)", Settings: BIOSSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, serverBIOS)).To(Succeed())

		By("Ensuring that the serverBIOS has the correct state, reached InVersionUpgrade")
		Eventually(Object(serverBIOS)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSMaintenanceStateInVersionUpgrade),
		))

		By("Ensuring that the Server has the correct bios server settings ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", Not(BeNil())),
			HaveField("Spec.BIOSSettingsRef.Name", serverBIOS.Name),
		))

		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		By("Ensuring that the Maintenance resource has been referenced by serverBIOS")
		Eventually(Object(serverBIOS)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", Not(BeNil())),
		))

		By("Ensuring that the serverBIOS has created the Maintenance request")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      serverBIOS.Spec.ServerMaintenanceRef.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the server has accepted the Maintenance request")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", Not(BeNil())),
			HaveField("Spec.ServerMaintenanceRef.UID", serverMaintenance.UID),
			HaveField("Spec.ServerMaintenanceRef.UID", serverBIOS.Spec.ServerMaintenanceRef.UID),
		))

		By("Ensuring that the serverBIOS state is correct State inVersionUpgrade")
		Eventually(Object(serverBIOS)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSMaintenanceStateInVersionUpgrade),
		))

		By("Simulate the server BIOS version update by matching the spec version")
		Eventually(Update(serverBIOS, func() {
			serverBIOS.Spec.BIOS.Version = "P79 v1.45 (12/06/2017)"
		})).Should(Succeed())

		By("Ensuring that the serverBIOS has completed Upgrade and setting update moved the state")
		Eventually(Object(serverBIOS)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSMaintenanceStateInSettingUpdate),
			HaveField("Status.State", metalv1alpha1.BIOSMaintenanceStateSynced),
		))
		Eventually(Object(serverBIOS)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSMaintenanceStateSynced),
		))

		By("Ensuring that the Server Maintenance BIOS ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		By("Ensuring that the serverMaintenance is deleted")
		Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))

		By("Deleting the ServerBIOS")
		Expect(k8sClient.Delete(ctx, serverBIOS)).To(Succeed())

		By("Ensuring that the serverBIOS is removed")
		Eventually(Get(serverBIOS)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the Server BIOS ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})

})
