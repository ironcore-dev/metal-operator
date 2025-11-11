// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ironcore-dev/controller-utils/conditionutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

var _ = Describe("BMCVersion Controller", func() {
	ns := SetupTest()

	var (
		server                  *metalv1alpha1.Server
		bmcObj                  *metalv1alpha1.BMC
		bmcSecret               *metalv1alpha1.BMCSecret
		upgradeServerBMCVersion = "1.46.455b66-rev4"
	)

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-secret-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a BMC resource")
		bmcObj = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "23:11:8A:33:CF:EA",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: 8000,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcObj)).To(Succeed())

		By("Ensuring that the Server resource will be created")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmcObj),
			},
		}
		Eventually(Get(server)).Should(Succeed())

		By("Ensuring that the Server is in an available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmcObj)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))
	})

	AfterEach(func(ctx SpecContext) {
		bmc.UnitTestMockUps.ResetBMCVersionUpdate()

		Expect(k8sClient.Delete(ctx, bmcObj)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		EnsureCleanState()
	})

	It("Should successfully mark completed if no BMC version change", func(ctx SpecContext) {
		By("Creating a BMCVersion")
		bmcVersion := &metalv1alpha1.BMCVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCVersionSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmcObj.Name},
				BMCVersionTemplate: metalv1alpha1.BMCVersionTemplate{
					Version:                 defaultMockUpServerBMCVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: defaultMockUpServerBMCVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcVersion)).To(Succeed())

		By("Ensuring that BMC upgrade has completed")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
		)

		By("Ensuring that BMC upgrade Conditions has not created")
		Consistently(Object(bmcVersion)).Should(
			HaveField("Status.Conditions", BeNil()),
		)

		By("Ensuring that the Maintenance resource has NOT been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BMCVersion")
		Expect(k8sClient.Delete(ctx, bmcVersion)).To(Succeed())

		By("Ensuring that the BMCVersion has been removed")
		Eventually(Get(bmcVersion)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(bmcVersion)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should successfully Start and monitor Upgrade task to completion", func(ctx SpecContext) {
		// mocked at
		// metal-operator/bmc/redfish_local.go mockedBIOS*
		// note: ImageURI need to have the version string.

		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})

		By("Update the server state to Available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BMCVersion")
		bmcVersion := &metalv1alpha1.BMCVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCVersionSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmcObj.Name},
				BMCVersionTemplate: metalv1alpha1.BMCVersionTemplate{
					Version:                 upgradeServerBMCVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBMCVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcVersion)).To(Succeed())

		By("Ensuring that the bmcVersion has entered Inprogress state")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", HaveLen(1)))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      serverMaintenanceList.Items[0].Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by bmcVersion")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Spec.ServerMaintenanceRefs",
				[]metalv1alpha1.ServerMaintenanceRefItem{{
					ServerMaintenanceRef: &v1.ObjectReference{
						Kind:       "ServerMaintenance",
						Name:       serverMaintenance.Name,
						Namespace:  serverMaintenance.Namespace,
						UID:        serverMaintenance.UID,
						APIVersion: metalv1alpha1.GroupVersion.String(),
					}}}),
		)

		By("Ensuring that Server in Maintenance state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
		))

		ensureBMCVersionConditionTransition(ctx, acc, bmcVersion)

		By("Ensuring that BMC upgrade has completed")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
		)

		By("Ensuring that BMCVersion has removed Maintenance")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		)
		Consistently(Object(bmcVersion)).Should(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		)

		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BMCVersion")
		Expect(k8sClient.Delete(ctx, bmcVersion)).To(Succeed())

		By("Ensuring that the BMCVersion has been removed")
		Eventually(Get(bmcVersion)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(bmcVersion)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should upgrade servers BMC when server in reserved state", func(ctx SpecContext) {
		// mocked at
		// metal-operator/bmc/redfish_local.go mockedBIOS*
		// note: ImageURI need to have the version string.

		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
		By("Creating an Ignition secret")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: nil,
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

		By("Creating a ServerClaim")
		serverClaim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:             metalv1alpha1.PowerOn,
				ServerRef:         &v1.LocalObjectReference{Name: server.Name},
				IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
				Image:             "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())

		By("Ensuring that the Server has been claimed")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		)

		By("Creating a BMCVersion")
		bmcVersion := &metalv1alpha1.BMCVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCVersionSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmcObj.Name},
				BMCVersionTemplate: metalv1alpha1.BMCVersionTemplate{
					Version:                 upgradeServerBMCVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBMCVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcVersion)).To(Succeed())

		By("Ensuring that the bmcVersion has entered Inprogress state")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", HaveLen(1)))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      serverMaintenanceList.Items[0].Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by bmcVersion")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Spec.ServerMaintenanceRefs",
				[]metalv1alpha1.ServerMaintenanceRefItem{{
					ServerMaintenanceRef: &v1.ObjectReference{
						Kind:       "ServerMaintenance",
						Name:       serverMaintenance.Name,
						Namespace:  serverMaintenance.Namespace,
						UID:        serverMaintenance.UID,
						APIVersion: metalv1alpha1.GroupVersion.String(),
					}}}),
		)

		By("Ensuring that the bmcVersion has Inprogress state and waiting")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
		)

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, "true")
		})).Should(Succeed())

		By("Ensuring that Server in Maintenance state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
		))

		ensureBMCVersionConditionTransition(ctx, acc, bmcVersion)

		By("Ensuring that BMC upgrade has completed")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
		)

		By("Ensuring that BMCVersion has removed Maintenance")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		)
		Consistently(Object(bmcVersion)).Should(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		)

		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BMCVersion")
		Expect(k8sClient.Delete(ctx, bmcVersion)).To(Succeed())

		By("Ensuring that the BiosVersion has been removed")
		Eventually(Get(bmcVersion)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(bmcVersion)).Should(Satisfy(apierrors.IsNotFound))

		// cleanup
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
	})

	It("Should allow retry using annotation", func(ctx SpecContext) {
		By("Creating a BMCVersion")
		bmcVersion := &metalv1alpha1.BMCVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCVersionSpec{
				BMCRef: &v1.LocalObjectReference{Name: bmcObj.Name},
				BMCVersionTemplate: metalv1alpha1.BMCVersionTemplate{
					Version:                 upgradeServerBMCVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBMCVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcVersion)).To(Succeed())

		By("Moving to Failed state")
		Eventually(UpdateStatus(bmcVersion, func() {
			bmcVersion.Status.State = metalv1alpha1.BMCVersionStateFailed
		})).Should(Succeed())

		Eventually(Update(bmcVersion, func() {
			bmcVersion.Annotations = map[string]string{
				metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationRetryFailed,
			}
		})).Should(Succeed())

		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
		)

		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
		)

		// cleanup
		Expect(k8sClient.Delete(ctx, bmcVersion)).To(Succeed())
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())
	})
})

