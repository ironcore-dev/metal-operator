// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMCSettings Webhook", func() {
	var (
		BMCSettingsV1 *metalv1alpha1.BMCSettings
		validator     BMCSettingsCustomValidator
	)

	BeforeEach(func() {
		BMCSettingsV1 = &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCRef: &v1.LocalObjectReference{Name: "foo"},
				BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
					Version:                 "P70 v1.45 (12/06/2017)",
					SettingsMap:             map[string]string{},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				}},
		}
		By("Creating an BMCSettings")
		Expect(k8sClient.Create(ctx, BMCSettingsV1)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCSettingsV1)
		validator = BMCSettingsCustomValidator{Client: k8sClient}
		SetClient(k8sClient)

	})

	Context("When creating or updating BMCSettings under Validating Webhook", func() {

		It("Should deny creation if a BMC referred is already referred by another", func(ctx SpecContext) {
			By("Creating another BMCSettings with reference to existing referred BMC")
			BMCSettingsV2 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-bmc-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCRef: &v1.LocalObjectReference{Name: "foo"},
					BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
						Version:                 "1.45.455b66-rev4",
						SettingsMap:             map[string]string{},
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					}},
			}
			Expect(validator.ValidateCreate(ctx, BMCSettingsV2)).Error().To(HaveOccurred())
		})

		It("Should create if a referenced BMC is NOT duplicate", func() {
			By("Creating another BMCSetting for different BMCRef")
			BMCSettingsV2 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCRef: &v1.LocalObjectReference{Name: "bar"},
					BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
						Version:                 "P70 v1.45 (12/06/2017)",
						SettingsMap:             map[string]string{},
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					}},
			}
			Expect(k8sClient.Create(ctx, BMCSettingsV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, BMCSettingsV2)
		})

		It("Should deny Update if a BMC referred is already referred by another", func() {
			By("Creating another BMCSetting with different BMCRef")
			BMCSettingsV2 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCRef: &v1.LocalObjectReference{Name: "bar"},
					BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
						Version:                 "P70 v1.45 (12/06/2017)",
						SettingsMap:             map[string]string{},
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					}},
			}
			Expect(k8sClient.Create(ctx, BMCSettingsV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, BMCSettingsV2)

			By("Updating an BMCSettingsV2 to refer to existing BMC")
			BMCSettingsV2Updated := BMCSettingsV2.DeepCopy()
			BMCSettingsV2Updated.Spec.BMCRef = BMCSettingsV1.Spec.BMCRef
			Expect(validator.ValidateUpdate(ctx, BMCSettingsV2, BMCSettingsV2Updated)).Error().To(HaveOccurred())
		})

		It("Should Update if a BMC referred is NOT referred by another", func() {
			By("Creating another BMCSetting with different BMCref")
			BMCSettingsV2 := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCRef: &v1.LocalObjectReference{Name: "bar"},
					BMCSettingsTemplate: metalv1alpha1.BMCSettingsTemplate{
						Version:                 "P70 v1.45 (12/06/2017)",
						SettingsMap:             map[string]string{},
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					}},
			}
			Expect(k8sClient.Create(ctx, BMCSettingsV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, BMCSettingsV2)

			By("Updating an BMCSettingsV2 to refer to new BMC")
			BMCSettingsV2Updated := BMCSettingsV2.DeepCopy()
			BMCSettingsV2Updated.Spec.BMCRef = &v1.LocalObjectReference{Name: "new-bmc2"}
			Expect(validator.ValidateUpdate(ctx, BMCSettingsV2, BMCSettingsV2Updated)).Error().NotTo(HaveOccurred())
		})

		It("Should NOT allow update settings is in progress. but should allow to Force it", func() {
			By("Patching the bmcSettings V1 to Inprogress state")
			Eventually(UpdateStatus(BMCSettingsV1, func() {
				BMCSettingsV1.Status.State = metalv1alpha1.BMCSettingsStateInProgress
			})).Should(Succeed())
			By("mock servermaintenance Creation maintenance")
			Eventually(Update(BMCSettingsV1, func() {
				BMCSettingsV1.Spec.ServerMaintenanceRefs = []metalv1alpha1.ServerMaintenanceRefItem{
					{ServerMaintenanceRef: &v1.ObjectReference{Name: "foobar-Maintenance"}},
				}
			})).Should(Succeed())
			By("Updating an bmcSettings V1 spec, should fail to update when inProgress")
			bmcSettingsV1Updated := BMCSettingsV1.DeepCopy()
			bmcSettingsV1Updated.Spec.SettingsMap = map[string]string{"test": "value"}
			Expect(validator.ValidateUpdate(ctx, BMCSettingsV1, bmcSettingsV1Updated)).Error().To(HaveOccurred())
			By("Updating an bmcSettings V1 spec, should pass to update when inProgress with ForceUpdateResource finalizer")
			bmcSettingsV1Updated.Annotations = map[string]string{metalv1alpha1.ForceUpdateAnnotation: metalv1alpha1.OperationAnnotationForceUpdateInProgress}
			Expect(validator.ValidateUpdate(ctx, BMCSettingsV1, bmcSettingsV1Updated)).Error().ToNot(HaveOccurred())

			Eventually(UpdateStatus(BMCSettingsV1, func() {
				BMCSettingsV1.Status.State = metalv1alpha1.BMCSettingsStateApplied
			})).Should(Succeed())
		})

		It("Should refuse to delete if InProgress", func() {
			By("Patching the BMCSettings V1 to a InProgress state")
			Eventually(UpdateStatus(BMCSettingsV1, func() {
				BMCSettingsV1.Status.State = metalv1alpha1.BMCSettingsStateInProgress
			})).Should(Succeed())

			By("Deleting the BMCSettings V1 should fail")
			Expect(k8sClient.Delete(ctx, BMCSettingsV1)).To(Not(Succeed()), fmt.Sprintf("BMCSettings state %v", BMCSettingsV1.Status.State))

			Eventually(UpdateStatus(BMCSettingsV1, func() {
				BMCSettingsV1.Status.State = metalv1alpha1.BMCSettingsStateApplied
			})).Should(Succeed())

			By("Deleting the BMCSettings should pass: by DeferCleanup")
		})
	})
})
