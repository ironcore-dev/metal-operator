// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BIOSSettings Controller", func() {
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

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BiosSettingsSpec{
				BIOSSettings:            metalv1alpha1.Settings{Version: "P79 v1.45 (12/06/2017)", SettingsMap: BIOSSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettings.Name}),
		))

		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BiosSettingsStateApplied),
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

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BiosSettingsSpec{
				BIOSSettings:            metalv1alpha1.Settings{Version: "P79 v1.45 (12/06/2017)", SettingsMap: BIOSSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the Server has the bios setting ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettings.Name}),
		))

		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BiosSettingsStateApplied),
		))

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the bios maintenance ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})

	// todo: add more tests with https://github.com/ironcore-dev/metal-operator/issues/292

	// can not test upgrade workflow as we need to decide on the architecture.

	// It("should move to through upgrade state if no bios version is not right", func(ctx SpecContext) {
	// 	BIOSSetting := make(map[string]string)

	// 	By("Ensuring that the server has Initial state")
	// 	Eventually(Object(server)).Should(SatisfyAll(
	// 		HaveField("Status.State", metalv1alpha1.ServerStateInitial),
	// 	))

	// 	By("update the server state to Available  state")
	// 	Eventually(UpdateStatus(server, func() {
	// 		server.Status.State = metalv1alpha1.ServerStateAvailable
	// 	})).Should(Succeed())

	// 	By("Creating a BIOSSetting")
	// 	biosSettings := &metalv1alpha1.BIOSSettings{
	// 		ObjectMeta: metav1.ObjectMeta{
	// 			Namespace:    ns.Name,
	// 			GenerateName: "test-",
	// 		},
	// 		Spec: metalv1alpha1.BiosSettingsSpec{
	// 			BIOSSettings:            metalv1alpha1.Settings{Version: "P79 v2.0 (12/06/2017)", SettingsMap: BIOSSetting},
	// 			ServerRef:               &v1.LocalObjectReference{Name: server.Name},
	// 			ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
	// 		},
	// 	}
	// 	Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

	// 	By("Ensuring that the biosSettings has the correct state, reached InVersionUpgrade")
	// 	Eventually(Object(biosSettings)).Should(SatisfyAll(
	// 		HaveField("Status.State", metalv1alpha1.BiosSettingsStateInVersionUpgrade),
	// 	))

	// 	By("Ensuring that the Server has the correct bios server settings ref")
	// 	Eventually(Object(server)).Should(SatisfyAll(
	// 		HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettings.Name}),
	// 	))

	// 	By("Ensuring that the biosSettings has created the Maintenance request")
	// 	var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
	// 	Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

	// 	serverMaintenance := &metalv1alpha1.ServerMaintenance{
	// 		ObjectMeta: metav1.ObjectMeta{
	// 			Namespace: ns.Name,
	// 			Name:      biosSettings.Name,
	// 		},
	// 	}
	// 	Eventually(Get(serverMaintenance)).Should(Succeed())

	// 	By("Ensuring that the Maintenance resource has been referenced by biosSettings")
	// 	Eventually(Object(biosSettings)).Should(SatisfyAny(
	// 		HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
	// 			Kind:      "ServerMaintenance",
	// 			Name:      serverMaintenance.Name,
	// 			Namespace: serverMaintenance.Namespace,
	// 			UID:       serverMaintenance.UID,
	// 		}),
	// 		HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
	// 			Kind:       "ServerMaintenance",
	// 			Name:       serverMaintenance.Name,
	// 			Namespace:  serverMaintenance.Namespace,
	// 			UID:        serverMaintenance.UID,
	// 			APIVersion: "metal.ironcore.dev/v1alpha1",
	// 		}),
	// 	))

	// 	By("Ensuring that the server has accepted the Maintenance request")
	// 	Eventually(Object(server)).Should(SatisfyAll(
	// 		HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
	// 			Kind:       "ServerMaintenance",
	// 			Name:       serverMaintenance.Name,
	// 			Namespace:  serverMaintenance.Namespace,
	// 			UID:        serverMaintenance.UID,
	// 			APIVersion: "metal.ironcore.dev/v1alpha1",
	// 		}),
	// 	))

	// 	By("Ensuring that the biosSettings state is correct State inVersionUpgrade")
	// 	Eventually(Object(biosSettings)).Should(SatisfyAny(
	// 		HaveField("Status.State", metalv1alpha1.BiosSettingsStateInVersionUpgrade),
	// 	))

	// 	By("Simulate the server BIOS version update by matching the spec version")
	// 	Eventually(Update(biosSettings, func() {
	// 		biosSettings.Spec.BIOSSettings.Version = "P79 v1.45 (12/06/2017)"
	// 	})).Should(Succeed())

	// 	By("Ensuring that the biosSettings has completed Upgrade and setting update moved the state")
	// 	Eventually(Object(biosSettings)).Should(SatisfyAny(
	// 		HaveField("Status.State", metalv1alpha1.BiosSettingsStateInProgress),
	// 		HaveField("Status.State", metalv1alpha1.BiosSettingsStateSynced),
	// 	))
	// 	Eventually(Object(biosSettings)).Should(SatisfyAll(
	// 		HaveField("Status.State", metalv1alpha1.BiosSettingsStateSynced),
	// 	))

	// 	By("Ensuring that the Server Maintenance BIOS ref is empty")
	// 	Eventually(Object(server)).Should(SatisfyAll(
	// 		HaveField("Spec.ServerMaintenanceRef", BeNil()),
	// 	))

	// 	By("Ensuring that the serverMaintenance is deleted")
	// 	Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))

	// 	By("Deleting the BIOSSettings")
	// 	Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

	// 	By("Ensuring that the biosSettings is removed")
	// 	Eventually(Get(biosSettings)).Should(Satisfy(apierrors.IsNotFound))

	// 	By("Ensuring that the Server BIOS ref is empty")
	// 	Eventually(Object(server)).Should(SatisfyAll(
	// 		HaveField("Spec.BIOSSettingsRef", BeNil()),
	// 	))
	// })
})
