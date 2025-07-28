// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"time"

	"github.com/ironcore-dev/controller-utils/conditionutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BIOSSettings Controller", func() {
	ns := SetupTest()

	var (
		server *metalv1alpha1.Server
	)

	BeforeEach(func(ctx SpecContext) {
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
				GenerateName: "test-maintenance-",
			},
			Spec: metalv1alpha1.ServerSpec{
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
		bmc.UnitTestMockUps.ResetBIOSSettings()
	})

	It("should successfully patch its reference to referred server", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		biosSetting := make(map[string]string) // no setting to apply

		By("Ensuring that the server has Available state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		)
		By("Creating a BIOSSetting V1")
		biosSettingsV1 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-reference",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
					SettingsMap: biosSetting,
					Priority:    1,
				}},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV1)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettingsV1.Name}),
		)

		Eventually(Object(biosSettingsV1)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Ensuring right number of conditions are present")
		Eventually(
			func(g Gomega) int {
				g.Expect(Get(biosSettingsV1)()).To(Succeed())
				return len(biosSettingsV1.Status.Conditions)
			}).Should(BeNumerically("==", 1))

		By("Ensuring the update has been applied by the server")
		condVerifySettingsUpdate := &metav1.Condition{}
		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettingsV1)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettingsV1.Status.Conditions,
					fmt.Sprintf("%s-%d", verifySettingCondition, biosSettingsV1.Status.CurrentSettingPriority),
					condVerifySettingsUpdate)).To(BeTrue())
				return condVerifySettingsUpdate.Status == metav1.ConditionTrue
			}).Should(BeTrue())

		By("Creating a BIOSSetting V2")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-reference-dup",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: defaultMockUpServerBiosVersion + "2",
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
					SettingsMap: biosSetting,
					Priority:    1,
				}},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettingsV2.Name}),
		)

		Eventually(Object(biosSettingsV2)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Deleting the BIOSSettings V1 (old)")
		Expect(k8sClient.Delete(ctx, biosSettingsV1)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettingsV2.Name}),
		)

	})

	It("should move to completed if no bios setting changes to referred server", func(ctx SpecContext) {
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		biosSetting := make(map[string]string)
		biosSetting["abc"] = "bar"

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-no-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
					SettingsMap: biosSetting,
					Priority:    1,
				}},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the Server has the bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettings.Name}),
		)

		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Ensuring right number of conditions are present")
		Eventually(
			func(g Gomega) int {
				g.Expect(Get(biosSettings)()).To(Succeed())
				return len(biosSettings.Status.Conditions)
			}).Should(BeNumerically("==", 1))

		By("Ensuring the update has been applied by the server")
		condVerifySettingsUpdate := &metav1.Condition{}
		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettings)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", verifySettingCondition, biosSettings.Status.CurrentSettingPriority),
					condVerifySettingsUpdate)).To(BeTrue())
				return condVerifySettingsUpdate.Status == metav1.ConditionTrue
			}).Should(BeTrue())

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the bios ref is empty")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		)
	})

	It("should request maintenance when changing power status of server, even if bios settings update does not need it", func(ctx SpecContext) {
		biosSetting := make(map[string]string)
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		biosSetting["abc"] = "bar-changed-to-turn-server-on"

		// put the server in Off state, to mock need of change in power state on server

		// Reserved state is needed to transition through the unit test step by step
		// else, unit test finishes the state very fast without being able to check the transition
		// creating OwnerApproval through reserved state gives more control when to approve the maintenance
		serverClaim := BuildServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOff, "foo:bar")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-change-poweron",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
					SettingsMap: biosSetting,
					Priority:    1,
				}},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOS setting has reached next state: inProgress")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.LastAppliedTime", BeNil()),
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
		Eventually(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: metalv1alpha1.GroupVersion.String(),
			}),
		)

		By("Ensuring that the BIOS setting has state: inProgress")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		)

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, "true")
		})).Should(Succeed())

		By("Ensuring that the biosSettings resource has started bios setting update")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.LastAppliedTime", BeNil()),
		))

		By("Ensuring that the Server is in correct power state")
		Eventually(Object(server)).Should(
			HaveField("Status.PowerState", metalv1alpha1.ServerOnPowerState),
		)

		// because of how we mock the setting update, it applied immediately and hence will not go through reboots to apply setting
		// this is the eventual state we would need to reach
		By("Ensuring that the BIOS setting has reached next state: Completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Ensuring that the BIOS setting has not referenced serverMaintenance anymore")
		Eventually(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		By("Ensuring that the BIOS setting has right conditions")
		ensureBiosSettingsCondition(biosSettings, false, false)

		By("Ensuring that the Maintenance resource has been deleted")
		Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the Server BIOSSettings ref is empty")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		)
	})

	It("should create maintenance if setting update needs reboot", func(ctx SpecContext) {
		// settings which does need reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		biosSetting := make(map[string]string)
		biosSetting["fooreboot"] = "144"

		// put the server in reserved state,

		// Reserved state is needed to transition through the unit test step by step
		// else, unit test finishes the state very fast without being able to check the transition
		// creating OwnerApproval through reserved state gives more control when to approve the maintenance
		By("update the server powerstate to Off and reserved state")
		serverClaim := BuildServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOff, "foo:bar")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-reboot-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
					SettingsMap: biosSetting,
					Priority:    1,
				}},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOS setting has reached next state: inProgress")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.LastAppliedTime", BeNil()),
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
		Eventually(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: metalv1alpha1.GroupVersion.String(),
			}),
		)

		By("Ensuring that the BIOS setting has state: inProgress")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		)

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, "true")
		})).Should(Succeed())

		By("Ensuring that the biosSettings resource has started bios setting update")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		)

		By("Ensuring that the Server is in Maintenance")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
		)

		// because of how we mock the setting update, it applied immediately and hence will not go through reboots to apply setting
		// this is the eventual state we would need to reach
		By("Ensuring that the BIOS setting has reached next state: Completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Ensuring that the BIOS setting has not referenced serverMaintenance anymore")
		Eventually(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		By("Ensuring that the BIOS setting has right conditions")
		ensureBiosSettingsCondition(biosSettings, true, false)

		By("Ensuring that the Maintenance resource has been deleted")
		Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should update setting if server is in available state", func(ctx SpecContext) {
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		biosSetting := make(map[string]string)
		biosSetting["fooreboot"] = "10"

		// just to double confirm the starting state here for easy readability
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
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
					SettingsMap: biosSetting,
					Priority:    1,
				}},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOS setting has reached next state: inProgress")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.LastAppliedTime", BeNil()),
		))

		By("Ensuring that the Server has the bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", &v1.LocalObjectReference{Name: biosSettings.Name}),
		)

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
		Eventually(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: metalv1alpha1.GroupVersion.String(),
			}),
		)

		By("Ensuring that the Server is in Maintenance")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
		)

		By("Ensuring that the BIOS setting has reached next state: issue/reboot or verification state")
		// check condition

		// because of the mocking, the transistions are super fast here.
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Ensuring that the BIOS setting has right conditions")
		ensureBiosSettingsCondition(biosSettings, true, false)

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the bios ref is empty")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		)
	})

	It("should wait for upgrade and reconcile when biosSettings version is correct", func(ctx SpecContext) {
		biosSetting := make(map[string]string)
		biosSetting["abc"] = "bar-wait-on-version-upgrade"

		// put the server in PowerOn state,

		// Reserved state is needed to as Available state will turn off the power automatically.
		// powerOn is needed to skip the change in power on system, Hence skip maintenance.
		By("update the server powerstate to On and reserved state")
		serverClaim := BuildServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOn, "foo:bar")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BMCSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-upgrade-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: "2.45.455b66-rev4",
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
					SettingsMap: biosSetting,
					Priority:    1,
				}},
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
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStatePending),
			HaveField("Status.LastAppliedTime", BeNil()),
		))

		By("Ensuring that the serverMaintenance not ref.")
		Eventually(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		Consistently(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)

		By("Simulate the server biosSettings version update by matching the spec version")
		Eventually(Update(biosSettings, func() {
			biosSettings.Spec.Version = defaultMockUpServerBiosVersion
		})).Should(Succeed())

		By("Ensuring that the biosSettings resource has setting updated, and moved the state")
		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
		// due to nature of mocking, we cant not determine few steps here. hence need a longer wait time
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Ensuring that the serverMaintenance not ref.")
		Eventually(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		Consistently(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		By("Ensuring that the BIOS setting has right conditions")
		ensureBiosSettingsCondition(biosSettings, false, true)

		By("Deleting the BMCSetting resource")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the biosSettings resource is removed")
		Eventually(Get(biosSettings)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(biosSettings)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the Server biosSettings ref is empty on BMC")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		)
		Consistently(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		)
	})
})

