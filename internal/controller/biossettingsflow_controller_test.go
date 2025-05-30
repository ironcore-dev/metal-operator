// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"maps"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		))

		By("Ensuring that the BIOSSetting Object has moved to completed")
		Eventually(Object(biosSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		))
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
		))
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

		By("Ensuring that the BIOSSetting Object is created")
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

		By("Ensuring the objects are deleted")
		Eventually(Get(biosSettingsFlow)).Should(Not(Succeed()))
		Consistently(Get(biosSettingsFlow)).Should(Not(Succeed()))
		Eventually(Get(biosSettings)).Should(Not(Succeed()))
		Consistently(Get(biosSettings)).Should(Not(Succeed()))
	})
})
