// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"maps"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/ironcore-dev/controller-utils/conditionutils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
)

var _ = Describe("BIOSSettingsFlow Controller", func() {
	ns := SetupTest()

	var (
		server           *metalv1alpha1.Server
		biosSettingsFlow *metalv1alpha1.BIOSSettingsFlow
	)

	BeforeEach(func(ctx SpecContext) {
		By("Ensuring clean state")
		var serverList metalv1alpha1.ServerList
		Eventually(ObjectList(&serverList)).Should(HaveField("Items", (BeEmpty())))
		var biosFLowList metalv1alpha1.BIOSSettingsFlowList
		Eventually(ObjectList(&biosFLowList)).Should(HaveField("Items", (BeEmpty())))
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

		By("Creating a BIOSSettingFlow")
		biosSettingsFlow = &metalv1alpha1.BIOSSettingsFlow{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-setting-flow-",
			},
			Spec: metalv1alpha1.BIOSSettingsFlowSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{
					{
						Priority: 100,
						Settings: map[string]string{"fooreboot": "10"},
					},
					{
						Priority: 1000,
						Settings: map[string]string{"fooreboot": "100"},
					},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsFlow)).To(Succeed())

		By("Ensuring that the BIOSSetting Object is created")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsFlow.Name,
			},
		}
		Eventually(Get(biosSettings)).Should(Succeed())

		By("Ensuring that the BIOSSetting Object has applied first set of settings")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.SettingsMap", biosSettingsFlow.Spec.SettingsFlow[0].Settings),
			HaveField("Spec.CurrentSettingPriority", biosSettingsFlow.Spec.SettingsFlow[0].Priority),
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BIOSSettingsFlow",
				Name:               biosSettingsFlow.Name,
				UID:                biosSettingsFlow.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
		))

		By("Ensuring that the BIOSSetting Object has moved to next settings")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.SettingsMap", biosSettingsFlow.Spec.SettingsFlow[1].Settings),
			HaveField("Spec.CurrentSettingPriority", int32(math.MaxInt32)),
		))
		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
		By("Ensuring that the BIOSSettings conditions are updated")
		ensureBiosSettingsFlowCondition(biosSettings, biosSettingsFlow)

		By("Ensuring that the BIOSSettingFLow Object has status applied")
		Eventually(Object(biosSettingsFlow)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsFlowStateApplied),
		)

		By("Deleting the BIOSSettingFlow")
		Expect(k8sClient.Delete(ctx, biosSettingsFlow)).To(Succeed())
	})

	It("should successfully apply sequence of different settings", func(ctx SpecContext) {

		By("Creating a BIOSSettingFlow")
		biosSettingsFlow = &metalv1alpha1.BIOSSettingsFlow{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-reference",
			},
			Spec: metalv1alpha1.BIOSSettingsFlowSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{
					{
						Priority: 100,
						Settings: map[string]string{"abc": "foo-bar"},
					},
					{
						Priority: 1000,
						Settings: map[string]string{"fooreboot": "100"},
					},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsFlow)).To(Succeed())

		currentSettingMap := map[string]string{}
		maps.Copy(currentSettingMap, biosSettingsFlow.Spec.SettingsFlow[0].Settings)
		By("Ensuring that the BIOSSetting Object is created")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsFlow.Name,
			},
		}
		Eventually(Get(biosSettings)).Should(Succeed())

		By("Ensuring that the BIOSSetting Object has been patched with first set of settings")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.SettingsMap", currentSettingMap),
			HaveField("Spec.CurrentSettingPriority", biosSettingsFlow.Spec.SettingsFlow[0].Priority),
		))

		maps.Copy(currentSettingMap, biosSettingsFlow.Spec.SettingsFlow[1].Settings)
		By("Ensuring that the BIOSSetting Object has moved to next settings")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Spec.SettingsMap", currentSettingMap),
			HaveField("Spec.CurrentSettingPriority", int32(math.MaxInt32)),
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BIOSSettingsFlow",
				Name:               biosSettingsFlow.Name,
				UID:                biosSettingsFlow.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
		))

		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
		By("Ensuring that the BIOSSettings conditions are updated")
		ensureBiosSettingsFlowCondition(biosSettings, biosSettingsFlow)

		By("Ensuring that the BIOSSettingFLow Object has status applied")
		Eventually(Object(biosSettingsFlow)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsFlowStateApplied),
		)

		By("Deleting the BIOSSettingFlow")
		Expect(k8sClient.Delete(ctx, biosSettingsFlow)).To(Succeed())
	})

	It("should successfully apply single settings", func(ctx SpecContext) {

		By("Creating a BIOSSettingFlow")
		biosSettingsFlow = &metalv1alpha1.BIOSSettingsFlow{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-reference",
			},
			Spec: metalv1alpha1.BIOSSettingsFlowSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{
					{
						Priority: 100,
						Settings: map[string]string{"abc": "bar-foo"},
					},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsFlow)).To(Succeed())

		By("Ensuring that the BIOSSetting Object is created")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsFlow.Name,
			},
		}
		Eventually(Get(biosSettings)).Should(Succeed())

		By("Ensuring that the BIOSSetting Object has applied first set of settings")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Spec.SettingsMap", biosSettingsFlow.Spec.SettingsFlow[0].Settings),
			HaveField("Spec.CurrentSettingPriority", int32(math.MaxInt32)),
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BIOSSettingsFlow",
				Name:               biosSettingsFlow.Name,
				UID:                biosSettingsFlow.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
		))
		By("Ensuring that the BIOSSettings conditions are updated")
		ensureBiosSettingsFlowCondition(biosSettings, biosSettingsFlow)

		By("Ensuring that the BIOSSettingFLow Object has status applied")
		Eventually(Object(biosSettingsFlow)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsFlowStateApplied),
		)

		By("Deleting the BIOSSettingFlow")
		Expect(k8sClient.Delete(ctx, biosSettingsFlow)).To(Succeed())
	})

	It("should successfully Complete with no settings", func(ctx SpecContext) {

		By("Creating a BIOSSettingFlow")
		biosSettingsFlow = &metalv1alpha1.BIOSSettingsFlow{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-reference",
			},
			Spec: metalv1alpha1.BIOSSettingsFlowSpec{
				Version:                 defaultMockUpServerBiosVersion,
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsFlow)).To(Succeed())

		By("Ensuring that the BIOSSetting Object is not created")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsFlow.Name,
			},
		}
		Eventually(Get(biosSettings)).Should(Not(Succeed()))
		Consistently(Get(biosSettings)).Should(Not(Succeed()))

		By("Ensuring that the BIOSSettingFLow Object has status applied")
		Eventually(Object(biosSettingsFlow)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsFlowStateApplied),
		)

		By("Deleting the BIOSSettingFlow")
		Expect(k8sClient.Delete(ctx, biosSettingsFlow)).To(Succeed())
		By("Ensuring the objects are deleted")
		Eventually(Get(biosSettingsFlow)).Should(Not(Succeed()))
		Consistently(Get(biosSettingsFlow)).Should(Not(Succeed()))
		Eventually(Get(biosSettings)).Should(Not(Succeed()))
		Consistently(Get(biosSettings)).Should(Not(Succeed()))
	})

	It("should not delete biosSettings when BIOS Settings in progress", func(ctx SpecContext) {

		By("Creating a BIOSSettingFlow")
		biosSettingsFlow = &metalv1alpha1.BIOSSettingsFlow{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-reference",
			},
			Spec: metalv1alpha1.BIOSSettingsFlowSpec{
				Version: defaultMockUpServerBiosVersion,
				SettingsFlow: []metalv1alpha1.SettingsFlowItem{
					{
						Priority: 100,
						Settings: map[string]string{"abc": "bar-bar"},
					},
					{
						Priority: 1000,
						Settings: map[string]string{"123": "foo-foo"},
					},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsFlow)).To(Succeed())

		currentSettingMap := map[string]string{}
		maps.Copy(currentSettingMap, biosSettingsFlow.Spec.SettingsFlow[0].Settings)
		By("Ensuring that the BIOSSetting Object is created")
		biosSettings := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsFlow.Name,
			},
		}
		Eventually(Get(biosSettings)).Should(Succeed())

		By("Deleting the BIOSSettingFlow continue to apply the settings")
		Expect(k8sClient.Delete(ctx, biosSettingsFlow)).To(Succeed())

		// to be fast polling as the delete operation after settings are applied is super fast
		By("Ensuring that the BIOSSetting Object has applied the settings")
		Eventually(
			func(g Gomega) {
				biosSettings, err := Object(biosSettings)()
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(biosSettings).Should(
					HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
				)
			}).WithPolling(1 * time.Millisecond).Should(Succeed())
	})
})

