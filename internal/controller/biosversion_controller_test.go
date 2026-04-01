// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"

	"github.com/ironcore-dev/metal-operator/internal/bmcutils"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BIOSVersion Controller", func() {
	ns := SetupTest(nil)

	var (
		server    *metalv1alpha1.Server
		bmcSecret *metalv1alpha1.BMCSecret
	)
	const upgradeServerBiosVersion string = "P80 v1.45 (12/06/2017)"

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
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
				GenerateName: "test-maintenance-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: MockServerPort,
					},
					Address: MockServerIP,
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Ensuring that the Server is in available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		bmc.UnitTestMockUps.ResetBIOSVersionUpdate()

		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())

		EnsureCleanState()
	})

	It("Should successfully mark completed if no BIOS version change", func(ctx SpecContext) {
		By("Ensuring that the Server has Available state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		)
		By("Creating a BIOSVersion")
		version := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 defaultMockUpServerBiosVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: defaultMockUpServerBiosVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, version)).To(Succeed())

		By("Ensuring that the BIOS upgrade has completed")
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
		)

		By("Ensuring that BIOS upgrade conditions have not been created")
		Consistently(Object(version)).Should(
			HaveField("Status.Conditions", BeNil()),
		)

		By("Ensuring that the Maintenance resource has NOT been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BIOSVersion")
		Expect(k8sClient.Delete(ctx, version)).To(Succeed())

		By("Ensuring that the BIOSVersion has been removed")
		Eventually(Get(version)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(version)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should successfully Start and monitor Upgrade task to completion", func(ctx SpecContext) {
		// mocked at
		// metal-operator/bmc/redfish_local.go mockedBIOS*
		// note: ImageURI need to have the version string.

		By("Ensuring that the Server has Available state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		)
		By("Creating a BIOSVersion")
		version := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 upgradeServerBiosVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, version)).To(Succeed())

		By("Ensuring that the BIOSVersion has entered InProgress state")
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      version.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by the BIOSVersion")
		Eventually(Object(version)).Should(
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Namespace: serverMaintenance.Namespace,
				Name:      serverMaintenance.Name,
			}),
		)

		By("Ensuring that the Server is in Maintenance state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Namespace: serverMaintenance.Namespace,
				Name:      serverMaintenance.Name,
			}),
		))

		By("Ensuring that both BMCVersion and BIOSVersion report consistent InProgress state while waiting on maintenance")
		// This test verifies that both version CRDs use the same state enum value when waiting on maintenance
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		ensureBiosVersionConditionTransition(version, server)

		By("Ensuring that BIOS upgrade has completed")
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
		)

		By("Ensuring that the BIOSVersion has removed Maintenance")
		Eventually(Object(version)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		Consistently(Object(version)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)

		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BIOSVersion")
		Expect(k8sClient.Delete(ctx, version)).To(Succeed())

		By("Ensuring that the BIOSVersion has been removed")
		Eventually(Get(version)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(version)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should preserve conditions during upgrade and through completion", func(ctx SpecContext) {
		By("Ensuring that the Server has Available state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		)

		By("Creating a BIOSVersion that requires an upgrade")
		version := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 upgradeServerBiosVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, version)).To(Succeed())

		By("Ensuring that the BIOSVersion has entered InProgress state")
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      version.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Server is in Maintenance state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Namespace: serverMaintenance.Namespace,
				Name:      serverMaintenance.Name,
			}),
		))

		By("Driving through condition transitions")
		ensureBiosVersionConditionTransition(version, server)

		By("Verifying that conditions are preserved when state transitions to Completed")
		Eventually(Object(version)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
			HaveField("Status.Conditions", Not(BeEmpty())),
		))

		By("Deleting the BIOSVersion")
		Expect(k8sClient.Delete(ctx, version)).To(Succeed())
		Eventually(Get(version)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should upgrade servers BIOS when in reserved state", func(ctx SpecContext) {
		// mocked at
		// metal-operator/bmc/redfish_local.go mockedBIOS*
		// note: ImageURI need to have the version string.

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

		By("Creating a BIOSVersion")
		version := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 upgradeServerBiosVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, version)).To(Succeed())

		By("Ensuring that the BIOSVersion has entered InProgress state")
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      version.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by the BIOSVersion")
		Eventually(Object(version)).Should(
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Namespace: serverMaintenance.Namespace,
				Name:      serverMaintenance.Name,
			}),
		)

		By("Ensuring that the BIOSVersion has InProgress state and is waiting")
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, trueValue)
			metautils.SetLabel(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, trueValue)
		})).Should(Succeed())

		By("Ensuring that the Server is in Maintenance state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Namespace: serverMaintenance.Namespace,
				Name:      serverMaintenance.Name,
			}),
		))

		ensureBiosVersionConditionTransition(version, server)

		By("Ensuring that BIOS upgrade has completed")
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
		)

		By("Ensuring that the BIOSVersion has removed Maintenance")
		Eventually(Object(version)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		Consistently(Object(version)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)

		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BIOSVersion")
		Expect(k8sClient.Delete(ctx, version)).To(Succeed())

		By("Ensuring that the BIOSVersion has been removed")
		Eventually(Get(version)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(version)).Should(Satisfy(apierrors.IsNotFound))

		// cleanup
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateReserved))),
		))
	})

	It("Should allow retry using annotation", func(ctx SpecContext) {
		By("Creating a BIOSVersion")
		version := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 upgradeServerBiosVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, version)).To(Succeed())

		By("Moving to Failed state")
		Eventually(UpdateStatus(version, func() {
			version.Status.State = metalv1alpha1.BIOSVersionStateFailed
		})).Should(Succeed())

		Eventually(Update(version, func() {
			version.Annotations = map[string]string{
				metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationRetryFailed,
			}
		})).Should(Succeed())

		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
		)

		// cleanup
		Expect(k8sClient.Delete(ctx, version)).To(Succeed())
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})
})

