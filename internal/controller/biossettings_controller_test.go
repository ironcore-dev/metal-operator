// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BIOSSettings Controller", func() {
	ns := SetupTest(nil)

	var (
		server      *metalv1alpha1.Server
		bmcSecret   *metalv1alpha1.BMCSecret
		serverClaim *metalv1alpha1.ServerClaim
	)

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret = &metalv1alpha1.BMCSecret{
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
						Port: MockServerPort,
					},
					Address: MockServerIP,
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Ensuring that the Server is in available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		bmc.UnitTestMockUps.ResetBIOSSettings()

		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		EnsureCleanState()
	})

	It("Should successfully patch its reference to referred server", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
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
			// settings mocked at
			// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV1)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef.Name", biosSettingsV1.Name),
		)

		By("Ensuring that the BIOS setting has reached next state: Applied")
		Eventually(Object(biosSettingsV1)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Ensuring the right number of conditions are present")
		Eventually(Object(biosSettingsV1)).Should(
			HaveField("Status.Conditions", HaveLen(1)),
		)

		By("Ensuring the update has been applied by the server")
		Eventually(Object(biosSettingsV1)).Should(
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingsConditionVerifySettings),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
		)

		By("Creating a BIOSSetting V2")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-reference-dup",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion + "2",
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef.Name", biosSettingsV2.Name),
		)

		Eventually(Object(biosSettingsV2)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Deleting the BIOSSettings V1 (old)")
		Expect(k8sClient.Delete(ctx, biosSettingsV1)).To(Succeed())

		By("Ensuring that the Server has the correct server bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef.Name", biosSettingsV2.Name),
		)
		By("Deleting the BIOSSettings V2 (new)")
		Expect(k8sClient.Delete(ctx, biosSettingsV2)).To(Succeed())
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		)
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should move to completed if no bios setting changes to referred server", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
		biosSetting := make(map[string]string)
		biosSetting["EmbeddedSata"] = "Raid"

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-no-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the Server has the bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef.Name", biosSettings.Name),
		)

		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Ensuring right number of conditions are present")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.Conditions", HaveLen(1)),
		)

		By("Ensuring the update has been applied by the server")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingsConditionVerifySettings),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
		)

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the bios ref is empty")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		)
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should reboot server if the resetRequired field is missing in the biosRegistry", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
		biosSetting := make(map[string]string)
		biosSetting["EmbeddedSata"] = "NonRaid"

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-no-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the Server has the bios setting ref")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef.Name", biosSettings.Name),
		)

		Eventually(Object(biosSettings)).Should(
			HaveField("Status.FlowState", ContainElement(SatisfyAll(
				HaveField("Conditions", ContainElement(SatisfyAll(
					HaveField("Type", BIOSSettingsConditionRebootPostUpdate),
					HaveField("Reason", BIOSSettingsReasonRebootNeeded),
					HaveField("Status", metav1.ConditionFalse)),
				)),
				HaveField("Name", "one"),
			))),
		)

		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the bios ref is empty")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		)
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should request maintenance when changing power status of server, even if bios settings update does not need it", func(ctx SpecContext) {
		biosSetting := make(map[string]string)
		// settings mocked at
		// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
		biosSetting["AdminPhone"] = "bar-changed-to-turn-server-on"

		// put the server in Off state, to mock need of change in power state on server

		// Reserved state is needed to transition through the unit test step by step
		// else, unit test finishes the state very fast without being able to check the transition
		// creating OwnerApproval through reserved state gives more control when to approve the maintenance
		By("Creating an Ignition secret")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: nil,
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

		By("Creating a ServerClaim")
		serverClaim = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:             metalv1alpha1.PowerOff,
				ServerRef:         &v1.LocalObjectReference{Name: server.Name},
				IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
				Image:             "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())

		By("Ensuring that the Server has been claimed")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		)

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-change-poweron",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
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
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, trueValue)
			metautils.SetLabel(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, trueValue)
		})).Should(Succeed())

		By("Ensuring that the biosSettings resource has started bios setting update")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.LastAppliedTime", BeNil()),
		))

		Eventually(Object(serverMaintenance)).Should(
			HaveField("Status.State", metalv1alpha1.ServerMaintenanceStateInMaintenance),
		)

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

		// cleanup
		Expect(k8sClient.Delete(ctx, serverClaim)).Should(Succeed())
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateReserved))),
		))
	})

	It("Should create maintenance if setting update needs reboot", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
		biosSetting := make(map[string]string)
		biosSetting["PowerProfile"] = "SysDbpm"

		// put the server in reserved state,

		// Reserved state is needed to transition through the unit test step by step
		// else, unit test finishes the state very fast without being able to check the transition
		// creating OwnerApproval through reserved state gives more control when to approve the maintenance
		By("Creating an Ignition secret")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: nil,
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

		By("Creating a ServerClaim")
		serverClaim = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:             metalv1alpha1.PowerOff,
				ServerRef:         &v1.LocalObjectReference{Name: server.Name},
				IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
				Image:             "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())

		By("Ensuring that the Server has been claimed")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		)

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-reboot-change",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
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
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, trueValue)
			metautils.SetLabel(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, trueValue)
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

		// cleanup
		Expect(k8sClient.Delete(ctx, biosSettings)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, serverClaim)).Should(Succeed())
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateReserved))),
		))
	})

	It("Should update setting if server is in available state", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
		biosSetting := make(map[string]string)
		biosSetting["PowerProfile"] = "OsDbpm"

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
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
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

		// because of the mocking, the transitions are superfast here.
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		By("Ensuring that the BIOS setting has right conditions")
		ensureBiosSettingsCondition(biosSettings, true, false)

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
		Eventually(Get(biosSettings)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the bios ref is empty")
		Eventually(Object(server)).Should(
			HaveField("Spec.BIOSSettingsRef", BeNil()),
		)
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should wait for upgrade and reconcile when biosSettings version is correct", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
		biosSetting := make(map[string]string)
		biosSetting["AdminPhone"] = "bar-wait-on-version-upgrade"

		// put the server in PowerOn state,

		// Reserved state is needed to as Available state will turn off the power automatically.
		// powerOn is needed to skip the change in power on system, Hence skip maintenance.
		By("Creating an Ignition secret")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: nil,
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

		By("Creating a ServerClaim")
		serverClaim = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:             metalv1alpha1.PowerOn,
				ServerRef:         &v1.LocalObjectReference{Name: server.Name},
				IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
				Image:             "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())

		By("Ensuring that the Server has been claimed")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		)

		By("Creating a BMCSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-upgrade-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: "2.45.455b66-rev4",
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
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

		// cleanup
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should allow retry using annotation", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
		biosSetting := make(map[string]string)
		biosSetting["ProcCores"] = "2"

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-from-server-avail",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Moving to Failed state")
		Eventually(UpdateStatus(biosSettings, func() {
			biosSettings.Status.State = metalv1alpha1.BIOSSettingsStateFailed
		})).Should(Succeed())

		Eventually(Update(biosSettings, func() {
			biosSettings.Annotations = map[string]string{
				metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationRetryFailed,
			}
		})).Should(Succeed())

		Eventually(Object(biosSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
		Eventually(Get(biosSettings)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should replace missing BIOSSettings ref in server", func(ctx SpecContext) {
		// settings mocked at
		// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
		biosSetting := make(map[string]string)
		biosSetting["ProcCores"] = "2"

		By("Creating a BIOSSetting")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-from-server-avail-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Wait for the BIOSSettings to be ref on the Server")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", Not(BeNil())),
			HaveField("Spec.BIOSSettingsRef.Name", biosSettings.Name),
		))
		// delete the old settings
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
		By("force deletion")
		Eventually(func() error {
			err := Update(biosSettings, func() {
				biosSettings.Finalizers = []string{}
			})()
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}).Should(Succeed())
		By("check if maintenance has been created on the server and delete if its present")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(func() error {
			_, err := ObjectList(&serverMaintenanceList)()
			if err != nil {
				return err
			}
			if len(serverMaintenanceList.Items) > 0 {
				for _, item := range serverMaintenanceList.Items {
					if len(item.OwnerReferences) > 0 && item.OwnerReferences[0].UID == biosSettings.UID {
						By(fmt.Sprintf("Deleting the ServerMaintenance created by biosSettings %v", item.Name))
						Expect(k8sClient.Delete(ctx, &item)).To(Succeed())
						Eventually(func() error {
							err := Update(&item, func() {
								item.Finalizers = []string{}
							})()
							if apierrors.IsNotFound(err) {
								return nil
							}
							return err
						}).Should(Succeed())
					}
				}
			}
			return nil
		}).Should(Succeed())

		biosSettings2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-from-server-recreated-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings2)).To(Succeed())

		By("Wait for the BIOSSettings2 to be ref on the Server")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BIOSSettingsRef", Not(BeNil())),
			HaveField("Spec.BIOSSettingsRef.Name", biosSettings2.Name),
		))

		Eventually(Object(biosSettings2)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))

		Eventually(Object(biosSettings2)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.LastAppliedTime.IsZero()", false),
		))

		Expect(k8sClient.Delete(ctx, biosSettings2)).To(Succeed())
		Eventually(Get(biosSettings2)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})
})

