// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ironcore-dev/metal-operator/bmc"
)

var _ = Describe("BIOSSettings Controller", func() {
	ns := SetupTest()
	ns.Name = "default"

	var server *metalv1alpha1.Server
	var defaultPendingMockedBIOSSetting map[string]map[string]any

	BeforeEach(func(ctx SpecContext) {

		defaultPendingMockedBIOSSetting = bmc.DefaultMockedBIOSSetting

		By("Ensuring clean state")
		var serverList metalv1alpha1.ServerList
		Eventually(ObjectList(&serverList)).Should(HaveField("Items", (BeEmpty())))
		var biosList metalv1alpha1.BIOSSettingsList
		Eventually(ObjectList(&biosList)).Should(HaveField("Items", (BeEmpty())))

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
				GenerateName: "test-biossetting-maintenance-",
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
		bmc.PendingMockedBIOSSetting = map[string]map[string]any{}
		bmc.DefaultMockedBIOSSetting = defaultPendingMockedBIOSSetting
	})

	It("should successfully patch its reference to referred server", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		BIOSSetting := make(map[string]string) // no setting to apply

		By("Ensuring that the server has Available state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))
		By("Creating a BIOSSetting V1")
		biosSettingsV1 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-reference",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version:                 "P79 v1.45 (12/06/2017)",
				SettingsMap:             BIOSSetting,
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV1)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettingsV1.Name}),
		))

		Eventually(Object(biosSettingsV1)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Creating a BIOSSetting V2")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-reference-dup",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version:                 "P79 v2.45 (12/06/2017)",
				SettingsMap:             BIOSSetting,
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettingsV2.Name}),
		))

		Eventually(Object(biosSettingsV2)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Deleting the BIOSSettings V1 (old)")
		Expect(k8sClient.Delete(ctx, biosSettingsV1)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettingsV2.Name}),
		))

	})

	It("should move to completed if no bios setting changes to referred server", func(ctx SpecContext) {
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		BIOSSetting := make(map[string]string)
		BIOSSetting["abc"] = "bar"

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-no-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version:                 "P79 v1.45 (12/06/2017)",
				SettingsMap:             BIOSSetting,
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

		By("Ensuring that the bios ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})

	It("should update the setting without maintenance if setting requested needs no server reboot", func(ctx SpecContext) {
		BIOSSetting := make(map[string]string)
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		BIOSSetting["abc"] = "bar-changed-no-reboot"

		// mock BIOSSettings to not request maintenance by powering on the system (mock no need of power change on system)
		// note: cant be in Available state as it will power off automatically.
		serverClaim := GetServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOn, "foo:bar")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-change-noreboot-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version:                 "P79 v1.45 (12/06/2017)",
				SettingsMap:             BIOSSetting,
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

		By("Ensuring that the Server has correct state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettings.Name}),
			HaveField("Spec.Power", metalv1alpha1.PowerOn),
			HaveField("Status.PowerState", metalv1alpha1.ServerOnPowerState),
		))

		By("Ensuring that the Maintenance resource has not been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(Object(biosSettings)).Should(SatisfyAll(
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

	It("should request maintenance when changing power status of server, even if bios settings update does not need it", func(ctx SpecContext) {
		BIOSSetting := make(map[string]string)
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		BIOSSetting["abc"] = "bar-changed-to-turn-server-on"

		// put the server in Off state, to mock need of change in power state on server

		// Reserved state is needed to transition through the unit test step by step
		// else, unit test finishes the state very fast without being able to check the transition
		// creating OwnerApproval through reserved state gives more control when to approve the maintenance
		serverClaim := GetServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOff, "foo:bar")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-change-poweron",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version:                 "P79 v1.45 (12/06/2017)",
				SettingsMap:             BIOSSetting,
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
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.UpdateSettingState", metalv1alpha1.BIOSSettingUpdateState("")),
		))

		By("Ensuring that the Server is in correct power state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.PowerState", metalv1alpha1.ServerOnPowerState),
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

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the Server BIOSSettings ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})

	It("should create maintenance if setting update needs reboot", func(ctx SpecContext) {
		// settings which does need reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		BIOSSetting := make(map[string]string)
		BIOSSetting["fooreboot"] = "144"

		// put the server in reserved state,

		// Reserved state is needed to transition through the unit test step by step
		// else, unit test finishes the state very fast without being able to check the transition
		// creating OwnerApproval through reserved state gives more control when to approve the maintenance
		By("update the server powerstate to Off and reserved state")
		serverClaim := GetServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOff, "foo:bar")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-reboot-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version:                 "P79 v1.45 (12/06/2017)",
				SettingsMap:             BIOSSetting,
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
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.UpdateSettingState", metalv1alpha1.BIOSSettingUpdateState("")),
		))

		By("Ensuring that the Server is in Maintenance")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
		))

		// due to issue with serverClaim, which forces the power state on the server even during maintenance we need this
		By("Ensuring that the Server is in correct power state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.PowerState", metalv1alpha1.ServerOnPowerState),
		))

		By("Ensuring that the BIOS setting has reached next state: issue/reboot power off")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.UpdateSettingState", metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOff),
		))
		CheckServerPowerStateTransistionsDuringMaintenance(ctx, k8sClient, serverMaintenance, server, metalv1alpha1.PowerOff)
		By("Ensuring that the BIOS setting has reached next state: issue/reboot power on")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.UpdateSettingState", metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOn),
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
		if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
			CheckServerPowerStateTransistionsDuringMaintenance(ctx, k8sClient, serverMaintenance, server, metalv1alpha1.PowerOn)
		}

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

	It("should update setting if server is in availalbe state", func(ctx SpecContext) {
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		BIOSSetting := make(map[string]string)
		BIOSSetting["fooreboot"] = "10"

		By("Ensuring that the Server has available")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-from-server-avail",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version:                 "P79 v1.45 (12/06/2017)",
				SettingsMap:             BIOSSetting,
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOS setting has reached next state: inProgress")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		))

		By("Ensuring that the Server has the bios setting ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettings.Name}),
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

		By("Ensuring that the Server is in Maintenance")
		Eventually(Object(server)).WithTimeout(6 * time.Second).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
			HaveField("Status.PowerState", metalv1alpha1.ServerOnPowerState),
		))

		By("Ensuring that the BIOS setting has reached next state: issue/reboot power off")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.UpdateSettingState", metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOff),
		))
		CheckServerPowerStateTransistionsDuringMaintenance(ctx, k8sClient, serverMaintenance, server, metalv1alpha1.PowerOff)
		By("Ensuring that the BIOS setting has reached next state: issue/reboot power on")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.UpdateSettingState", metalv1alpha1.BIOSSettingUpdateWaitOnServerRebootPowerOn),
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
		if biosSettings.Status.State == metalv1alpha1.BIOSSettingsStateInProgress {
			CheckServerPowerStateTransistionsDuringMaintenance(ctx, k8sClient, serverMaintenance, server, metalv1alpha1.PowerOn)
		}
		// because of the mocking, the transistions are super fast here.
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the bios ref is empty")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})

	It("should wait for upgrade and reconcile when biosSettings version is correct", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)
		bmcSetting["abc"] = "bar-wait-on-version-upgrade"

		// put the server in PowerOn state,

		// Reserved state is needed to as Available state will turn off the power automatically.
		// powerOn is needed to skip the change in power on system, Hence skip maintenance.
		By("update the server powerstate to On and reserved state")
		serverClaim := GetServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOn, "foo:bar")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BMCSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-upgrade",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version:                 "2.45.455b66-rev4",
				SettingsMap:             bmcSetting,
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BMC has the correct BMC settings ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", Not(BeNil())),
			HaveField("Spec.BIOSSettingsRef.Name", biosSettings.Name),
		))

		By("Ensuring that the biosSettings resource state is correct State inVersionUpgrade")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		))

		By("Ensuring that the serverMaintenance not ref.")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))
		Consistently(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		By("Simulate the server biosSettings version update by matching the spec version")
		Eventually(Update(biosSettings, func() {
			biosSettings.Spec.Version = "P79 v1.45 (12/06/2017)"
		})).Should(Succeed())

		By("Ensuring that the biosSettings resource has setting updated, and moved the state")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
		// due to nature of mocking, we cant not determine few steps here. hence need a longer wait time
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		By("Ensuring that the serverMaintenance not ref.")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))
		Consistently(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		By("Deleting the BMCSetting resource")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the biosSettings resource is removed")
		Eventually(Get(biosSettings)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(biosSettings)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the Server biosSettings ref is empty on BMC")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
		Consistently(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		))
	})
})
