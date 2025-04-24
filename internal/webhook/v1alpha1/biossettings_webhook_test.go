// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	// TODO (user): Add any additional imports if needed
)

var _ = Describe("BIOSSettings Webhook", func() {
	var (
		obj       *metalv1alpha1.BIOSSettings
		oldObj    *metalv1alpha1.BIOSSettings
		validator BIOSSettingsCustomValidator
	)

	BeforeEach(func() {
		obj = &metalv1alpha1.BIOSSettings{}
		oldObj = &metalv1alpha1.BIOSSettings{}
		validator = BIOSSettingsCustomValidator{Client: k8sClient}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating or updating BIOSSettings under Validating Webhook", func() {

		It("Should deny creation if a Spec.ServerRef field is duplicate", func(ctx SpecContext) {
			By("Creating an BIOSSetttings")
			biosSettingsV1 := &metalv1alpha1.BIOSSettings{
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
			By("Creating an BIOSSetttings")
			biosSettingsV1 := &metalv1alpha1.BIOSSettings{
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
			By("Creating an BIOSSetttings")
			biosSettingsV1 := &metalv1alpha1.BIOSSettings{
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
			By("Creating an BIOSSetttings")
			biosSettingsV1 := &metalv1alpha1.BIOSSettings{
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
			By("Creating an BIOSSetttings")
			biosSettingsV1 := &metalv1alpha1.BIOSSettings{
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
	})

})
