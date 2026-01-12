// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

var _ = Describe("BMCSettingsSet Controller", func() {
	Context("When reconciling a resource", func() {

		ns := SetupTest(nil)
		var server01 *metalv1alpha1.Server
		var server02 *metalv1alpha1.Server
		var bmcSecret *metalv1alpha1.BMCSecret

		var bmc01 *metalv1alpha1.BMC
		var bmc02 *metalv1alpha1.BMC

		const changedBMCSetting = "changed-bmc-setting"

		BeforeEach(func(ctx SpecContext) {

			By("Creating a BMCSecret")
			bmcSecret = &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

			By("Creating BMC1 which fits the labels")
			bmc01 = &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc01",
					Labels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "foo",
						"metal.ironcore.dev/Model":        "bar",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					Endpoint: &metalv1alpha1.InlineEndpoint{
						IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
						MACAddress: "23:11:8A:33:CF:EA",
					},
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name},
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000},
				},
			}
			Expect(k8sClient.Create(ctx, bmc01)).To(Succeed())

			By("Creating BMC2 with same manufacturer but different model")
			bmc02 = &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc02",
					Labels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "foo",
						"metal.ironcore.dev/Model":        "bar2",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					Endpoint: &metalv1alpha1.InlineEndpoint{
						IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
						MACAddress: "23:11:8A:33:CF:EA",
					},
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name},
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000},
				},
			}
			Expect(k8sClient.Create(ctx, bmc02)).To(Succeed())

			By("Ensuring that the Server resource will be created")
			server01 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc01),
				},
			}
			Eventually(Get(server01)).Should(Succeed())

			By("Ensuring that the BMC has right state: enabled")
			Eventually(Object(bmc01)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			))

			By("Ensuring that the Server resource will be created")
			server02 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc02),
				},
			}
			Eventually(Get(server02)).Should(Succeed())

			By("Ensuring that the BMC has right state: enabled")
			Eventually(Object(bmc02)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			))
		})
		AfterEach(func(ctx SpecContext) {

			By("Ensuring servers are available before deletion (if they exist)")
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(server01), server01); err == nil {
				Eventually(UpdateStatus(server01, func() {
					server01.Status.State = metalv1alpha1.ServerStateAvailable
				})).Should(Succeed())
			}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(server02), server02); err == nil {
				Eventually(UpdateStatus(server02, func() {
					server02.Status.State = metalv1alpha1.ServerStateAvailable
				})).Should(Succeed())
			}

			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, server01))).To(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, server02))).To(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, bmc01))).To(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, bmc02))).To(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, bmcSecret))).To(Succeed())
			By("Ensuring all resources are cleaned up")
			EnsureCleanState()
		})

		It("Should successfully reconcile when BMCSettingsSet was generated, labels match and bmcsettings were generated", func(ctx SpecContext) {
			bmcSetting := make(map[string]string)
			bmcSetting["abc"] = changedBMCSetting

			By("Creating a BMCSettingsSet")
			bmcSettingsSet := &metalv1alpha1.BMCSettingsSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    ns.Name,
					GenerateName: "test-bmcsettingsset"},
				Spec: metalv1alpha1.BMCSettingsSetSpec{
					BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
						Version:                 "1.45.455b66-rev4",
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
						SettingsMap:             bmcSetting,
					},
					BMCSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "foo",
							"metal.ironcore.dev/Model":        "bar",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmcSettingsSet)).To(Succeed())

			By("Checking if the bmcSettings for bmc01 was generated")
			bmcSettings01 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcSettingsSet.Name + "-" + bmc01.Name,
				},
			}
			Eventually(Get(bmcSettings01)).Should(Succeed())

			By("Checking bmcSettings01 fields")
			Eventually(Object(bmcSettings01)).Should(SatisfyAll(
				HaveField("Spec.BMCRef.Name", Equal(bmc01.Name)),
				HaveField("Spec.Version", Equal(bmcSettingsSet.Spec.BMCSettingsTemplate.Version)),
				HaveField("Spec.SettingsMap", HaveKeyWithValue("abc", changedBMCSetting)),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BMCSettingsSet",
					Name:               bmcSettingsSet.Name,
					UID:                bmcSettingsSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking that BMCSettings was NOT created for non-matching BMCs")
			bmcSettings02 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcSettingsSet.Name + "-" + bmc02.Name,
				},
			}
			Consistently(Get(bmcSettings02)).Should(Satisfy(apierrors.IsNotFound))

			Eventually(Object(bmcSettingsSet)).Should(SatisfyAll(
				HaveField(("Status.FullyLabeledBMCs"), BeNumerically("==", 1)),
				HaveField(("Status.AvailableBMCSettings"), BeNumerically("==", 1)),
				HaveField(("Status.FailedBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.PendingBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.InProgressBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.CompletedBMCSettings"), BeNumerically("==", 1)),
			))

			By("Deleting the BMCSettingsSet")
			Expect(k8sClient.Delete(ctx, bmcSettingsSet)).To(Succeed())
			By("Waiting for BMCSettingsSet and dependent resources to be deleted")
			Eventually(Get(bmcSettingsSet)).Should(Satisfy(apierrors.IsNotFound))

			By("Checking if the BMCSettingsSet was deleted")
			Eventually(Get(bmcSettingsSet)).Should(Satisfy(apierrors.IsNotFound))

			By("Deleting the BMCSetting resource")
			Expect(k8sClient.Delete(ctx, bmcSettings01)).To(Succeed())

			By("Ensuring that the BMCSettings resource is removed")
			Eventually(Get(bmcSettings01)).Should(Satisfy(apierrors.IsNotFound))
			Consistently(Get(bmcSettings01)).Should(Satisfy(apierrors.IsNotFound))

		})

		It("Should successfully reconcile when bmc resource was deleted", func(ctx SpecContext) {
			bmcSetting := make(map[string]string)
			bmcSetting["abc"] = changedBMCSetting

			By("Creating a BMCSettingsSet")
			bmcSettingsSet := &metalv1alpha1.BMCSettingsSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    ns.Name,
					GenerateName: "test-bmcsettingsset"},
				Spec: metalv1alpha1.BMCSettingsSetSpec{
					BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
						Version:                 "1.45.455b66-rev4",
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
						SettingsMap:             bmcSetting,
					},
					BMCSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "foo",
							"metal.ironcore.dev/Model":        "bar",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmcSettingsSet)).To(Succeed())

			By("Checking if the bmcSettings for bmc01 was generated")
			bmcSettings01 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcSettingsSet.Name + "-" + bmc01.Name,
				},
			}
			Eventually(Get(bmcSettings01)).Should(Succeed())

			By("Checking bmcSettings01 fields")
			Eventually(Object(bmcSettings01)).Should(SatisfyAll(
				HaveField("Spec.BMCRef.Name", Equal(bmc01.Name)),
				HaveField("Spec.Version", Equal(bmcSettingsSet.Spec.BMCSettingsTemplate.Version)),
				HaveField("Spec.SettingsMap", HaveKeyWithValue("abc", changedBMCSetting)),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BMCSettingsSet",
					Name:               bmcSettingsSet.Name,
					UID:                bmcSettingsSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking that BMCSettings was NOT created for non-matching BMCs")
			bmcSettings02 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcSettingsSet.Name + "-" + bmc02.Name,
				},
			}
			Consistently(Get(bmcSettings02)).Should(Satisfy(apierrors.IsNotFound))

			Eventually(Object(bmcSettingsSet)).Should(SatisfyAll(
				HaveField(("Status.FullyLabeledBMCs"), BeNumerically("==", 1)),
				HaveField(("Status.AvailableBMCSettings"), BeNumerically("==", 1)),
				HaveField(("Status.FailedBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.PendingBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.InProgressBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.CompletedBMCSettings"), BeNumerically("==", 1)),
			))

			By("Deleting BMC01 resource")
			Expect(k8sClient.Delete(ctx, bmc01)).To(Succeed())
			Eventually(Get(bmc01)).Should(Satisfy(apierrors.IsNotFound))

			By("Checking that BMCSettings was deleted after BMC deletion")
			Eventually(Get(bmcSettings01)).Should(Satisfy(apierrors.IsNotFound))

			Eventually(Object(bmcSettingsSet)).Should(SatisfyAll(
				HaveField(("Status.FullyLabeledBMCs"), BeNumerically("==", 0)),
				HaveField(("Status.AvailableBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.FailedBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.PendingBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.InProgressBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.CompletedBMCSettings"), BeNumerically("==", 0)),
			))

			By("Deleting the BMCSettingsSet")
			Expect(k8sClient.Delete(ctx, bmcSettingsSet)).To(Succeed())

			By("Checking if the BMCSettingsSet was deleted")
			Eventually(Get(bmcSettingsSet)).Should(Satisfy(apierrors.IsNotFound))

		})

		It("Should successfully reconcile when label of bmc02 was changed", func(ctx SpecContext) {
			bmcSetting := make(map[string]string)
			bmcSetting["abc"] = changedBMCSetting

			By("Creating a BMCSettingsSet")
			bmcSettingsSet := &metalv1alpha1.BMCSettingsSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    ns.Name,
					GenerateName: "test-bmcsettingsset"},
				Spec: metalv1alpha1.BMCSettingsSetSpec{
					BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
						Version:                 "1.45.455b66-rev4",
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
						SettingsMap:             bmcSetting,
					},
					BMCSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "foo",
							"metal.ironcore.dev/Model":        "bar",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmcSettingsSet)).To(Succeed())

			By("Checking if the bmcSettings was generated")
			bmcSettings01 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcSettingsSet.Name + "-" + bmc01.Name,
				},
			}
			Eventually(Get(bmcSettings01)).Should(Succeed())

			Eventually(Object(bmcSettingsSet)).Should(SatisfyAll(
				HaveField(("Status.FullyLabeledBMCs"), BeNumerically("==", 1)),
				HaveField(("Status.AvailableBMCSettings"), BeNumerically("==", 1)),
				HaveField(("Status.FailedBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.PendingBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.InProgressBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.CompletedBMCSettings"), BeNumerically("==", 1)),
			))

			By("Changing labels on BMC02 so that it matches the selector")
			Eventually(Update(bmc02, func() {
				bmc02.Labels["metal.ironcore.dev/Model"] = "bar"
			})).Should(Succeed())

			By("Checking if the bmcSettings for BMC02 was generated")
			bmcSettings02 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcSettingsSet.Name + "-" + bmc02.Name,
				},
			}
			Eventually(Get(bmcSettings02)).Should(Succeed())
			By("Checking bmcSettings02 fields")
			Eventually(Object(bmcSettings02)).Should(SatisfyAll(
				HaveField("Spec.BMCRef.Name", Equal(bmc02.Name)),
				HaveField("Spec.Version", Equal(bmcSettingsSet.Spec.BMCSettingsTemplate.Version)),
				HaveField("Spec.SettingsMap", HaveKeyWithValue("abc", changedBMCSetting)),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BMCSettingsSet",
					Name:               bmcSettingsSet.Name,
					UID:                bmcSettingsSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			Eventually(Object(bmcSettingsSet)).Should(SatisfyAll(
				HaveField(("Status.FullyLabeledBMCs"), BeNumerically("==", 2)),
				HaveField(("Status.AvailableBMCSettings"), BeNumerically("==", 2)),
				HaveField(("Status.FailedBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.PendingBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.InProgressBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.CompletedBMCSettings"), BeNumerically("==", 2)),
			))

			By("Deleting the BMCSettingsSet")
			Expect(k8sClient.Delete(ctx, bmcSettingsSet)).To(Succeed())

			By("Checking if the BMCSettingsSet was deleted")
			Eventually(Get(bmcSettingsSet)).Should(Satisfy(apierrors.IsNotFound))

			By("Deleting the BMCSetting resource")
			Expect(k8sClient.Delete(ctx, bmcSettings01)).To(Succeed())

			By("Ensuring that the BMCSettings resource is removed")
			Eventually(Get(bmcSettings01)).Should(Satisfy(apierrors.IsNotFound))
			Consistently(Get(bmcSettings01)).Should(Satisfy(apierrors.IsNotFound))
			Expect(k8sClient.Delete(ctx, bmcSettings02)).To(Succeed())

			By("Ensuring that the BMCSettings resource is removed")
			Eventually(Get(bmcSettings02)).Should(Satisfy(apierrors.IsNotFound))
			Consistently(Get(bmcSettings02)).Should(Satisfy(apierrors.IsNotFound))

		})
		It("Should successfully reconcile when bmcsettingset was updated", func(ctx SpecContext) {
			bmcSetting := make(map[string]string)
			bmcSetting["abc"] = changedBMCSetting
			bmcSettingNew := make(map[string]string)
			bmcSettingNew["abc"] = "new-bmc-setting"

			By("Creating a BMCSettingsSet")
			bmcSettingsSet := &metalv1alpha1.BMCSettingsSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    ns.Name,
					GenerateName: "test-bmcsettingsset"},
				Spec: metalv1alpha1.BMCSettingsSetSpec{
					BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
						Version:                 "1.45.455b66-rev4",
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
						SettingsMap:             bmcSetting,
					},
					BMCSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "foo",
							"metal.ironcore.dev/Model":        "bar",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmcSettingsSet)).To(Succeed())

			By("Checking if the bmcSettings was generated")
			bmcSettings01 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcSettingsSet.Name + "-" + bmc01.Name,
				},
			}
			Eventually(Get(bmcSettings01)).Should(Succeed())
			By("Checking bmcSettings01 fields")
			Eventually(Object(bmcSettings01)).Should(SatisfyAll(
				HaveField("Spec.BMCRef.Name", Equal(bmc01.Name)),
				HaveField("Spec.Version", Equal(bmcSettingsSet.Spec.BMCSettingsTemplate.Version)),
				HaveField("Spec.SettingsMap", HaveKeyWithValue("abc", changedBMCSetting)),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BMCSettingsSet",
					Name:               bmcSettingsSet.Name,
					UID:                bmcSettingsSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking that BMCSettings was NOT created for non-matching BMCs")
			bmcSettings02 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcSettingsSet.Name + "-" + bmc02.Name,
				},
			}
			Consistently(Get(bmcSettings02)).Should(Satisfy(apierrors.IsNotFound))

			Eventually(Object(bmcSettingsSet)).Should(SatisfyAll(
				HaveField(("Status.FullyLabeledBMCs"), BeNumerically("==", 1)),
				HaveField(("Status.AvailableBMCSettings"), BeNumerically("==", 1)),
				HaveField(("Status.FailedBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.PendingBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.InProgressBMCSettings"), BeNumerically("==", 0)),
				HaveField(("Status.CompletedBMCSettings"), BeNumerically("==", 1)),
			))

			By("Updating the BMCSettingsSet template")
			Eventually(Update(bmcSettingsSet, func() {
				bmcSettingsSet.Spec.BMCSettingsTemplate.Version = "1.45.455b66-rev4"
				bmcSettingsSet.Spec.BMCSettingsTemplate.SettingsMap = bmcSettingNew
			})).Should(Succeed())

			By("Checking if the bmcSettings was updated")
			Eventually(Object(bmcSettings01)).Should(HaveField("Spec.Version", Equal("1.45.455b66-rev4")))
			Eventually(Object(bmcSettings01)).Should(HaveField("Spec.SettingsMap", HaveKeyWithValue("abc", "new-bmc-setting")))

			By("Deleting the BMCSettingsSet")
			Expect(k8sClient.Delete(ctx, bmcSettingsSet)).To(Succeed())

			By("Checking if the BMCSettingsSet was deleted")
			Eventually(Get(bmcSettingsSet)).Should(Satisfy(apierrors.IsNotFound))

			By("Deleting the BMCSettings01 resource")
			Expect(k8sClient.Delete(ctx, bmcSettings01)).To(Succeed())
			Eventually(Get(bmcSettings01)).Should(Satisfy(apierrors.IsNotFound))

			By("Deleting the BMCSettings02 resource")
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, bmcSettings02))).To(Succeed())
			Eventually(Get(bmcSettings02)).Should(Satisfy(apierrors.IsNotFound))

			By("Deleting all ServerMaintenance objects created during the test")
			var maintList metalv1alpha1.ServerMaintenanceList
			Expect(k8sClient.List(ctx, &maintList, client.InNamespace(ns.Name))).To(Succeed())
			for i := range maintList.Items {
				maint := &maintList.Items[i]
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, maint))).To(Succeed())
				Eventually(Get(maint)).Should(Satisfy(apierrors.IsNotFound))
			}

			Expect(k8sClient.List(ctx, &maintList)).To(Succeed())
		})

	})
})
