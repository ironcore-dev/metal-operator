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

var _ = Describe("BIOSVersion Webhook", func() {
	var (
		biosVersionV1 *metalv1alpha1.BIOSVersion
		validator     BIOSVersionCustomValidator
	)

	BeforeEach(func() {
		validator = BIOSVersionCustomValidator{Client: k8sClient}
		By("Creating a BIOSVersion")
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
	})

	It("should deny creation if spec.serverRef is duplicate", func(ctx SpecContext) {
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

	It("should create if a spec.serverRef field is not a duplicate", func() {
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
		Expect(validator.ValidateCreate(ctx, biosVersionV2)).Error().ToNot(HaveOccurred())
	})

	It("should deny update if spec.serverRef is duplicate", func() {
		By("Creating a BIOSVersion with different ServerRef")
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

		By("Updating a BIOSVersion V2 to conflicting spec.serverRef")
		biosVersionV2Updated := biosVersionV2.DeepCopy()
		biosVersionV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		Expect(validator.ValidateUpdate(ctx, biosVersionV2, biosVersionV2Updated)).Error().To(HaveOccurred())
	})

	It("should allow update if a different field is duplicate", func() {
		By("Creating a BIOSVersion with different ServerRef")
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

		By("Updating BIOSVersion V2 to duplicate spec.image")
		biosVersionV2Updated := biosVersionV2.DeepCopy()
		biosVersionV2Updated.Spec.Image = biosVersionV1.Spec.Image
		Expect(validator.ValidateUpdate(ctx, biosVersionV2, biosVersionV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("should allow update if spec.serverRef changes to an existing one but version differs", func() {
		By("Creating a BIOSVersion with different ServerRef and version")
		biosVersionV2 := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 "P79 v1.45 (12/06/2017)",
					Image:                   metalv1alpha1.ImageSpec{URI: "two"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
			},
		}
		Expect(k8sClient.Create(ctx, biosVersionV2)).To(Succeed())

		By("Updating BIOSVersion V2 serverRef to match V1 while keeping a different version")
		biosVersionV2Updated := biosVersionV2.DeepCopy()
		biosVersionV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		Expect(validator.ValidateUpdate(ctx, biosVersionV2, biosVersionV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("should allow update when spec.serverRef is unchanged even if version now matches another record", func() {
		By("Creating a BIOSVersion with different ServerRef")
		biosVersionV2 := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 "P79 v1.45 (12/06/2017)",
					Image:                   metalv1alpha1.ImageSpec{URI: "two"},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
			},
		}
		Expect(k8sClient.Create(ctx, biosVersionV2)).To(Succeed())

		By("Updating BIOSVersion V2 version to match V1 without changing serverRef")
		biosVersionV2Updated := biosVersionV2.DeepCopy()
		biosVersionV2Updated.Spec.Version = biosVersionV1.Spec.Version
		Expect(validator.ValidateUpdate(ctx, biosVersionV2, biosVersionV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("should allow update if a ServerRef field is not a duplicate", func() {
		By("Creating a BIOSVersion with different ServerRef")
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

		By("Updating a BIOSVersion V2 to a non-conflicting spec.serverRef")
		biosVersionV2Updated := biosVersionV2.DeepCopy()
		biosVersionV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foobar"}
		Expect(validator.ValidateUpdate(ctx, biosVersionV2, biosVersionV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("should not allow update when BIOSVersion is in progress, but should allow force update", func() {
		By("Creating a ServerMaintenance in InMaintenance state")
		sm := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-sm-",
				Namespace:    metav1.NamespaceDefault,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				Policy:    metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, sm)).To(Succeed())
		Eventually(UpdateStatus(sm, func() {
			sm.Status.State = metalv1alpha1.ServerMaintenanceStateInMaintenance
		})).Should(Succeed())

		By("Patching the BIOSVersion V1 to in-progress state")
		Eventually(UpdateStatus(biosVersionV1, func() {
			biosVersionV1.Status.State = metalv1alpha1.BIOSVersionStateInProgress
		})).Should(Succeed())

		By("Adding ServerMaintenance reference")
		Eventually(Update(biosVersionV1, func() {
			biosVersionV1.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{Name: sm.Name, Namespace: sm.Namespace}
		})).Should(Succeed())

		By("Updating the BIOSVersion V1 spec should fail to update while maintenance is active")
		biosVersionV1Updated := biosVersionV1.DeepCopy()
		biosVersionV1Updated.Spec.Version = "P712"
		Expect(validator.ValidateUpdate(ctx, biosVersionV1, biosVersionV1Updated)).Error().To(HaveOccurred())

		By("Updating BIOSVersion V1 spec should succeed with ForceUpdateInProgress annotation")
		biosVersionV1Updated.Annotations = map[string]string{metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationForceUpdateInProgress}
		Expect(validator.ValidateUpdate(ctx, biosVersionV1, biosVersionV1Updated)).Error().ToNot(HaveOccurred())

		By("Patching the BIOSVersion V1 to Completed state")
		Eventually(UpdateStatus(biosVersionV1, func() {
			biosVersionV1.Status.State = metalv1alpha1.BIOSVersionStateCompleted
		})).Should(Succeed())

		Eventually(UpdateStatus(sm, func() {
			sm.Status.State = metalv1alpha1.ServerMaintenanceStatePending
		})).Should(Succeed())
	})

	It("should refuse to delete while ServerMaintenance is active", func() {
		By("Creating a ServerMaintenance in InMaintenance state")
		sm := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-sm-",
				Namespace:    metav1.NamespaceDefault,
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				Policy:    metalv1alpha1.ServerMaintenancePolicyEnforced,
				ServerRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, sm)).To(Succeed())
		Eventually(UpdateStatus(sm, func() {
			sm.Status.State = metalv1alpha1.ServerMaintenanceStateInMaintenance
		})).Should(Succeed())

		By("Setting ServerMaintenanceRef on BIOSVersion V1")
		Eventually(Update(biosVersionV1, func() {
			biosVersionV1.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{Name: sm.Name, Namespace: sm.Namespace}
		})).Should(Succeed())

		By("Validating deletion of BIOSVersion V1 should fail while maintenance is active")
		Expect(validator.ValidateDelete(ctx, biosVersionV1)).Error().To(HaveOccurred())

		By("Deactivating the ServerMaintenance")
		Eventually(UpdateStatus(sm, func() {
			sm.Status.State = metalv1alpha1.ServerMaintenanceStatePending
		})).Should(Succeed())

		By("Validating deletion of BIOSVersion V1 should succeed once maintenance is inactive")
		Expect(validator.ValidateDelete(ctx, biosVersionV1)).Error().ToNot(HaveOccurred())
	})
})
