// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BIOSSettings Webhook", func() {
	var (
		biosSettingsV1                 *metalv1alpha1.BIOSSettings
		validator                      BIOSSettingsCustomValidator
		defaultMockUpServerBiosVersion = "P79 v1.45 (12/06/2017)"
		anotherMockUpServerBiosVersion = "P71 v1.45 (12/06/2017)"
	)

	BeforeEach(func(ctx SpecContext) {
		validator = BIOSSettingsCustomValidator{Client: k8sClient}
		By("Creating a BIOSSettings")
		biosSettingsV1 = &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: "foo"},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: map[string]string{},
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV1)).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		By("Deleting the BIOSSettings and Server resources")
		Expect(k8sClient.DeleteAllOf(ctx, &metalv1alpha1.BIOSSettings{})).To(Succeed())
		By("Deleting Server resources if created")
		Expect(client.IgnoreNotFound(k8sClient.DeleteAllOf(ctx, &metalv1alpha1.Server{}))).To(Succeed())
		By("Deleting ServerMaintenance resources if created")
		Expect(client.IgnoreNotFound(k8sClient.DeleteAllOf(ctx, &metalv1alpha1.ServerMaintenance{}))).To(Succeed())
	})

	It("should deny creation if a Server already has a BIOSSettings", func(ctx SpecContext) {
		By("Creating another BIOSSettings targeting the same Server")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: "foo"},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: map[string]string{},
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(validator.ValidateCreate(ctx, biosSettingsV2)).Error().To(HaveOccurred())
	})

	It("should allow creating a BIOSSettings for a Server without one", func() {
		By("Creating a BIOSSettings targeting a new Server")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: defaultMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: map[string]string{},
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())
	})

	It("should deny update if spec.serverRef and version are both duplicate", func() {
		By("Creating a BIOSSettings with different ServerRef")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: anotherMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: map[string]string{},
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())

		By("Updating biosSettingsV2 with both a conflicting ServerRef and matching version")
		biosSettingsV2Updated := biosSettingsV2.DeepCopy()
		biosSettingsV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		biosSettingsV2Updated.Spec.Version = defaultMockUpServerBiosVersion
		Expect(validator.ValidateUpdate(ctx, biosSettingsV2, biosSettingsV2Updated)).Error().To(HaveOccurred())
	})

	It("should allow update if spec.serverRef changes to an existing one but version differs", func() {
		By("Creating a BIOSSettings with different ServerRef")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: anotherMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: map[string]string{},
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())

		By("Updating biosSettingsV2 with a serverRef matching V1 but a different version")
		biosSettingsV2Updated := biosSettingsV2.DeepCopy()
		biosSettingsV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		Expect(validator.ValidateUpdate(ctx, biosSettingsV2, biosSettingsV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("should allow update when spec.serverRef is unchanged even if version now matches another record", func() {
		By("Creating a BIOSSettings with different ServerRef")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: anotherMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: map[string]string{},
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())

		By("Updating biosSettingsV2 version to match V1 without changing serverRef")
		biosSettingsV2Updated := biosSettingsV2.DeepCopy()
		biosSettingsV2Updated.Spec.Version = defaultMockUpServerBiosVersion
		Expect(validator.ValidateUpdate(ctx, biosSettingsV2, biosSettingsV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("should allow update if a different field is duplicate", func() {
		By("Creating a BIOSSetting with different ServerRef")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: anotherMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: map[string]string{},
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())

		By("Updating an biosSettingsV2 with conflicting Spec.BIOSSettings")
		biosSettingsV2Updated := biosSettingsV2.DeepCopy()
		biosSettingsV2Updated.Spec.Version = biosSettingsV1.Spec.Version
		Expect(validator.ValidateUpdate(ctx, biosSettingsV2, biosSettingsV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("should allow update if a ServerRef field is not duplicate", func() {
		By("Creating a BIOSSetting with different ServerRef")
		biosSettingsV2 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSSettingsSpec{
				ServerRef: &v1.LocalObjectReference{Name: "bar"},
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version: anotherMockUpServerBiosVersion,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{{
						Settings: map[string]string{},
						Priority: 1,
						Name:     "one",
					}},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsV2)).To(Succeed())

		By("Updating an biosSettingsV2 to a non conflicting ServerRef")
		biosSettingsV2Updated := biosSettingsV2.DeepCopy()
		biosSettingsV2Updated.Spec.ServerRef = &v1.LocalObjectReference{Name: "foobar"}
		Expect(validator.ValidateUpdate(ctx, biosSettingsV2, biosSettingsV2Updated)).Error().ToNot(HaveOccurred())
	})

	It("should not allow update of BIOSSettings which are in-progress, but should allow forcefully deleting it", func() {
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

		By("Patching the BIOSSettings V1 to InProgress state")
		Eventually(UpdateStatus(biosSettingsV1, func() {
			biosSettingsV1.Status.State = metalv1alpha1.BIOSSettingsStateInProgress
		})).Should(Succeed())

		By("Mocking a corresponding ServerMaintenance for the BIOSSettings V1")
		Eventually(Update(biosSettingsV1, func() {
			biosSettingsV1.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{Name: sm.Name, Namespace: sm.Namespace}
		})).Should(Succeed())

		By("Denying the spec update of an in-progress BIOSSettings")
		biosSettingsV1Updated := biosSettingsV1.DeepCopy()
		biosSettingsV1Updated.Spec.SettingsFlow = []metalv1alpha1.SettingsFlowItem{{Priority: 1, Settings: map[string]string{"test": "value"}}}
		Expect(validator.ValidateUpdate(ctx, biosSettingsV1, biosSettingsV1Updated)).Error().To(HaveOccurred())

		By("Allowing the spec update of an in-progress BIOSSettings with force-update annotation")
		biosSettingsV1Updated.Annotations = map[string]string{metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationForceUpdateInProgress}
		Expect(validator.ValidateUpdate(ctx, biosSettingsV1, biosSettingsV1Updated)).Error().ToNot(HaveOccurred())

		By("Ensuring the BIOSSettings V1 is back to Applied state")
		Eventually(UpdateStatus(biosSettingsV1, func() {
			biosSettingsV1.Status.State = metalv1alpha1.BIOSSettingsStateApplied
		})).Should(Succeed())

		Eventually(UpdateStatus(sm, func() {
			sm.Status.State = metalv1alpha1.ServerMaintenanceStatePending
		})).Should(Succeed())
	})

	It("should deny deletion of an in-progress BIOSSettings", func() {
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

		By("Patching the BIOSSettings V1 to InProgress state")
		Eventually(UpdateStatus(biosSettingsV1, func() {
			biosSettingsV1.Status.State = metalv1alpha1.BIOSSettingsStateInProgress
		})).Should(Succeed())

		By("Setting ServerMaintenanceRef on BIOSSettings V1")
		Eventually(Update(biosSettingsV1, func() {
			biosSettingsV1.Spec.ServerMaintenanceRef = &metalv1alpha1.ObjectReference{Name: sm.Name, Namespace: sm.Namespace}
		})).Should(Succeed())

		By("Denying the deletion of an in-progress BIOSSettings")
		Expect(k8sClient.Delete(ctx, biosSettingsV1)).To(Not(Succeed()))

		By("Ensuring the BIOSSettings V1 is back to Applied state")
		Eventually(UpdateStatus(biosSettingsV1, func() {
			biosSettingsV1.Status.State = metalv1alpha1.BIOSSettingsStateApplied
		})).Should(Succeed())

		Eventually(UpdateStatus(sm, func() {
			sm.Status.State = metalv1alpha1.ServerMaintenanceStatePending
		})).Should(Succeed())
	})
})
