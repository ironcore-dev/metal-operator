// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/ironcore-dev/controller-utils/conditionutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BIOSVersion Controller", func() {
	ns := SetupTest()

	var (
		server                   *metalv1alpha1.Server
		upgradeServerBiosVersion string
	)

	BeforeEach(func(ctx SpecContext) {
		upgradeServerBiosVersion = "P80 v1.45 (12/06/2017)"
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a Server")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-maintenance-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Address: "127.0.0.1",
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		TransistionServerFromInitialToAvailableState(ctx, k8sClient, server, ns.Name)
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
		bmc.UnitTestMockUps.ResetBIOSVersionUpdate()
	})

	It("should successfully mark completed if no BIOS version change", func(ctx SpecContext) {

		By("Ensuring that the server has Available state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		)
		By("Creating a BIOSVersion")
		biosVersion := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				VersionUpdateSpec: metalv1alpha1.VersionUpdateSpec{
					Version: defaultMockUpServerBiosVersion,
					Image:   metalv1alpha1.ImageSpec{URI: defaultMockUpServerBiosVersion},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosVersion)).To(Succeed())

		By("Ensuring that BIOS upgrade has completed")
		Eventually(Object(biosVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
		)

		By("Ensuring that BIOS upgrade Conditions has not created")
		Consistently(Object(biosVersion)).Should(
			HaveField("Status.Conditions", BeNil()),
		)

		By("Ensuring that the Maintenance resource has NOT been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BIOSVersion")
		Expect(k8sClient.Delete(ctx, biosVersion)).To(Succeed())

		By("Ensuring that the BiosVersion has been removed")
		Eventually(Get(biosVersion)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(biosVersion)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should successfully Start and monitor Upgrade task to completion", func(ctx SpecContext) {
		// mocked at
		// metal-operator/bmc/redfish_local.go mockedBIOS*
		// note: ImageURI need to have the version string.

		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})

		By("Ensuring that the server has Available state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		)
		By("Creating a BIOSVersion")
		biosVersion := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				VersionUpdateSpec: metalv1alpha1.VersionUpdateSpec{
					Version: upgradeServerBiosVersion,
					Image:   metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosVersion)).To(Succeed())

		By("Ensuring that the biosVersion has entered Inprogress state")
		Eventually(Object(biosVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      biosVersion.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by biosVersion")
		Eventually(Object(biosVersion)).Should(SatisfyAny(
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			}),
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
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

		ensureBiosVersionConditionTransisition(acc, biosVersion, server)

		By("Ensuring that BIOS upgrade has completed")
		Eventually(Object(biosVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
		)

		By("Ensuring that BIOSVersion has removed Maintenance")
		Eventually(Object(biosVersion)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		Consistently(Object(biosVersion)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)

		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BIOSVersion")
		Expect(k8sClient.Delete(ctx, biosVersion)).To(Succeed())

		By("Ensuring that the BiosVersion has been removed")
		Eventually(Get(biosVersion)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(biosVersion)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should upgrade servers BIOS when in reserved state", func(ctx SpecContext) {
		// mocked at
		// metal-operator/bmc/redfish_local.go mockedBIOS*
		// note: ImageURI need to have the version string.

		acc := conditionutils.NewAccessor(conditionutils.AccessorOptions{})
		serverClaim := BuildServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOn, "foo:bar")
		TransistionServerToReserveredState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BIOSVersion")
		biosVersion := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				VersionUpdateSpec: metalv1alpha1.VersionUpdateSpec{
					Version: upgradeServerBiosVersion,
					Image:   metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
				},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosVersion)).To(Succeed())

		By("Ensuring that the biosVersion has entered Inprogress state")
		Eventually(Object(biosVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      biosVersion.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by biosVersion")
		Eventually(Object(biosVersion)).Should(SatisfyAny(
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			}),
			HaveField("Spec.ServerMaintenanceRef", &v1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
		))

		By("Ensuring that the biosVersion has Inprogress state and waiting")
		Eventually(Object(biosVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
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

		ensureBiosVersionConditionTransisition(acc, biosVersion, server)

		By("Ensuring that BIOS upgrade has completed")
		Eventually(Object(biosVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
		)

		By("Ensuring that BIOSVersion has removed Maintenance")
		Eventually(Object(biosVersion)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		Consistently(Object(biosVersion)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)

		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BIOSVersion")
		Expect(k8sClient.Delete(ctx, biosVersion)).To(Succeed())

		By("Ensuring that the BiosVersion has been removed")
		Eventually(Get(biosVersion)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(biosVersion)).Should(Satisfy(apierrors.IsNotFound))
	})
})

func ensureBiosVersionConditionTransisition(
	acc *conditionutils.Accessor,
	biosVersion *metalv1alpha1.BIOSVersion,
	server *metalv1alpha1.Server,
) {
	By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeIssued'")
	condIssue := &metav1.Condition{}
	Eventually(
		func(g Gomega) int {
			g.Expect(Get(biosVersion)()).To(Succeed())
			return len(biosVersion.Status.Conditions)
		}).Should(BeNumerically(">=", 1))
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosVersion)()).To(Succeed())
			g.Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeIssued, condIssue)).To(BeTrue())
			return condIssue.Status == metav1.ConditionTrue
		}).Should(BeTrue())

	By("Ensuring that BIOSVersion has updated the taskStatus with taskURI")
	Eventually(Object(biosVersion)).Should(
		HaveField("Status.UpgradeTask.URI", "dummyTask"),
	)

	By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeCompleted'")
	condComplete := &metav1.Condition{}
	Eventually(
		func(g Gomega) int {
			g.Expect(Get(biosVersion)()).To(Succeed())
			return len(biosVersion.Status.Conditions)
		}).Should(BeNumerically(">=", 2))
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosVersion)()).To(Succeed())
			g.Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeCompleted, condComplete)).To(BeTrue())
			return condComplete.Status == metav1.ConditionTrue
		}).Should(BeTrue())

	// waiting for serverMaintenance and server to eventually update the power state is making it flaky.
	// force turn on the server already for testing
	By("update the server state to PoweredOff state")
	Eventually(UpdateStatus(server, func() {
		server.Status.PowerState = metalv1alpha1.ServerOffPowerState
	})).Should(Succeed())

	By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeRebootServerPoweroff'")
	rebootStart := &metav1.Condition{}
	Eventually(
		func(g Gomega) int {
			g.Expect(Get(biosVersion)()).To(Succeed())
			return len(biosVersion.Status.Conditions)
		}).Should(BeNumerically(">=", 3))
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosVersion)()).To(Succeed())
			g.Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeCompleted, rebootStart)).To(BeTrue())
			return rebootStart.Status == metav1.ConditionTrue
		}).Should(BeTrue())

	// waiting for serverMaintenance and server to eventually update the power state is making it flaky.
	// force turn on the server already for testing
	By("update the server state to PoweredOn state")
	Eventually(UpdateStatus(server, func() {
		server.Status.PowerState = metalv1alpha1.ServerOnPowerState
	})).Should(Succeed())

	By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeRebootServerPowerOn'")
	rebootComplete := &metav1.Condition{}
	Eventually(
		func(g Gomega) int {
			g.Expect(Get(biosVersion)()).To(Succeed())
			return len(biosVersion.Status.Conditions)
		}).Should(BeNumerically(">=", 4))
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosVersion)()).To(Succeed())
			g.Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeCompleted, rebootComplete)).To(BeTrue())
			return rebootComplete.Status == metav1.ConditionTrue
		}).Should(BeTrue())

	By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeVerficationCondition'")
	verficationComplete := &metav1.Condition{}
	Eventually(
		func(g Gomega) int {
			g.Expect(Get(biosVersion)()).To(Succeed())
			return len(biosVersion.Status.Conditions)
		}).Should(BeNumerically(">=", 5))
	Eventually(
		func(g Gomega) bool {
			g.Expect(Get(biosVersion)()).To(Succeed())
			g.Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeVerficationCondition, verficationComplete)).To(BeTrue())
			return verficationComplete.Status == metav1.ConditionTrue
		}).Should(BeTrue())

}
