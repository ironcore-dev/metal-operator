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

var _ = Describe("BMCSettings Webhook", func() {
	var (
		obj        *metalv1alpha1.BMCSettings
		oldObj     *metalv1alpha1.BMCSettings
		validator  BMCSettingsCustomValidator
		serverOne  *metalv1alpha1.Server
		serverTwo  *metalv1alpha1.Server
		bmcNameOne string
		bmcNameTwo string
	)

	BeforeEach(func() {
		obj = &metalv1alpha1.BMCSettings{}
		oldObj = &metalv1alpha1.BMCSettings{}
		validator = BMCSettingsCustomValidator{Client: k8sClient}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
		bmcNameOne = "bmc-foo"
		bmcNameTwo = "bmc-bar"
		serverOne = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: "server-foo",
			},
			Spec: metalv1alpha1.ServerSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmcNameOne},
			},
		}
		Expect(k8sClient.Create(ctx, serverOne)).To(Succeed())
		DeferCleanup(k8sClient.Delete, serverOne)

		serverTwo = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: "server-bar",
			},
			Spec: metalv1alpha1.ServerSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmcNameTwo},
			},
		}
		Expect(k8sClient.Create(ctx, serverTwo)).To(Succeed())
		DeferCleanup(k8sClient.Delete, serverTwo)
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating or updating BMCSettings under Validating Webhook", func() {
		It("Should deny creation if a BMC referred is already referred by another", func(ctx SpecContext) {
			By("Creating an BMCSettings with ServerRefList")
			BMCSettingsV1 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
					ServerRefList:               []*v1.LocalObjectReference{{Name: serverOne.Name}},
					ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(k8sClient.Create(ctx, BMCSettingsV1)).To(Succeed())

			By("Creating another BMCSettings with reference to existing referred BMC through ServerRefList")
			BMCSettingsV2 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
					ServerRefList:               []*v1.LocalObjectReference{{Name: serverOne.Name}},
					ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(validator.ValidateCreate(ctx, BMCSettingsV2)).Error().To(HaveOccurred())

			By("Creating another BMCSettings with reference to existing referred BMC through BMCRef")
			BMCSettingsV2 = &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
					BMCRef:                      &v1.LocalObjectReference{Name: bmcNameOne},
					ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(validator.ValidateCreate(ctx, BMCSettingsV2)).Error().To(HaveOccurred())

			By("Deleting BMCSettings with ServerRefList")
			Expect(k8sClient.Delete(ctx, BMCSettingsV1)).To(Succeed())

			By("Creating an BMCSettings with BMCRef")
			BMCSettingsV1 = &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
					BMCRef:                      &v1.LocalObjectReference{Name: bmcNameOne},
					ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(k8sClient.Create(ctx, BMCSettingsV1)).To(Succeed())
			DeferCleanup(k8sClient.Delete, BMCSettingsV1)

			By("Creating another BMCSettings with reference to existing referred BMC through ServerRefList")
			BMCSettingsV2 = &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
					ServerRefList:               []*v1.LocalObjectReference{{Name: serverOne.Name}},
					ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(validator.ValidateCreate(ctx, BMCSettingsV2)).Error().To(HaveOccurred())

			By("Creating another BMCSettings with reference to existing referred BMC through BMCRef")
			BMCSettingsV2 = &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
					BMCRef:                      &v1.LocalObjectReference{Name: bmcNameOne},
					ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			}
			Expect(validator.ValidateCreate(ctx, BMCSettingsV2)).Error().To(HaveOccurred())
		})
	})

	It("Should create if a referenced BMC is NOT duplicate", func() {
		By("Creating an BMCSettings")
		BMCSettingsV1 := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
				ServerRefList:               []*v1.LocalObjectReference{{Name: serverOne.Name}},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, BMCSettingsV1)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCSettingsV1)

		By("Creating another BIOSSetting with different ServerRefList")
		BMCSettingsV2 := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
				ServerRefList:               []*v1.LocalObjectReference{{Name: serverTwo.Name}},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, BMCSettingsV2)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCSettingsV2)

		By("Creating another BIOSSetting with different ServerRefList")
		BMCSettingsV3 := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
				BMCRef:                      &v1.LocalObjectReference{Name: "foo-bar-BMC"},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, BMCSettingsV3)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCSettingsV3)
	})

	It("Should deny Update if a BMC referred is already referred by another", func() {
		By("Creating an BMCSettings")
		BMCSettingsV1 := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
				ServerRefList:               []*v1.LocalObjectReference{{Name: serverOne.Name}},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, BMCSettingsV1)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCSettingsV1)

		By("Creating another BIOSSetting with different ServerRefList")
		BMCSettingsV2 := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
				BMCRef:                      &v1.LocalObjectReference{Name: bmcNameTwo},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, BMCSettingsV2)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCSettingsV2)

		By("Updating an BMCSettingsV2 to refer to conflicting BMCs through Spec.ServerRefList")
		BMCSettingsV2Updated := BMCSettingsV2.DeepCopy()
		BMCSettingsV2Updated.Spec.BMCRef = nil
		BMCSettingsV2Updated.Spec.ServerRefList = BMCSettingsV1.Spec.ServerRefList
		Expect(validator.ValidateUpdate(ctx, BMCSettingsV2, BMCSettingsV2Updated)).Error().To(HaveOccurred())

		By("Updating an BMCSettingsV2 to refer to conflicting BMCs through Spec.BMCRef")
		BMCSettingsV2Updated = BMCSettingsV2.DeepCopy()
		BMCSettingsV2Updated.Spec.ServerRefList = nil
		BMCSettingsV2Updated.Spec.BMCRef = &v1.LocalObjectReference{Name: bmcNameOne}
		Expect(validator.ValidateUpdate(ctx, BMCSettingsV2, BMCSettingsV2Updated)).Error().To(HaveOccurred())

		By("Updating an BMCSettingsV1 to refer to conflicting BMCs through Spec.ServerRefList")
		BMCSettingsV1Updated := BMCSettingsV1.DeepCopy()
		BMCSettingsV1Updated.Spec.BMCRef = nil
		BMCSettingsV1Updated.Spec.ServerRefList = []*v1.LocalObjectReference{{Name: serverTwo.Name}}
		Expect(validator.ValidateUpdate(ctx, BMCSettingsV1, BMCSettingsV1Updated)).Error().To(HaveOccurred())

		By("Updating an BMCSettingsV1 to refer to conflicting BMCs through Spec.BMCRef")
		BMCSettingsV1Updated = BMCSettingsV1.DeepCopy()
		BMCSettingsV1Updated.Spec.ServerRefList = nil
		BMCSettingsV1Updated.Spec.BMCRef = BMCSettingsV2.Spec.BMCRef
		Expect(validator.ValidateUpdate(ctx, BMCSettingsV1, BMCSettingsV1Updated)).Error().To(HaveOccurred())
	})

	It("Should Update if a BMC referred is NOT referred by another", func() {
		By("Creating an BMCSettings")
		BMCSettingsV1 := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
				ServerRefList:               []*v1.LocalObjectReference{{Name: serverOne.Name}},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, BMCSettingsV1)).To(Succeed())

		By("Updating an BMCSettingsV1 to refer to Non conflicting BMCs through Spec.ServerRefList")
		BMCSettingsV1Updated := BMCSettingsV1.DeepCopy()
		BMCSettingsV1Updated.Spec.BMCRef = nil
		BMCSettingsV1Updated.Spec.ServerRefList = []*v1.LocalObjectReference{{Name: serverTwo.Name}}
		Expect(validator.ValidateUpdate(ctx, BMCSettingsV1, BMCSettingsV1Updated)).Error().NotTo(HaveOccurred())
		By("reverting back")
		Expect(validator.ValidateUpdate(ctx, BMCSettingsV1, BMCSettingsV1)).Error().NotTo(HaveOccurred())

		By("Creating another BIOSSetting with different ServerRefList")
		BMCSettingsV2 := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettingsSpec:             metalv1alpha1.Settings{Version: "P70 v1.45 (12/06/2017)", SettingsMap: map[string]string{}},
				BMCRef:                      &v1.LocalObjectReference{Name: bmcNameTwo},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, BMCSettingsV2)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCSettingsV2)

		By("Updating an BMCSettingsV1 to refer to Non conflicting BMCs through Spec.BMCRef")
		BMCSettingsV1Updated = BMCSettingsV1.DeepCopy()
		BMCSettingsV1Updated.Spec.ServerRefList = nil
		BMCSettingsV1Updated.Spec.BMCRef = &v1.LocalObjectReference{Name: "new-bmc"}
		Expect(validator.ValidateUpdate(ctx, BMCSettingsV1, BMCSettingsV1Updated)).Error().NotTo(HaveOccurred())

		By("Updating an BMCSettingsV2 to refer to conflicting BMCs through Spec.ServerRefList")
		BMCSettingsV2Updated := BMCSettingsV2.DeepCopy()
		BMCSettingsV2Updated.Spec.BMCRef = &v1.LocalObjectReference{Name: "new-bmc2"}
		BMCSettingsV2Updated.Spec.ServerRefList = nil
		Expect(validator.ValidateUpdate(ctx, BMCSettingsV2, BMCSettingsV2Updated)).Error().NotTo(HaveOccurred())

		By("Deleting an BMCSettingsV1")
		Expect(k8sClient.Delete(ctx, BMCSettingsV1)).To(Succeed())

		By("Updating an BMCSettingsV2 to refer to conflicting BMCs through Spec.ServerRefList")
		BMCSettingsV2Updated = BMCSettingsV2.DeepCopy()
		BMCSettingsV2Updated.Spec.BMCRef = nil
		BMCSettingsV2Updated.Spec.ServerRefList = []*v1.LocalObjectReference{{Name: serverOne.Name}}
		Expect(validator.ValidateUpdate(ctx, BMCSettingsV2, BMCSettingsV2Updated)).Error().NotTo(HaveOccurred())
	})
})
