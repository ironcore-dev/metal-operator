// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"

	bmcPkg "github.com/ironcore-dev/metal-operator/bmc"
)

var _ = Describe("BMCSettings Controller", func() {
	ns := SetupTest(nil)

	var (
		server    *metalv1alpha1.Server
		bmc       *metalv1alpha1.BMC
		bmcSecret *metalv1alpha1.BMCSecret
	)

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
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

		By("Creating a BMC resource")
		bmc = &metalv1alpha1.BMC{
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
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		By("Ensuring that the Server resource will be created")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Get(server)).Should(Succeed())

		By("Ensuring that the Server is in an available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))
	})

	AfterEach(func(ctx SpecContext) {
		bmcPkg.UnitTestMockUps.ResetBMCSettings()

		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		EnsureCleanState()
	})

	It("Should successfully patch BMCSettings reference to referred BMC", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "1.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				}},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMC has the BMCSettings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", &v1.LocalObjectReference{Name: bmcSettings.Name}),
		))

		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())
	})

	It("Should move to completed if no BMCSettings changes to referred BMC", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)

		By("Creating a bmcSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-nochange",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "1.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				}},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMC has the BMCSettings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", &v1.LocalObjectReference{Name: bmcSettings.Name}),
		))

		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Deleting the BMCSettings")
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMCSettings ref is empty on BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", BeNil()),
		))
	})

	It("Should update the setting if BMCSettings changes requested in Available State", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)
		bmcSetting["abc"] = "changed-bmc-setting"

		By("update the server state to Available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-change",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "1.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				}},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMC has the BMCSettings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", &v1.LocalObjectReference{Name: bmcSettings.Name}),
		))

		By("Ensuring that the BMCSettings has reached next state")
		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))
		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Ensuring that the Maintenance resource has been deleted")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		))

		By("Deleting the BMCSettings")
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMCSettings ref is empty on BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", BeNil()),
		))

		// cleanup
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should create maintenance and wait for its approval before applying settings", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)
		bmcSetting["abc"] = "changed-to-req-server-maintenance-through-ownerapproved"

		// put server in reserved state. and create a bmc setting in owner approved which needs reboot.
		// this is needed to check the states traversed.
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
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-change",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "1.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				}},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
		))

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", HaveLen(1)))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      serverMaintenanceList.Items[0].Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by BMCSettings resource")
		Eventually(Object(bmcSettings)).Should(
			HaveField("Spec.ServerMaintenanceRefs",
				[]metalv1alpha1.ServerMaintenanceRefItem{{
					ServerMaintenanceRef: &metalv1alpha1.ObjectReference{
						Kind:       "ServerMaintenance",
						Name:       serverMaintenance.Name,
						Namespace:  serverMaintenance.Namespace,
						UID:        serverMaintenance.UID,
						APIVersion: metalv1alpha1.GroupVersion.String(),
					}}}),
		)

		By("Ensuring that the BMC has the BMCSettings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", &v1.LocalObjectReference{Name: bmcSettings.Name}),
		))

		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
		))

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, trueValue)
			metautils.SetLabel(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, trueValue)
		})).Should(Succeed())

		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Ensuring that the Maintenance resource has been deleted")
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		))

		By("Deleting the BMCSettings")
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMCSettings ref is empty on BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", BeNil()),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateReserved))),
		))
	})

	It("Should wait for upgrade and reconcile BMCSettings version is correct", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)
		bmcSetting["fooreboot"] = "145"

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BMCSetting")
		BMCSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-upgrade",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "2.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				}},
		}
		Expect(k8sClient.Create(ctx, BMCSettings)).To(Succeed())

		By("Ensuring that the BMC has the correct BMC settings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", Not(BeNil())),
			HaveField("Spec.BMCSettingRef.Name", BMCSettings.Name),
		))

		By("Ensuring that the BMCSettings resource state is correct State inVersionUpgrade")
		Eventually(Object(BMCSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStatePending),
			HaveField("Status.Conditions", Not(ContainElement(SatisfyAll(
				HaveField("Type", BMCVersionUpdatePendingCondition),
				HaveField("Status", metav1.ConditionTrue),
			)))),
		))

		By("Ensuring that the serverMaintenance not ref. while waiting for upgrade")
		Consistently(Object(BMCSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		))

		By("Simulate the server BMCSettings version update by matching the spec version")
		Eventually(Update(BMCSettings, func() {
			BMCSettings.Spec.Version = "1.45.455b66-rev4"
		})).Should(Succeed())

		By("Ensuring that the BMCSettings resource has completed Upgrade and setting update, and moved the state")
		Eventually(Object(BMCSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.Conditions", Not(ContainElement(SatisfyAll(
				HaveField("Type", BMCVersionUpdatePendingCondition),
				HaveField("Status", metav1.ConditionFalse),
			)))),
		))

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", HaveLen(1)))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      serverMaintenanceList.Items[0].Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the BMCSettings resource has moved to next state")
		Eventually(Object(BMCSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))
		Eventually(Object(BMCSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Ensuring that the Maintenance resource has been deleted")
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(Object(BMCSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		))

		By("Deleting the BMCSetting resource")
		Expect(k8sClient.Delete(ctx, BMCSettings)).To(Succeed())

		By("Ensuring that the BMCSettings resource is removed")
		Eventually(Get(BMCSettings)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(BMCSettings)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the Server BMCSettings ref is empty on BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", BeNil()),
		))

		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should replace missing BMCSettings ref in server", func(ctx SpecContext) {
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBMCSetting
		bmcSetting := make(map[string]string)
		bmcSetting["fooreboot"] = "145"

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-upgrade",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "1.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				}},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		By("Wait for the BMCSettings to be ref on the BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", Not(BeNil())),
			HaveField("Spec.BMCSettingRef.Name", bmcSettings.Name),
		))
		// delete the old settings
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())
		By("force deletion of the object by removing finalizers")
		Eventually(func() error {
			err := Update(bmcSettings, func() {
				bmcSettings.Finalizers = []string{}
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
					if len(item.OwnerReferences) > 0 && item.OwnerReferences[0].UID == bmcSettings.UID {
						By(fmt.Sprintf("Deleting the ServerMaintenance created by BMCSettings %v", item.Name))
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

		By("creation of new BMCSettings with same spec")
		bmcSettings2 := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-recreate-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "1.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				}},
		}
		Expect(k8sClient.Create(ctx, bmcSettings2)).To(Succeed())

		By("Wait for the BMCSettings2 to be ref on the BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", Not(BeNil())),
			HaveField("Spec.BMCSettingRef.Name", bmcSettings2.Name),
		))

		Eventually(Object(bmcSettings2)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		Eventually(Object(bmcSettings2)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		Expect(k8sClient.Delete(ctx, bmcSettings2)).To(Succeed())
		Eventually(Get(bmcSettings2)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should replace missing BMCSettings ref in server", func(ctx SpecContext) {
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBMCSetting
		bmcSetting := make(map[string]string)
		bmcSetting["fooreboot"] = "145"

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-upgrade",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "1.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				}},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		By("Wait for the BMCSettings to be ref on the BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", Not(BeNil())),
			HaveField("Spec.BMCSettingRef.Name", bmcSettings.Name),
		))
		// delete the old settings
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())
		By("force deletion of the object by removing finalizers")
		Eventually(func() error {
			err := Update(bmcSettings, func() {
				bmcSettings.Finalizers = []string{}
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
					if len(item.OwnerReferences) > 0 && item.OwnerReferences[0].UID == bmcSettings.UID {
						By(fmt.Sprintf("Deleting the ServerMaintenance created by BMCSettings %v", item.Name))
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

		By("creation of new BMCSettings with same spec")
		bmcSettings2 := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-recreate-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "1.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				}},
		}
		Expect(k8sClient.Create(ctx, bmcSettings2)).To(Succeed())

		By("Wait for the BMCSettings2 to be ref on the BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", Not(BeNil())),
			HaveField("Spec.BMCSettingRef.Name", bmcSettings2.Name),
		))

		Eventually(Object(bmcSettings2)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		Eventually(Object(bmcSettings2)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		Expect(k8sClient.Delete(ctx, bmcSettings2)).To(Succeed())
		Eventually(Get(bmcSettings2)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should allow retry using annotation", func(ctx SpecContext) {
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBMCSetting
		bmcSetting := make(map[string]string)
		bmcSetting["UnknownData"] = "145"

		retryCount := 2

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-upgrade",
				Annotations: map[string]string{
					metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationRetryFailed,
				},
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmc.Name},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "1.45.455b66-rev4",
					SettingsMap:             bmcSetting,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					FailedAutoRetryCount:    GetPtr(int32(retryCount)),
				}},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		Eventually(func(g Gomega) bool {
			g.Expect(Get(bmcSettings)()).To(Succeed())
			return bmcSettings.Status.State == metalv1alpha1.BMCSettingsStateFailed && bmcSettings.Status.AutoRetryCountRemaining == nil
		}).WithPolling((10 * time.Microsecond)).Should(BeTrue())

		Eventually(func(g Gomega) bool {
			g.Expect(Get(bmcSettings)()).To(Succeed())
			return bmcSettings.Status.State == metalv1alpha1.BMCSettingsStateFailed && bmcSettings.Status.AutoRetryCountRemaining != nil && *bmcSettings.Status.AutoRetryCountRemaining == int32(1)
		}).WithPolling((10 * time.Microsecond)).Should(BeTrue())

		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateFailed),
			HaveField("Status.AutoRetryCountRemaining", Equal(GetPtr(int32(0)))),
		))

		Eventually(Object(bmcSettings)).Should(
			HaveField("ObjectMeta.Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))),
		)

		By("Ensuring that the BMC setting has not been changed")
		Consistently(Object(bmcSettings), "25ms").Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateFailed),
			HaveField("Status.AutoRetryCountRemaining", Equal(GetPtr(int32(0)))),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())
		// clean up maintenance if any, as the test not auto delete child objects
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))
		for _, maintenance := range serverMaintenanceList.Items {
			Expect(k8sClient.Delete(ctx, &maintenance)).To(Succeed())
		}
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})
})
