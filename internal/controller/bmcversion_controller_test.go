// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

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
	ns.Name = "default"

	var (
		server                  *metalv1alpha1.Server
		upgradeServerBMCVersion string
		bmcCRD                  *metalv1alpha1.BMC
	)

	BeforeEach(func(ctx SpecContext) {
		upgradeServerBMCVersion = "1.46.455b66-rev4"
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
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
		bmcCRD = &metalv1alpha1.BMC{
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
		Expect(k8sClient.Create(ctx, bmcCRD)).To(Succeed())

		Eventually(Get(bmcCRD)).Should(Succeed())

		By("Ensuring that the Server resource will be created")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmcCRD),
			},
		}
		Eventually(Get(server)).Should(Succeed())

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmcCRD)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
		bmc.UnitTestMockUps.ResetBMCVersionUpdate()
	})

	It("should successfully mark completed if no BMC version change", func(ctx SpecContext) {

		By("Creating a BMCVersion")
		bmcVersion := &metalv1alpha1.BMCVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCVersionSpec{
				Version:                 defaultMockUpServerBMCVersion,
				Image:                   metalv1alpha1.ImageSpec{URI: defaultMockUpServerBMCVersion},
				BMCRef:                  &v1.LocalObjectReference{Name: bmcCRD.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
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

	It("should successfully Start and monitor Upgrade task to completion", func(ctx SpecContext) {
		// mocked at
		// metal-operator/bmc/redfish_local.go mockedBIOS*
		// note: ImageURI need to have the version string.

		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})

		By("update the server state to Available  state")
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
				Version:                 upgradeServerBMCVersion,
				Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBMCVersion},
				BMCRef:                  &v1.LocalObjectReference{Name: bmcCRD.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, bmcVersion)).To(Succeed())

		By("Ensuring that the bmcVersion has entered Inprogress state")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      fmt.Sprintf("%s-%s", bmcVersion.Name, server.Name),
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by bmcVersion")
		Eventually(Object(bmcVersion)).Should(SatisfyAny(
			HaveField("Spec.ServerMaintenanceRefs",
				[]metalv1alpha1.ServerMaintenanceRefItem{{
					ServerMaintenanceRef: &v1.ObjectReference{
						Kind:       "ServerMaintenance",
						Name:       serverMaintenance.Name,
						Namespace:  serverMaintenance.Namespace,
						UID:        serverMaintenance.UID,
						APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
					}}}),
			HaveField("Spec.ServerMaintenanceRefs",
				[]metalv1alpha1.ServerMaintenanceRefItem{{
					ServerMaintenanceRef: &v1.ObjectReference{
						Kind:       "ServerMaintenance",
						Name:       serverMaintenance.Name,
						Namespace:  serverMaintenance.Namespace,
						UID:        serverMaintenance.UID,
						APIVersion: "metal.ironcore.dev/v1alpha1",
					}}}),
		))

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

		ensureBMCVersionConditionTransisition(ctx, acc, bmcVersion)

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

	It("should upgrade servers BMC when server in reserved state", func(ctx SpecContext) {
		// mocked at
		// metal-operator/bmc/redfish_local.go mockedBIOS*
		// note: ImageURI need to have the version string.

		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
		serverClaim := BuildServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOn, "foo:bar")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BMCVersion")
		bmcVersion := &metalv1alpha1.BMCVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCVersionSpec{
				Version:                 upgradeServerBMCVersion,
				Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBMCVersion},
				BMCRef:                  &v1.LocalObjectReference{Name: bmcCRD.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
			},
		}
		Expect(k8sClient.Create(ctx, bmcVersion)).To(Succeed())

		By("Ensuring that the bmcVersion has entered Inprogress state")
		Eventually(Object(bmcVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      fmt.Sprintf("%s-%s", bmcVersion.Name, server.Name),
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by bmcVersion")
		Eventually(Object(bmcVersion)).Should(SatisfyAny(
			HaveField("Spec.ServerMaintenanceRefs",
				[]metalv1alpha1.ServerMaintenanceRefItem{{
					ServerMaintenanceRef: &v1.ObjectReference{
						Kind:       "ServerMaintenance",
						Name:       serverMaintenance.Name,
						Namespace:  serverMaintenance.Namespace,
						UID:        serverMaintenance.UID,
						APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
					}}}),
			HaveField("Spec.ServerMaintenanceRefs",
				[]metalv1alpha1.ServerMaintenanceRefItem{{
					ServerMaintenanceRef: &v1.ObjectReference{
						Kind:       "ServerMaintenance",
						Name:       serverMaintenance.Name,
						Namespace:  serverMaintenance.Namespace,
						UID:        serverMaintenance.UID,
						APIVersion: "metal.ironcore.dev/v1alpha1",
					}}}),
		))

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

		ensureBMCVersionConditionTransisition(ctx, acc, bmcVersion)

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
	})
})

func ensureBMCVersionConditionTransisition(
	ctx context.Context,
	acc *conditionutils.Accessor,
	bmcVersion *metalv1alpha1.BMCVersion,
) {
	By("Ensuring that BMC Conditions have reached expected state 'biosVersionUpgradeIssued'")
	condIssue := &metav1.Condition{}
	Eventually(
		func(g Gomega) int {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
			return len(bmcVersion.Status.Conditions)
		}).Should(BeNumerically(">=", 1))
	Eventually(
		func(g Gomega) bool {
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
	Eventually(
		func(g Gomega) int {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
			return len(bmcVersion.Status.Conditions)
		}).Should(BeNumerically(">=", 2))
	Eventually(
		func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
			g.Expect(acc.FindSlice(bmcVersion.Status.Conditions, bmcVersionUpgradeCompleted, condComplete)).To(BeTrue())
			return condComplete.Status == metav1.ConditionTrue
		}).Should(BeTrue())

	By("Ensuring that BMC Conditions have reached expected state 'biosVersionUpgradeVerficationCondition'")
	verficationComplete := &metav1.Condition{}
	Eventually(
		func(g Gomega) int {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
			return len(bmcVersion.Status.Conditions)
		}).Should(BeNumerically(">=", 4))
	Eventually(
		func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmcVersion.Name}, bmcVersion)).To(Succeed())
			g.Expect(acc.FindSlice(bmcVersion.Status.Conditions, bmcVersionUpgradeVerficationCondition, verficationComplete)).To(BeTrue())
			return verficationComplete.Status == metav1.ConditionTrue
		}).Should(BeTrue())
}