var _ = Describe("BIOSSettings Controller with BMCRef BMC", func() {
	ns := SetupTest(nil)

	var (
		server      *metalv1alpha1.Server
		bmcObj      *metalv1alpha1.BMC
		bmcSecret   *metalv1alpha1.BMCSecret
		serverClaim *metalv1alpha1.ServerClaim
	)
	BeforeEach(func(ctx SpecContext) {
		bmcSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-secret-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		bmcObj = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "23:11:8A:33:CF:EA",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcObj)).To(Succeed())

		By("Ensuring that the Server resource will be created")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmcObj),
			},
		}
		Eventually(Get(server)).Should(Succeed())

		By("Ensuring that the Server is in available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		bmc.UnitTestMockUps.ResetBIOSSettings()

		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcObj)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		EnsureCleanState()
	})

	It("Should request maintenance when changing power status of server, even if bios settings update does not need it", func(ctx SpecContext) {
		biosSetting := make(map[string]string)
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBIOSSetting
		biosSetting["AdminPhone"] = "bar-changed-to-turn-server-on-bmcref"

		// put the server in Off state, to mock need of change in power state on server

		// Reserved state is needed to transition through the unit test step by step
		// else, unit test finishes the state very fast without being able to check the transition
		// creating OwnerApproval through reserved state gives more control when to approve the maintenance
		By("Creating an Ignition secret")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: nil,
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

		By("Creating a ServerClaim")
		serverClaim = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:             metalv1alpha1.PowerOff,
				ServerRef:         &v1.LocalObjectReference{Name: server.Name},
				IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
				Image:             "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())

		By("Ensuring that the Server has been claimed")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		)

		By("Creating a BIOS settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bios-change-poweron",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
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

		// cleanup
		Expect(k8sClient.Delete(ctx, serverClaim)).Should(Succeed())
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateReserved))),
		))
	})

	It("Should fail and create maintenance when pending settings exist", func(ctx SpecContext) {
		By("Ensuring BMC is in Enabled state")
		Eventually(UpdateStatus(bmcObj, func() {
			bmcObj.Status.State = metalv1alpha1.BMCStateEnabled
		})).To(Succeed())

		biosSetting := make(map[string]string)
		biosSetting["EmbeddedSata"] = "Raid"

		By("Creating an Ignition secret")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: nil,
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

		By("Creating a ServerClaim")
		serverClaim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:             metalv1alpha1.PowerOff,
				ServerRef:         &v1.LocalObjectReference{Name: server.Name},
				IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
				Image:             "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())

		By("Ensuring that the Server has been claimed")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		)

		By("Creating pending changes on the mock server via HTTP PATCH")
		pendingSettingsURL := fmt.Sprintf("http://%s:%d/redfish/v1/Systems/437XR1138R2/Bios/Settings", MockServerIP, MockServerPort)
		pendingData := map[string]any{
			"Attributes": map[string]any{
				"PendingSetting1": "PendingValue1",
			},
		}
		pendingBody, err := json.Marshal(pendingData)
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, pendingSettingsURL, bytes.NewReader(pendingBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{
			Timeout: 5 * time.Second,
		}
		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(resp.Body.Close)
		Expect(resp.StatusCode).To(BeElementOf(http.StatusOK, http.StatusNoContent, http.StatusAccepted))

		By("Creating BIOSSettings to apply new changes")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-pending-changes",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: biosSetting,
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring BIOSSettings transitions to Failed state")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateFailed),
		)

		By("Verifying ServerMaintenance object was created")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      biosSettings.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Verifying BIOSSettings has maintenance reference")
		Eventually(Object(biosSettings)).Should(
			HaveField("Spec.ServerMaintenanceRef", Not(BeNil())),
		)

		By("Clearing pending changes on the mock server via HTTP DELETE")
		clearPendingURL := fmt.Sprintf("http://%s:%d/redfish/v1/Systems/437XR1138R2/Bios/Settings", MockServerIP, MockServerPort)
		clearReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, clearPendingURL, nil)
		Expect(err).NotTo(HaveOccurred())

		clearResp, err := client.Do(clearReq)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(clearResp.Body.Close)
		Expect(clearResp.StatusCode).To(BeElementOf(http.StatusOK, http.StatusNoContent, http.StatusAccepted))

		By("Cleaning up BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())

		By("Cleaning up ServerMaintenance")
		Expect(k8sClient.Delete(ctx, serverMaintenance)).Should(Succeed())

		By("Waiting for Server to exit Maintenance state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)

		By("Cleaning up ServerClaim")
		Expect(k8sClient.Delete(ctx, serverClaim)).Should(Succeed())

		By("Cleaning up Ignition secret")
		Expect(k8sClient.Delete(ctx, ignitionSecret)).Should(Succeed())
	})
})

