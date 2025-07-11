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
	"github.com/ironcore-dev/metal-operator/internal/controller"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMCVersion Webhook", func() {
	var (
		BMCVersionV1 *metalv1alpha1.BMCVersion
		validator    BMCVersionCustomValidator
	)

	BeforeEach(func() {
		validator = BMCVersionCustomValidator{Client: k8sClient}

		BMCVersionV1 = &metalv1alpha1.BMCVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-bmc-ver",
			},
			Spec: metalv1alpha1.BMCVersionSpec{
				Version: "P70 v1.45 (12/06/2017)",
				Image:   metalv1alpha1.ImageSpec{URI: "P70 v1.45 (12/06/2017)"},
				BMCRef:  &v1.LocalObjectReference{Name: "foo"},
			},
		}
		By("Creating an BMCVersion")
		Expect(k8sClient.Create(ctx, BMCVersionV1)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCVersionV1)
		SetClient(k8sClient)

	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating or updating BMCVersion under Validating Webhook", func() {
		It("Should deny creation if a BMC referred is already referred by another", func(ctx SpecContext) {
			By("Creating another BMCVersion with reference to existing referred BMC")
			BMCVersionV2 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-bmc-ver",
				},
				Spec: metalv1alpha1.BMCVersionSpec{
					Version: "P71 v1.45 (12/06/2017)",
					Image:   metalv1alpha1.ImageSpec{URI: "P71 v1.45 (12/06/2017)"},
					BMCRef:  &v1.LocalObjectReference{Name: "foo"},
				},
			}
			Expect(validator.ValidateCreate(ctx, BMCVersionV2)).Error().To(HaveOccurred())
		})

		It("Should create if a referenced BMC is NOT duplicate", func() {
			By("Creating another BMCVersion for different BMCRef")
			BMCVersionV2 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-bmc-ver",
				},
				Spec: metalv1alpha1.BMCVersionSpec{
					Version: "P70 v1.45 (12/06/2017)",
					Image:   metalv1alpha1.ImageSpec{URI: "P70 v1.45 (12/06/2017)"},
					BMCRef:  &v1.LocalObjectReference{Name: "bar"},
				},
			}
			Expect(k8sClient.Create(ctx, BMCVersionV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, BMCVersionV2)
		})

		It("Should deny Update if a BMC referred is already referred by another", func() {
			By("Creating another BMCVersion with different BMCRef")
			BMCVersionV2 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-bmc-ver",
				},
				Spec: metalv1alpha1.BMCVersionSpec{
					Version: "P71 v1.45 (12/06/2017)",
					Image:   metalv1alpha1.ImageSpec{URI: "P71 v1.45 (12/06/2017)"},
					BMCRef:  &v1.LocalObjectReference{Name: "bar"},
				},
			}
			Expect(k8sClient.Create(ctx, BMCVersionV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, BMCVersionV2)

			By("Updating an BMCVersionV2 to refer to existing BMC")
			BMCVersionV2Updated := BMCVersionV2.DeepCopy()
			BMCVersionV2Updated.Spec.BMCRef = BMCVersionV1.Spec.BMCRef
			Expect(validator.ValidateUpdate(ctx, BMCVersionV1, BMCVersionV2Updated)).Error().To(HaveOccurred())
		})

		It("Should Update if a BMC referred is NOT referred by another", func() {
			By("Creating another BMCVersion with different BMCref")
			BMCVersionV2 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-bmc-ver",
				},
				Spec: metalv1alpha1.BMCVersionSpec{
					Version: "P71 v1.45 (12/06/2017)",
					Image:   metalv1alpha1.ImageSpec{URI: "P71 v1.45 (12/06/2017)"},
					BMCRef:  &v1.LocalObjectReference{Name: "bar"},
				},
			}
			Expect(k8sClient.Create(ctx, BMCVersionV2)).To(Succeed())
			DeferCleanup(k8sClient.Delete, BMCVersionV2)

			By("Updating an BMCVersionV2 to refer to new BMC")
			BMCVersionV2Updated := BMCVersionV2.DeepCopy()
			BMCVersionV2Updated.Spec.BMCRef = &v1.LocalObjectReference{Name: "new-bmc2"}
			Expect(validator.ValidateUpdate(ctx, BMCVersionV2, BMCVersionV2Updated)).Error().NotTo(HaveOccurred())
		})

		It("Should NOT allow update settings is in progress. but should allow to Force it", func() {
			By("Patching the biosSettings V1 to Inprogress state")
			Eventually(UpdateStatus(BMCVersionV1, func() {
				BMCVersionV1.Status.State = metalv1alpha1.BMCVersionStateInProgress
			})).Should(Succeed())
			By("Updating an biosSettingsV1 spec, should fail to update when inProgress")
			BMCVersionV1Updated := BMCVersionV1.DeepCopy()
			BMCVersionV1Updated.Spec.Version = "P72"
			Expect(validator.ValidateUpdate(ctx, BMCVersionV1, BMCVersionV1Updated)).Error().To(HaveOccurred())
			By("Updating an biosSettingsV1 spec, should pass to update when inProgress with ForceUpdateResource finalizer")
			BMCVersionV1Updated.Finalizers = append(BMCVersionV1Updated.Finalizers, metalv1alpha1.ForceUpdateResource)
			Expect(validator.ValidateUpdate(ctx, BMCVersionV1, BMCVersionV1Updated)).Error().ToNot(HaveOccurred())
		})

		It("Should refuse to delete if InProgress", func() {
			By("Creating an BMCVersion V2 with different ServerRef")
			BMCVersionV2 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    "ns.Name",
					GenerateName: "test-bmc-ver",
				},
				Spec: metalv1alpha1.BMCVersionSpec{
					Version: "P71 v1.45 (12/06/2017)",
					Image:   metalv1alpha1.ImageSpec{URI: "P71 v1.45 (12/06/2017)"},
					BMCRef:  &v1.LocalObjectReference{Name: "bar"},
				},
			}
			Expect(k8sClient.Create(ctx, BMCVersionV2)).To(Succeed())
			By("Patching the BMCVersion V2 to Inprogress state")
			Eventually(UpdateStatus(BMCVersionV2, func() {
				BMCVersionV2.Status.State = metalv1alpha1.BMCVersionStateInProgress
			})).Should(Succeed())
			By("Deleting the BMCVersion V2 should pass: without the finalizer")
			Expect(k8sClient.Delete(ctx, BMCVersionV2)).To(Succeed())

			By("Patching the BMCVersionV1 to a Inprogress state and adding finalizer")
			Eventually(Update(BMCVersionV1, func() {
				BMCVersionV1.Finalizers = append(BMCVersionV1.Finalizers, controller.BMCVersionFinalizer)
			})).Should(Succeed())
			Eventually(UpdateStatus(BMCVersionV1, func() {
				BMCVersionV1.Status.State = metalv1alpha1.BMCVersionStateInProgress
			})).Should(Succeed())

			By("Deleting the BMCVersionV1 should fail")
			Expect(k8sClient.Delete(ctx, BMCVersionV1)).To(Not(Succeed()), fmt.Sprintf("bmc version state %v", BMCVersionV1.Status.State))

			Eventually(UpdateStatus(BMCVersionV1, func() {
				BMCVersionV1.Status.State = metalv1alpha1.BMCVersionStateCompleted
			})).Should(Succeed())

			By("Deleting the BMCVersionV1 should pass: by DeferCleanup")
		})
	})

})