func ensureBiosSettingsFlowCondition(
	biosSettings *metalv1alpha1.BIOSSettings,
	biosSettingsFlow *metalv1alpha1.BIOSSettingsFlow,
) {
	acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
	requiredCondition := 7
	condMaintenanceDeleted := &metav1.Condition{}
	condMaintenanceCreated := &metav1.Condition{}
	condIssueSettingsUpdate := &metav1.Condition{}
	condVerifySettingsUpdate := &metav1.Condition{}
	condServerPoweredOn := &metav1.Condition{}
	condSkipReboot := &metav1.Condition{}

	condmoveToNextStep := &metav1.Condition{}

	condTimerStarted := &metav1.Condition{}

	condPendingVersionUpdate := &metav1.Condition{}

	By("Ensuring right number of conditions are present")
	Eventually(
		func(g Gomega) int {
			g.Expect(Get(biosSettings)()).To(Succeed())
			return len(biosSettings.Status.Conditions)
		}).Should(BeNumerically(">=", requiredCondition*len(biosSettingsFlow.Spec.SettingsFlow)))

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

	for idx, settings := range biosSettingsFlow.Spec.SettingsFlow {
		if idx == len(biosSettingsFlow.Spec.SettingsFlow)-1 {
			settings.Priority = int32(math.MaxInt32)
		}
		By(fmt.Sprintf("Ensuring the BIOSSettings Object has applied following settings %v", settings.Settings))
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
					fmt.Sprintf("%s-%d", skipRebootCondition, biosSettings.Spec.CurrentSettingPriority),
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

		if idx < len(biosSettingsFlow.Spec.SettingsFlow)-2 {
			By("Ensuring the move to next step condition has been set")
			Eventually(
				func(g Gomega) bool {
					g.Expect(Get(biosSettings)()).To(Succeed())
					g.Expect(acc.FindSlice(biosSettings.Status.Conditions,
						fmt.Sprintf("%s-%d", moveToNextStepCondition, settings.Priority),
						condmoveToNextStep)).To(BeTrue())
					return condmoveToNextStep.Status == metav1.ConditionTrue
				}).Should(BeTrue())
		}
	}

	By("Ensuring the server maintenance has been deleted")
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosSettings)()).To(Succeed())
			g.Expect(acc.FindSlice(biosSettings.Status.Conditions, serverMaintenanceDeletedCondition, condMaintenanceDeleted)).To(BeTrue())
			return condMaintenanceDeleted.Status == metav1.ConditionTrue
		}).Should(BeTrue())
}