var _ = Describe("BIOSSettings Sequence Controller", func() {
	ns := SetupTest(nil)

	var (
		server    *metalv1alpha1.Server
		bmcSecret *metalv1alpha1.BMCSecret
	)

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret = &metalv1alpha1.BMCSecret{
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
						Port: MockServerPort,
					},
					Address: MockServerIP,
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Ensure that the Server is in available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		bmc.UnitTestMockUps.ResetBIOSSettings()

		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		EnsureCleanState()
	})

	It("Should successfully apply sequence of settings", func(ctx SpecContext) {
		By("Creating a BIOSSetting with sequence of settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-",
			},
			// settings mocked at
			// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{
						{
							Priority: 100,
							Settings: map[string]string{"AdminPhone": "1010101"},
							Name:     "100",
						},
						{
							Priority: 1000,
							Settings: map[string]string{"PowerProfile": "MaxPerf"},
							Name:     "1000",
						},
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.FlowState", HaveLen(len(biosSettings.Spec.SettingsFlow))),
		))
		By("Ensuring that the BIOSSettings conditions are updated")
		ensureBiosSettingsFlowCondition(biosSettings)

		By("Deleting the BIOSSetting")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should fail if settings provide in inValid", func(ctx SpecContext) {
		By("Creating a BIOSSetting with sequence of settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-",
			},
			// settings mocked at
			// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{
						{
							Priority: 100,
							Settings: map[string]string{"PowerProfile": "UnKnownValue"},
							Name:     "100",
						},
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to Failed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateFailed),
		))

		By("Ensuring the Condition has been saved")
		Eventually(Object(biosSettings)).Should(
			SatisfyAll(
				HaveField("Status.FlowState", ContainElement(
					SatisfyAll(
						HaveField("Conditions", ContainElement(
							SatisfyAll(
								HaveField("Type", BIOSSettingsConditionWrongSettings),
								HaveField("Reason", BIOSSettingsReasonWrongSettings),
							),
						)),
						HaveField("Name", "100"),
						HaveField("State", metalv1alpha1.BIOSSettingsFlowStateFailed),
					),
				)),
			),
		)

		By("Deleting the biosSettings")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      biosSettings.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
		Expect(k8sClient.Delete(ctx, serverMaintenance)).To(Succeed())
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should fail if duplicate keys in names or settings found", func(ctx SpecContext) {
		By("Creating a BIOSSetting with sequence of settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-",
			},
			// settings mocked at
			// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{
						{
							Priority: 100,
							Settings: map[string]string{"PowerProfile": "SysDbpm"},
							Name:     "100",
						},
						{
							Priority: 1000,
							Settings: map[string]string{"PowerProfile": "OsDbpm"},
							Name:     "1000",
						},
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to Failed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateFailed),
		))

		By("Ensuring right number of conditions are present")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.Conditions", HaveLen(1)),
		)

		By("Ensuring the update has been applied by the server")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingsConditionDuplicateKey),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
		)

		biosSettings2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-",
			},
			// settings mocked at
			// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{
						{
							Priority: 100,
							Settings: map[string]string{"AdminPhone": "123-456"},
							Name:     "100",
						},
						{
							Priority: 1000,
							Settings: map[string]string{"PowerProfile": "OsDbpm"},
							Name:     "100",
						},
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings2)).To(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to Failed")
		Eventually(Object(biosSettings2)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateFailed),
		))

		By("Ensuring right number of conditions are present")
		Eventually(Object(biosSettings2)).Should(
			HaveField("Status.Conditions", HaveLen(1)),
		)

		By("Ensuring the update has been applied by the server")
		Eventually(Object(biosSettings2)).Should(
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingsConditionDuplicateKey),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
		)

		By("Deleting the biosSettings2")
		Expect(k8sClient.Delete(ctx, biosSettings2)).To(Succeed())
		By("Deleting the biosSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should successfully apply sequence of different settings and reconcile from applied state", func(ctx SpecContext) {
		By("Creating a BIOSSetting sequence of settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-differnet-",
			},
			// settings mocked at
			// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{
						{
							Priority: 100,
							Settings: map[string]string{"AdminPhone": "123-123"},
							Name:     "100",
						},
						{
							Priority: 1000,
							Settings: map[string]string{"PowerProfile": "SysDbpm"},
							Name:     "1000",
						},
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.FlowState", HaveLen(len(biosSettings.Spec.SettingsFlow))),
		))
		By("Ensuring that the BIOSSettings conditions are updated")
		ensureBiosSettingsFlowCondition(biosSettings)

		// move server back to available state (to avoid initial/discovery state loop)
		By("Ensure that the Server is in available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		// should reconcile again from the Applied state when the settings has been changed
		Eventually(Update(biosSettings, func() {
			biosSettings.Spec.SettingsFlow[1].Settings = map[string]string{"PowerProfile": "OsDbpm"}
		})).Should(Succeed())

		By("Ensuring that the BIOSSetting Object has moved to out of completed")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		)

		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.FlowState", HaveLen(len(biosSettings.Spec.SettingsFlow))),
		))

		By("Deleting the BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should successfully apply sequence of settings when the names and priority changed, before the settings update was issued on server", func(ctx SpecContext) {
		newNames := []string{"1", "10"}
		oldNames := []string{"100", "1000"}
		By("Creating a BIOSSetting with sequence of settings")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-",
			},
			// settings mocked at
			// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
			Spec: metalv1alpha1.BIOSSettingsSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{
						{
							Priority: 100,
							Settings: map[string]string{"AdminPhone": "one-two-three"},
							Name:     oldNames[0],
						},
						{
							Priority: 1000,
							Settings: map[string]string{"ProcCores": "1"},
							Name:     oldNames[1],
						},
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettings)).To(Succeed())

		Eventually(Object(biosSettings)).WithPolling(1 * time.Microsecond).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateInProgress),
		)

		Eventually(Object(biosSettings)).WithPolling(1 * time.Microsecond).Should(
			SatisfyAll(
				HaveField("Status.FlowState", Not(ContainElement(
					SatisfyAll(
						HaveField("Conditions", ContainElement(
							HaveField("Type", BIOSSettingsConditionIssuedUpdate),
						)),
						HaveField("Name", oldNames[1]),
					),
				))),
			),
		)

		Eventually(Update(biosSettings, func() {
			biosSettings.Spec.SettingsFlow = []metalv1alpha1.SettingsFlowItem{
				{
					Priority: 1000,
					Settings: map[string]string{"AdminPhone": "three-two-one"},
					Name:     newNames[0],
				},
				{
					Priority: 100,
					Settings: map[string]string{"ProcCores": "2"},
					Name:     newNames[1],
				},
			}
		})).Should(Succeed())

		// given we changed the names and priority, and older settings was in progress
		// we expect it to fail, as there is already
		By("Ensuring that the BIOSSetting Object has moved to Failed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Status.FlowState", HaveLen(len(biosSettings.Spec.SettingsFlow))),
		))

		Eventually(Object(biosSettings)).WithPolling(1 * time.Microsecond).Should(
			SatisfyAll(
				HaveField("Status.FlowState", ContainElement(
					SatisfyAll(
						HaveField("Name", newNames[0]),
						HaveField("State", metalv1alpha1.BIOSSettingsFlowStateApplied),
					),
				)),
				HaveField("Status.FlowState", ContainElement(
					SatisfyAll(
						HaveField("Name", newNames[1]),
						HaveField("State", metalv1alpha1.BIOSSettingsFlowStateApplied),
					),
				)),
			))

		By("Deleting the BIOSSetting")
		Expect(k8sClient.Delete(ctx, biosSettings)).To(Succeed())
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})
})