func ensureBMCVersionConditionTransition(ctx context.Context, acc *conditionutils.Accessor, bmcVersion *metalv1alpha1.BMCVersion) {
	GinkgoHelper()

	By("Ensuring that BMC Conditions have reached expected state 'biosVersionUpgradeIssued'")
	condIssue := &metav1.Condition{}
	Eventually(func(g Gomega) int {
		g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
		return len(bmcVersion.Status.Conditions)
	}).Should(BeNumerically(">=", 1))
	Eventually(func(g Gomega) bool {
		g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
		g.Expect(acc.FindSlice(bmcVersion.Status.Conditions, bmcVersionUpgradeIssued, condIssue)).To(BeTrue())
		return condIssue.Status == metav1.ConditionTrue
	}).Should(BeTrue())

	By("Ensuring that BMCVersion has updated the task Status with task URI")
	Eventually(Object(bmcVersion)).Should(
		HaveField("Status.UpgradeTask.URI", "dummyTask"),
	)

	By("Ensuring that BMC Conditions have reached expected state 'biosVersionUpgradeCompleted'")
	condComplete := &metav1.Condition{}
	Eventually(func(g Gomega) int {
		g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
		return len(bmcVersion.Status.Conditions)
	}).Should(BeNumerically(">=", 2))
	Eventually(func(g Gomega) bool {
		g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
		g.Expect(acc.FindSlice(bmcVersion.Status.Conditions, bmcVersionUpgradeCompleted, condComplete)).To(BeTrue())
		return condComplete.Status == metav1.ConditionTrue
	}).Should(BeTrue())

	By("Ensuring that BMC Conditions have reached expected state 'biosVersionUpgradeVerficationCondition'")
	verificationComplete := &metav1.Condition{}
	Eventually(func(g Gomega) int {
		g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
		return len(bmcVersion.Status.Conditions)
	}).Should(BeNumerically(">=", 4))
	Eventually(func(g Gomega) bool {
		g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
		g.Expect(acc.FindSlice(bmcVersion.Status.Conditions, bmcVersionUpgradeVerficationCondition, verificationComplete)).To(BeTrue())
		return verificationComplete.Status == metav1.ConditionTrue
	}).Should(BeTrue())
}