var _ = Describe("BIOSSettings Sequence Controller", func() {
	ns := SetupTest()

	var (
		server *metalv1alpha1.Server
	)

	BeforeEach(func(ctx SpecContext) {
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
				GenerateName: "test-maintenance-",
			},
			Spec: metalv1alpha1.ServerSpec{
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
		bmc.UnitTestMockUps.ResetBIOSSettings()
	})

	It("should successfully apply sequence of settings", func(ctx SpecContext) {

		By("Creating a BIOSSetting with sequence of settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{
					{
						Priority:    100,
						SettingsMap: map[string]string{"fooreboot": "10"},
					},
					{
						Priority:    1000,
						SettingsMap: map[string]string{"fooreboot": "100"},
					},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).WithTimeout(5 * time.Second).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
		By("Ensuring that the BIOSSettings conditions are updated")
		ensureBiosSettingsFlowCondition(biosSettings)

		By("Deleting the BIOSSettingFlow")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
	})

	It("should successfully apply sequence of different settings", func(ctx SpecContext) {

		By("Creating a BIOSSetting sequence of settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-differnet-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{
					{
						Priority:    100,
						SettingsMap: map[string]string{"abc": "foo-bar"},
					},
					{
						Priority:    1000,
						SettingsMap: map[string]string{"fooreboot": "100"},
					},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
		By("Ensuring that the BIOSSettings conditions are updated")
		ensureBiosSettingsFlowCondition(biosSettings)

		By("Deleting the BIOSSettingFlow")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
	})

	It("should successfully apply sequence of different settings", func(ctx SpecContext) {

		By("Creating a BIOSSetting sequence of settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-differnet-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{
					{
						Priority:    100,
						SettingsMap: map[string]string{"abc": "foo-bar"},
					},
					{
						Priority:    1000,
						SettingsMap: map[string]string{"fooreboot": "100"},
					},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
		By("Ensuring that the BIOSSettings conditions are updated")
		ensureBiosSettingsFlowCondition(biosSettings)

		By("Deleting the BIOSSettingFlow")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
	})

	It("should Fail when sequence of settings has duplicate priority", func(ctx SpecContext) {

		By("Creating a BIOSSetting sequence of settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-differnet-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{
					{
						Priority:    5,
						SettingsMap: map[string]string{"abc": "foo-bar"},
					},
					{
						Priority:    5,
						SettingsMap: map[string]string{"fooreboot": "100"},
					},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateFailed),
		))
	})
})

