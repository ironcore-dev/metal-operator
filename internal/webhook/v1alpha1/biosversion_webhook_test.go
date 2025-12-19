// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/ironcore-dev/metal-operator/internal/controller"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BIOSVersion Webhook", func() {
	var (
		biosVersionV1 *metalv1alpha1.BIOSVersion
		validator     BIOSVersionCustomValidator
	)

	BeforeEach(func() {
		validator = BIOSVersionCustomValidator{Client: k8sClient}
		By("Creating an BIOSVersion")
		biosVersionV1 = &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 "P71 v1.45 (12/06/2017)",
					Image:                   metalv1alpha1.ImageSpec{URI: "one"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, biosVersionV1)).To(Succeed())
	})

	AfterEach(func() {
		By("Deleting the BIOSVersion resources")
		Expect(k8sClient.DeleteAllOf(ctx, &metalv1alpha1.BIOSVersion{})).To(Succeed())

		By("Ensuring clean state")
		controller.EnsureCleanState()
	})

	It("Should deny creation if a spec.serverRef field is duplicate", func(ctx SpecContext) {
		By("Creating another BIOSVersion with existing ServerRef")
		biosVersionV2 := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 "P71 v1.45 (12/06/2017)",
					Image:                   metalv1alpha1.ImageSpec{URI: "two"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(validator.ValidateCreate(ctx, biosVersionV2)).Error().To(HaveOccurred())
	})

	It("Should create if a spec.serverRef field is not a duplicate", func() {
		By("Creating another BIOSVersion with different ServerRef")
		biosVersionV2 := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 "P71 v1.45 (12/06/2017)",
					Image:                   metalv1alpha1.ImageSpec{URI: "asd"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
			},
		}
		Expect(k8sClient.Create(ctx, biosVersionV2)).To(Succeed())
	})

	It("Should deny update if a spec.serverRef field is a duplicate", func() {
		By("Creating an BIOSVersion with different ServerRef")
		biosVersionV2 := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 "P71 v1.45 (12/06/2017)",
					Image:                   metalv1alpha1.ImageSpec{URI: "asd"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
			},
		}
		Expect(k8sClient.Create(ctx, biosVersionV2)).To(Succeed())

		By("Updating an BIOSVersion V2 to conflicting spec.serverRef")
		biosVersionV2Updated := biosVersionV2.DeepCopy()
		biosVersionV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		Expect(validator.ValidateUpdate(ctx, biosVersionV2, biosVersionV2Updated)).Error().To(HaveOccurred())
	})

	It("Should allow update if a different field is duplicate", func() {
		By("Creating an BIOSVersion with different ServerRef")
		biosVersionV2 := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 "P71 v1.45 (12/06/2017)",
					Image:                   metalv1alpha1.ImageSpec{URI: "two"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
			},
		}
		Expect(k8sClient.Create(ctx, biosVersionV2)).To(Succeed())

		By("Updating an BIOSVersion V2 to conflicting spec.biosVersionSpec")
		biosVersionV2Updated := biosVersionV2.DeepCopy()
		biosVersionV2Updated.Spec.Image = biosVersionV1.Spec.Image
		Expect(validator.ValidateUpdate(ctx, biosVersionV2, biosVersionV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("Should allow update if a ServerRef field is not a duplicate", func() {
		By("Creating an BIOSVersion with different ServerRef")
		biosVersionV2 := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 "P71 v1.45 (12/06/2017)",
					Image:                   metalv1alpha1.ImageSpec{URI: "asd"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
			},
		}
		Expect(k8sClient.Create(ctx, biosVersionV2)).To(Succeed())

		By("Updating an BIOSVersion V2 to a non-conflicting spec.serverRef ")
		biosVersionV2Updated := biosVersionV2.DeepCopy()
		biosVersionV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foobar"}
		Expect(validator.ValidateUpdate(ctx, biosVersionV2, biosVersionV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("Should no allow update a BIOSVersion is in progress, but should allow to force update it", func() {
		By("Patching the BIOSVersion V1 to in-progress state")
		Eventually(UpdateStatus(biosVersionV1, func() {
			biosVersionV1.Status.State = metalv1alpha1.BIOSVersionStateInProgress
		})).Should(Succeed())

		By("Add ServerMaintenance reference")
		Eventually(Update(biosVersionV1, func() {
			biosVersionV1.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{Name: "maintenance"}
		})).Should(Succeed())

		By("Updating the BIOSVersion V1 spec should fail to update when in InProgress state")
		biosVersionV1Updated := biosVersionV1.DeepCopy()
		biosVersionV1Updated.Spec.Version = "P712"
		Expect(validator.ValidateUpdate(ctx, biosVersionV1, biosVersionV1Updated)).Error().To(HaveOccurred())

		By("Updating an biosVersion V1 spec, should pass to update when inProgress with ForceUpdateResource finalizer")
		biosVersionV1Updated.Annotations = map[string]string{metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationForceUpdateInProgress}
		Expect(validator.ValidateUpdate(ctx, biosVersionV1, biosVersionV1Updated)).Error().ToNot(HaveOccurred())

		By("Patching the BIOSVersion V1 to Completed state")
		Eventually(UpdateStatus(biosVersionV1, func() {
			biosVersionV1.Status.State = metalv1alpha1.BIOSVersionStateCompleted
		})).Should(Succeed())
	})

	It("Should refuse to delete if InProgress", func() {
		By("Patching the BIOSVersionV1 to InProgress state")
		Eventually(UpdateStatus(biosVersionV1, func() {
			biosVersionV1.Status.State = metalv1alpha1.BIOSVersionStateInProgress
		})).Should(Succeed())

		By("Deleting the BIOSSettings V1 should fail")
		Expect(k8sClient.Delete(ctx, biosVersionV1)).To(Not(Succeed()))

		Eventually(UpdateStatus(biosVersionV1, func() {
			biosVersionV1.Status.State = metalv1alpha1.BIOSVersionStateCompleted
		})).Should(Succeed())
	})

})
