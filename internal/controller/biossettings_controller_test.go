// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
	})

	It("should successfully patch its reference to referred server", func(ctx SpecContext) {
		BIOSSetting := make(map[string]string)

		By("Ensuring that the server has Available state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
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
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
	})

	It("should move to completed if no bios setting changes to referred server", func(ctx SpecContext) {
		BIOSSetting := make(map[string]string)

		By("Ensuring that the server has Available state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
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
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the bios maintenance ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})

	It("should update the setting if setting changes requested with no reboot neeeded", func(ctx SpecContext) {
		BiosSetting := make(map[string]string)
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		BiosSetting["abc"] = "blahblah"

		// force BIOSSettings to not request maintenance
		By("update the server pwoerstate to On state")
		Eventually(UpdateStatus(server, func() {
			server.Status.PowerState = metalv1alpha1.ServerOnPowerState
		})).Should(Succeed())

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettings:            metalv1alpha1.Settings{Version: "P79 v1.45 (12/06/2017)", SettingsMap: BiosSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		// due to how the mocked setting is updated, the state transition are super fast
		By("Ensuring that the BIOS setting has reached next state: inProgress")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Ensuring that the Server has the bios setting ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettings.Name}),
		))

		By("Ensuring that the Maintenance resource has not been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		By("Ensuring that the BIOS setting has reached next state: stateSynced")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the Server BIOSSettings ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})

	It("should request maintenance when changing power status of server, even if bios settings does not need it", func(ctx SpecContext) {
		BiosSetting := make(map[string]string)
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		BiosSetting["abc"] = "blahblahreboottoturnserveron"

		// this is needed to transition through the unit test step by step
		// else, unit test finishes the state very fast without being able to check the transition
		serverClaim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-claim",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())
		By("Patching the Server to reserved state")
		Eventually(Update(server, func() {
			server.Spec.ServerClaimRef = &v1.ObjectReference{
				Name:      serverClaim.Name,
				Namespace: serverClaim.Namespace,
			}
			server.Spec.Power = metalv1alpha1.PowerOff
		})).Should(Succeed())
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateReserved
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettings:            metalv1alpha1.Settings{Version: "P79 v1.45 (12/06/2017)", SettingsMap: BiosSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOS setting has reached next state: inProgress")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		))

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      biosSettings.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by biosSettings")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:      "ServerMaintenance",
				Name:      serverMaintenance.Name,
				Namespace: serverMaintenance.Namespace,
				UID:       serverMaintenance.UID,
			}),
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
		))

		By("Ensuring that the BIOS setting has state: inProgress")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		))

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, "true")
		})).Should(Succeed())

		// because of how we mock the setting update, Hence check for multiple
		By("Ensuring that the BIOS setting has reached next state")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.UpdateSettingState", BeEmpty()),
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		// because of how we mock the setting update, it applied imedeiately and hence will not go through reboots to apply setting
		By("Ensuring that the BIOS setting has reached next state: Completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Ensuring that the BIOS setting has reached next state: Completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Ensuring that the BIOS setting has not referenced serverMaintenance anymore")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		By("Ensuring that the Maintenance resource has been deleted")
		Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the Server BIOSSettings ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})

	It("should create maintenance if setting update needs reboot", func(ctx SpecContext) {
		BiosSetting := make(map[string]string)
		BiosSetting["fooreboot"] = "144"

		// this is needed to transition through the unit test step by step
		// else, unit test finishes the state very fast without being able to check the transition
		serverClaim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-claim",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())
		By("Patching the Server to reserved state")
		Eventually(Update(server, func() {
			server.Spec.ServerClaimRef = &v1.ObjectReference{
				Name:      serverClaim.Name,
				Namespace: serverClaim.Namespace,
			}
			server.Spec.Power = metalv1alpha1.PowerOff
		})).Should(Succeed())
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateReserved
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-reboot-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettings:            metalv1alpha1.Settings{Version: "P79 v1.45 (12/06/2017)", SettingsMap: BiosSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOS setting has reached next state: inProgress")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		))

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      biosSettings.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by biosSettings")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:      "ServerMaintenance",
				Name:      serverMaintenance.Name,
				Namespace: serverMaintenance.Namespace,
				UID:       serverMaintenance.UID,
			}),
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
		))

		By("Ensuring that the BIOS setting has state: inProgress")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		))

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, "true")
		})).Should(Succeed())

		// because of how we mock the setting update, we can not determine the next state, Hence check for multiple
		By("Ensuring that the BIOS setting has reached next state")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.UpdateSettingState", BeEmpty()),
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		// because of how we mock the setting update, it applied immediately and hence will not go through reboots to apply setting
		// this is the eventual state we would need to reach
		By("Ensuring that the BIOS setting has reached next state: Completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Ensuring that the BIOS setting has not referenced serverMaintenance anymore")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		By("Ensuring that the Maintenance resource has been deleted")
		Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))
	})
})