func ensureBiosSettingsFlowCondition(
	biosSettings *metalv1alpha1.BIOSSettings,
) {
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	requiredCondition := 7
	condMaintenanceDeleted := &metav1.Condition{}
	condMaintenanceCreated := &metav1.Condition{}
	condIssueSettingsUpdate := &metav1.Condition{}
	condVerifySettingsUpdate := &metav1.Condition{}
	condServerPoweredOn := &metav1.Condition{}
	condSkipReboot := &metav1.Condition{}

	condTimerStarted := &metav1.Condition{}

	condPendingVersionUpdate := &metav1.Condition{}

	By("Ensuring right number of conditions are present")
	Eventually(
		func(g Gomega) int {
			g.Expect(Get(biosSettings)()).To(Succeed())
			return len(biosSettings.Status.Conditions)
		}).Should(BeNumerically(">=", requiredCondition*len(biosSettings.Spec.SettingsFlow)))

	By(fmt.Sprintf("Ensuring the wait for version upgrade condition has NOT been added %v", condPendingVersionUpdate.Status))
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions, pendingVersionUpdateCondition, condPendingVersionUpdate)).To(BeFalse())
			return condPendingVersionUpdate.Status == ""
		}).Should(BeTrue())

	By("Ensuring the serverMaintenance condition has been created")
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions, serverMaintenanceCreatedCondition, condMaintenanceCreated)).To(BeTrue())
			return condMaintenanceCreated.Status == metav1.ConditionTrue
		}).Should(BeTrue())

	for _, settings := range biosSettings.Spec.SettingsFlow {
		By(fmt.Sprintf("Ensuring the BIOSSettings Object has applied following settings %v", settings.SettingsMap))
		By("Ensuring the timeout error start time has been recorded")
		Eventually(
			func(g Gomega) {
				g.Expect(Get(biosSettings)()).To(Succeed())
				By("Ensuring the timeout error start time has been recorded")
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", timeoutStartCondition, settings.Priority),
					condTimerStarted)).To(BeTrue())
				g.Expect(condTimerStarted.Status).To(Equal(metav1.ConditionTrue))

				By("Ensuring the server has been powered on at the start")
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", turnServerOnCondition, settings.Priority),
					condServerPoweredOn)).To(BeTrue())
				g.Expect(condServerPoweredOn.Status).To(Equal(metav1.ConditionTrue))

				By("Ensuring the check if reboot of server required has been completed")
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", skipRebootCondition, biosSettings.Status.CurrentSettingPriority),
					condSkipReboot)).To(BeTrue())

				By("Ensuring the update has been issue to the server")
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", issueSettingsUpdateCondition, settings.Priority),
					condIssueSettingsUpdate)).To(BeTrue())
				g.Expect(condIssueSettingsUpdate.Status).To(Equal(metav1.ConditionTrue))

				By("Ensuring the update has been applied by the server")
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", verifySettingCondition, settings.Priority),
					condVerifySettingsUpdate)).To(BeTrue())
				g.Expect(condVerifySettingsUpdate.Status).To(Equal(metav1.ConditionTrue))
			}).Should(Succeed())
	}

	By("Ensuring the server maintenance has been deleted")
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions, serverMaintenanceDeletedCondition, condMaintenanceDeleted)).To(BeTrue())
			return condMaintenanceDeleted.Status == metav1.ConditionTrue
		}).Should(BeTrue())
}