func ensureBiosSettingsFlowCondition(biosSettings *metalv1alpha1.BIOSSettings) {
	By("Ensuring right number of conditions are present")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.Conditions", HaveLen(4)),
	)

	By("Ensuring the wait for version upgrade condition has NOT been added")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.Conditions", Not(ContainElement(
			HaveField("Type", BIOSVersionUpdateConditionPending),
		))),
	)

	By("Ensuring the serverMaintenance condition has been created")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.Conditions", ContainElement(
			SatisfyAll(
				HaveField("Type", ServerMaintenanceConditionCreated),
				HaveField("Status", metav1.ConditionTrue),
			),
		)),
	)

	for _, settings := range biosSettings.Spec.SettingsFlow {
		By(fmt.Sprintf("Ensuring the BIOSSettings Object has applied following settings %v", settings.Settings))

		// Create a matcher for a single FlowState element
		flowStateMatcher := SatisfyAll(
			HaveField("Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingConditionUpdateStartTime),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
			HaveField("Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingsConditionServerPowerOn),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
			HaveField("Conditions", ContainElement(
				HaveField("Type", BIOSSettingsConditionRebootPostUpdate),
			)),
			HaveField("Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingsConditionIssuedUpdate),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
			HaveField("Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingsConditionVerifySettings),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
		)

		Eventually(Object(biosSettings)).Should(
			HaveField("Status.FlowState", HaveEach(flowStateMatcher)),
		)
	}

	By("Ensuring the server maintenance has been deleted")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.Conditions", ContainElement(
			SatisfyAll(
				HaveField("Type", ServerMaintenanceConditionDeleted),
				HaveField("Status", metav1.ConditionTrue),
			),
		)),
	)
}