var _ = Describe("BIOSVersion Controller with BMCRef BMC", func() {
	ns := SetupTest(nil)

	var (
		server    *metalv1alpha1.Server
		bmcObj    *metalv1alpha1.BMC
		bmcSecret *metalv1alpha1.BMCSecret
	)
	const upgradeServerBiosVersion = "P80 v1.45 (12/06/2017)"

	BeforeEach(func(ctx SpecContext) {
		bmcSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-secret-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		bmcObj = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "23:11:8A:33:CF:EA",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
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

		By("Ensuring that the Server is in available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		bmc.UnitTestMockUps.ResetBIOSVersionUpdate()

		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcObj)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())

		EnsureCleanState()
	})
	It("Should successfully start and monitor upgrade task to completion", func(ctx SpecContext) {
		// mocked at
		// metal-operator/bmc/redfish_local.go mockedBIOS*
		// note: ImageURI need to have the version string.

		By("Ensuring that the Server has Available state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		)
		By("Creating a BIOSVersion")
		version := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version:                 upgradeServerBiosVersion,
					Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, version)).To(Succeed())

		By("Ensuring that the BIOSVersion has entered InProgress state")
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      version.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by the BIOSVersion")
		Eventually(Object(version)).Should(
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Namespace: serverMaintenance.Namespace,
				Name:      serverMaintenance.Name,
			}),
		)

		By("Ensuring that the Server is in Maintenance state")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateMaintenance),
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Namespace: serverMaintenance.Namespace,
				Name:      serverMaintenance.Name,
			}),
		))

		ensureBiosVersionConditionTransition(version, server)

		By("Ensuring that BIOS upgrade has completed")
		Eventually(Object(version)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
		)

		By("Ensuring that the BIOSVersion has removed Maintenance")
		Eventually(Object(version)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)
		Consistently(Object(version)).Should(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		)

		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Deleting the BIOSVersion")
		Expect(k8sClient.Delete(ctx, version)).To(Succeed())

		By("Ensuring that the BIOSVersion has been removed")
		Eventually(Get(version)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(version)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})
})

// conditionMatcher returns a matcher that checks for a specific condition type with ConditionTrue status.
func conditionMatcher(conditionType string) OmegaMatcher {
	return ContainElement(SatisfyAll(
		HaveField("Type", conditionType),
		HaveField("Status", Equal(metav1.ConditionTrue)),
	))
}

// ensureBiosVersionConditionTransition drives the BIOS upgrade through its condition waterfall.
// envtest has no server maintenance controller, so power state transitions must be forced manually.
func ensureBiosVersionConditionTransition(version *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) {
	GinkgoHelper()

	By("Waiting for the upgrade task to be tracked")
	Eventually(func(g Gomega) {
		g.Expect(Get(version)()).To(Succeed())
		g.Expect(version.Status.UpgradeTask).NotTo(BeNil())
		g.Expect(version.Status.UpgradeTask.URI).To(Equal(bmc.DummyMockTaskForUpgrade))
	}).Should(Succeed())

	By("Waiting for the upgrade issued condition")
	Eventually(Object(version)).Should(
		HaveField("Status.Conditions", conditionMatcher(ConditionBIOSUpgradeIssued)),
	)

	By("Waiting for the upgrade task to complete")
	Eventually(Object(version)).Should(
		HaveField("Status.Conditions", conditionMatcher(ConditionBIOSUpgradeCompleted)),
	)

	// envtest has no maintenance controller — force power state transitions manually.
	By("Forcing server power off for reboot")
	Eventually(UpdateStatus(server, func() {
		server.Status.PowerState = metalv1alpha1.ServerOffPowerState
	})).Should(Succeed())

	By("Waiting for the power off condition")
	Eventually(Object(version)).Should(
		HaveField("Status.Conditions", conditionMatcher(ConditionBIOSUpgradePowerOff)),
	)

	By("Forcing server power on for reboot")
	Eventually(UpdateStatus(server, func() {
		server.Status.PowerState = metalv1alpha1.ServerOnPowerState
	})).Should(Succeed())

	By("Waiting for the power on condition")
	Eventually(Object(version)).Should(
		HaveField("Status.Conditions", conditionMatcher(ConditionBIOSUpgradePowerOn)),
	)

	By("Waiting for verification to complete")
	Eventually(Object(version)).Should(
		HaveField("Status.Conditions", conditionMatcher(ConditionBIOSUpgradeVerification)),
	)
}
