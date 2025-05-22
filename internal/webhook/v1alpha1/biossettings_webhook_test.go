// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BIOSSettings Webhook", func() {
	var (
		biosSettingsV1 *metalv1alpha1.BIOSSettings
		validator      BIOSSettingsCustomValidator
	)

	BeforeEach(func() {
		validator = BIOSSettingsCustomValidator{Client: k8sClient}
		SetClient(k8sClient)
		By("Creating an BIOSSetttings")
		biosSettingsV1 = &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				Version:                 "P70 v1.45 (12/06/2017)",
				SettingsMap:             map[string]string{},
				ServerRef:               &v1.LocalObjectReference{Name: "foo"},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV1)).To(Succeed())
		DeferCleanup(k8sClient.Delete, biosSettingsV1)
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating or updating BIOSSettings under Validating Webhook", func() {

		It("Should deny creation if a Spec.ServerRef field is duplicate", func(ctx SpecContext) {
			By("Creating another BIOSSettings with existing ServerRef")
			biosSettingsV2 := &metalv1alpha1.BIOSSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BIOSSettingsSpec{
					Version:                 "P70 v1.45 (12/06/2017)",
					SettingsMap:             map[string]string{},
					ServerRef:               &v1.LocalObjectReference{Name: "foo"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(validator.ValidateCreate(ctx, biosSettingsV2)).Error().To(HaveOccurred())
		})

		It("Should create if a Spec.ServerRef field is NOT duplicate", func() {
			By("Creating another BIOSSetting with different ServerRef")
			biosSettingsV2 := &metalv1alpha1.BIOSSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BIOSSettingsSpec{
					Version:                 "P70 v1.45 (12/06/2017)",
					SettingsMap:             map[string]string{},
					ServerRef:               &v1.LocalObjectReference{Name: "bar"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, biosSettingsV2)
		})

		It("Should deny update if a Spec.ServerRef field is duplicate", func() {
			By("Creating an BIOSSetting with different ServerRef")
			biosSettingsV2 := &metalv1alpha1.BIOSSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BIOSSettingsSpec{
					Version:                 "P70 v1.45 (12/06/2017)",
					SettingsMap:             map[string]string{},
					ServerRef:               &v1.LocalObjectReference{Name: "bar"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, biosSettingsV2)

			By("Updating an biosSettingsV2 to conflicting Spec.ServerRef")
			biosSettingsV2Updated := biosSettingsV2.DeepCopy()
			biosSettingsV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
			Expect(validator.ValidateUpdate(ctx, biosSettingsV2, biosSettingsV2Updated)).Error().To(HaveOccurred())
		})

		It("Should allow update if a different field is duplicate", func() {
			By("Creating an BIOSSetting with different ServerRef")
			biosSettingsV2 := &metalv1alpha1.BIOSSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BIOSSettingsSpec{
					Version:                 "P71 v1.45 (12/06/2017)",
					SettingsMap:             map[string]string{},
					ServerRef:               &v1.LocalObjectReference{Name: "bar"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, biosSettingsV2)

			By("Updating an biosSettingsV2 to conflicting Spec.BIOSSettings")
			biosSettingsV2Updated := biosSettingsV2.DeepCopy()
			biosSettingsV2Updated.Spec.Version = biosSettingsV1.Spec.Version
			Expect(validator.ValidateUpdate(ctx, biosSettingsV2, biosSettingsV2Updated)).Error().ToNot(HaveOccurred())
		})

		It("Should allow update if a ServerRef field is NOT duplicate", func() {
			By("Creating an BIOSSetting with different ServerRef")
			biosSettingsV2 := &metalv1alpha1.BIOSSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BIOSSettingsSpec{
					Version:                 "P70 v1.45 (12/06/2017)",
					SettingsMap:             map[string]string{},
					ServerRef:               &v1.LocalObjectReference{Name: "bar"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, biosSettingsV2)

			By("Updating an biosSettingsV2 to NON conflicting Spec.ServerRef ")
			biosSettingsV2Updated := biosSettingsV2.DeepCopy()
			biosSettingsV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foobar"}
			Expect(validator.ValidateUpdate(ctx, biosSettingsV2, biosSettingsV2Updated)).Error().ToNot(HaveOccurred())
		})

		It("Should refuse to delete if InProgress", func() {
			By("Patching the boot configuration to a Inprogress state")
			Eventually(UpdateStatus(biosSettingsV1, func() {
				biosSettingsV1.Status.State = metalv1alpha1.BIOSSettingsStateInProgress
			})).Should(Succeed())

			By("Deleting the BIOSSettings should fail")
			Expect(k8sClient.Delete(ctx, biosSettingsV1)).To(Not(Succeed()))

			Eventually(UpdateStatus(biosSettingsV1, func() {
				biosSettingsV1.Status.State = metalv1alpha1.BIOSSettingsStateApplied
			})).Should(Succeed())

			By("Deleting the BIOSSettings should pass: by DeferCleanup")
		})
	})

})