func ensureBiosSettingsCondition(biosSettings *metalv1alpha1.BIOSSettings, RebootNeeded bool, waitForVersionUpgrade bool) {
	commonConditions := 4
	flowCondition := 5

	if RebootNeeded {
		flowCondition += 2
	}

	if waitForVersionUpgrade {
		commonConditions += 1
	}

	By("Ensuring the right number of conditions are present")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.Conditions", HaveLen(commonConditions)),
	)

	Eventually(Object(biosSettings)).Should(
		HaveField("Status.FlowState", WithTransform(
			func(flowStates []metalv1alpha1.BIOSSettingsFlowStatus) int {
				if len(flowStates) == 0 {
					return 0
				}
				return len(flowStates[0].Conditions)
			},
			BeNumerically("==", flowCondition),
		)),
	)

	if waitForVersionUpgrade {
		By("Ensuring the wait for version upgrade condition has been added")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSVersionUpdateConditionPending),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
		)
	} else {
		By("Ensuring the wait for version upgrade condition has NOT been added")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.Conditions", Not(ContainElement(
				HaveField("Type", BIOSVersionUpdateConditionPending),
			))),
		)
	}

	By("Ensuring the serverMaintenance condition has been created")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.Conditions", ContainElement(
			SatisfyAll(
				HaveField("Type", ServerMaintenanceConditionCreated),
				HaveField("Status", metav1.ConditionTrue),
			),
		)),
	)

	By("Ensuring the timeout error start time has been recorded")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.FlowState", ContainElement(
			HaveField("Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingConditionUpdateStartTime),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
		)),
	)

	By("Ensuring the server has been powered on at the start")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.FlowState", ContainElement(
			HaveField("Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", BIOSSettingsConditionServerPowerOn),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
		)),
	)

	if !RebootNeeded {
		By("Ensuring the server skip reboot check has been created and skips reboot")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.FlowState", ContainElement(
				HaveField("Conditions", SatisfyAll(
					ContainElement(
						SatisfyAll(
							HaveField("Type", BIOSSettingsConditionRebootPostUpdate),
							HaveField("Status", metav1.ConditionTrue),
						),
					),
					Not(ContainElement(HaveField("Type", BIOSSettingsConditionRebootPowerOff))),
					Not(ContainElement(HaveField("Type", BIOSSettingsConditionRebootPowerOn))),
				)),
			)),
		)
	} else {
		By("Ensuring the server skip reboot check has been created and reboots the server")
		Eventually(Object(biosSettings)).Should(
			HaveField("Status.FlowState", ContainElement(
				HaveField("Conditions", SatisfyAll(
					ContainElement(
						SatisfyAll(
							HaveField("Type", BIOSSettingsConditionRebootPostUpdate),
							HaveField("Status", metav1.ConditionFalse),
						),
					),
					ContainElement(
						SatisfyAll(
							HaveField("Type", BIOSSettingsConditionRebootPowerOff),
							HaveField("Status", metav1.ConditionTrue),
						),
					),
					ContainElement(
						SatisfyAll(
							HaveField("Type", BIOSSettingsConditionRebootPowerOn),
							HaveField("Status", metav1.ConditionTrue),
						),
					),
				)),
			)),
		)
	}

	By("Ensuring the update has been issue to the server")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.FlowState", ContainElements(
			SatisfyAll(
				HaveField("Conditions", ContainElements(
					HaveField("Type", BIOSSettingsConditionIssuedUpdate),
					HaveField("Status", metav1.ConditionTrue),
				)),
			),
		)),
	)

	By("Ensuring the update has been applied by the server")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.FlowState", ContainElements(
			SatisfyAll(
				HaveField("Conditions", ContainElements(
					HaveField("Type", BIOSSettingsConditionVerifySettings),
					HaveField("Status", metav1.ConditionTrue),
				)),
			),
		)),
	)

	By("Ensuring the server maintenance has been deleted")
	Eventually(Object(biosSettings)).Should(
		HaveField("Status.Conditions", ContainElements(
			HaveField("Type", ServerMaintenanceConditionDeleted),
			HaveField("Status", metav1.ConditionTrue),
		)),
	)
}