func ensureBiosSettingsCondition(
	biosSettings *metalv1alpha1.BIOSSettings,
	RebootNeeded bool,
	waitForVersionUpgrade bool,
) {
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	requiredCondition := 7
	condMaintenanceDeleted := &metav1.Condition{}
	condMaintenanceCreated := &metav1.Condition{}
	condIssueSettingsUpdate := &metav1.Condition{}
	condVerifySettingsUpdate := &metav1.Condition{}
	condServerPoweredOn := &metav1.Condition{}
	condSkipReboot := &metav1.Condition{}

	condTimerStarted := &metav1.Condition{}

	condPendingVersionUpdate := &metav1.Condition{}
	condRebootPowerOn := &metav1.Condition{}
	condRebootPowerOff := &metav1.Condition{}

	if RebootNeeded {
		requiredCondition += 2
	}

	if waitForVersionUpgrade {
		requiredCondition += 1
	}

	By("Ensuring right number of conditions are present")
	Eventually(
		func(g Gomega) int {
			g.Expect(Get(biosSettings)()).To(Succeed())
			return len(biosSettings.Status.Conditions)
		}).Should(BeNumerically("==", requiredCondition))

	if waitForVersionUpgrade {
		By("Ensuring the wait for version upgrade condition has been added")
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettings)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions, pendingVersionUpdateCondition, condPendingVersionUpdate)).To(BeTrue())
				return condPendingVersionUpdate.Status == metav1.ConditionTrue
			}).Should(BeTrue())
	} else {
		By(fmt.Sprintf("Ensuring the wait for version upgrade condition has NOT been added %v", condPendingVersionUpdate.Status))
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettings)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions, pendingVersionUpdateCondition, condPendingVersionUpdate)).To(BeFalse())
				return condPendingVersionUpdate.Status == ""
			}).Should(BeTrue())
	}

	By("Ensuring the serverMaintenance condition has been created")
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions, serverMaintenanceCreatedCondition, condMaintenanceCreated)).To(BeTrue())
			return condMaintenanceCreated.Status == metav1.ConditionTrue
		}).Should(BeTrue())

	By("Ensuring the timeout error start time has been recorded")
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
				fmt.Sprintf("%s-%d", timeoutStartCondition, biosSettings.Status.CurrentSettingPriority),
				condTimerStarted)).To(BeTrue())
			return condTimerStarted.Status == metav1.ConditionTrue
		}).Should(BeTrue())

	By("Ensuring the server has been powered on at the start")
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
				fmt.Sprintf("%s-%d", turnServerOnCondition, biosSettings.Status.CurrentSettingPriority),
				condServerPoweredOn)).To(BeTrue())
			return condServerPoweredOn.Status == metav1.ConditionTrue
		}).Should(BeTrue())

	if !RebootNeeded {
		By("Ensuring the server skip reboot check has been created and skips reboot")
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettings)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", skipRebootCondition, biosSettings.Status.CurrentSettingPriority),
					condSkipReboot)).To(BeTrue())
				return condSkipReboot.Status == metav1.ConditionTrue
			}).Should(BeTrue())
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettings)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", rebootPowerOffCondition, biosSettings.Status.CurrentSettingPriority),
					condRebootPowerOff)).To(BeFalse())
				return condRebootPowerOff.Status == ""
			}).Should(BeTrue())
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettings)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", rebootPowerOnCondition, biosSettings.Status.CurrentSettingPriority),
					condRebootPowerOn)).To(BeFalse())
				return condRebootPowerOn.Status == ""
			}).Should(BeTrue())
	} else {
		By("Ensuring the server skip reboot check has been created and reboots the server")
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettings)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", skipRebootCondition, biosSettings.Status.CurrentSettingPriority),
					condSkipReboot)).To(BeTrue())
				return condSkipReboot.Status == metav1.ConditionFalse
			}).Should(BeTrue())
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettings)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", rebootPowerOffCondition, biosSettings.Status.CurrentSettingPriority),
					condRebootPowerOff)).To(BeTrue())
				return condRebootPowerOff.Status == metav1.ConditionTrue
			}).Should(BeTrue())
		Eventually(
			func(g Gomega) bool {
				g.Expect(Get(biosSettings)()).To(Succeed())
				g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
					fmt.Sprintf("%s-%d", rebootPowerOnCondition, biosSettings.Status.CurrentSettingPriority),
					condRebootPowerOn)).To(BeTrue())
				return condRebootPowerOn.Status == metav1.ConditionTrue
			}).Should(BeTrue())
	}
	By("Ensuring the update has been issue to the server")
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
				fmt.Sprintf("%s-%d", issueSettingsUpdateCondition, biosSettings.Status.CurrentSettingPriority),
				condIssueSettingsUpdate)).To(BeTrue())
			return condIssueSettingsUpdate.Status == metav1.ConditionTrue
		}).Should(BeTrue())
	By("Ensuring the update has been applied by the server")
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
				fmt.Sprintf("%s-%d", verifySettingCondition, biosSettings.Status.CurrentSettingPriority),
				condVerifySettingsUpdate)).To(BeTrue())
			return condVerifySettingsUpdate.Status == metav1.ConditionTrue
		}).Should(BeTrue())
	By("Ensuring the server maintenance has been deleted")
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions, serverMaintenanceDeletedCondition, condMaintenanceDeleted)).To(BeTrue())
			return condMaintenanceDeleted.Status == metav1.ConditionTrue
		}).Should(BeTrue())
}
