// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	"github.com/ironcore-dev/controller-utils/conditionutils"
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
				GenerateName: "test-bios-version-",
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
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.SystemURI", Not(BeEmpty())),
			HaveField("Status.Manufacturer", Not(BeEmpty())),
		))

		By("Ensuring that the Server is in available state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", Equal(metalv1alpha1.ServerStateDiscovery)),
		)
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
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version: defaultMockUpServerBiosVersion,
					Image: metalv1alpha1.ImageSpec{
						URI: fmt.Sprintf(
							"{\"updatedVersion\": \"%s\", \"ResourceURI\": \"%s\", \"Module\": \"BIOS\"}",
							defaultMockUpServerBiosVersion, server.Spec.SystemURI),
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
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
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should successfully Start and monitor Upgrade task to completion", func(ctx SpecContext) {
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
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version: upgradeServerBiosVersion,
					Image: metalv1alpha1.ImageSpec{
						// create a fake json string as expected by the mock server
						URI: fmt.Sprintf(
							"{\"updatedVersion\": \"%s\", \"ResourceURI\": \"%s\", \"Module\": \"BIOS\"}",
							upgradeServerBiosVersion, server.Spec.SystemURI),
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			}),
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
		))

		ensureBiosVersionConditionTransition(acc, biosVersion, server)

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
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should upgrade servers BIOS when in reserved state", func(ctx SpecContext) {
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

		By("Creating a BIOSVersion")
		biosVersion := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version: upgradeServerBiosVersion,
					Image: metalv1alpha1.ImageSpec{
						URI: fmt.Sprintf(
							"{\"updatedVersion\": \"%s\", \"ResourceURI\": \"%s\", \"Module\": \"BIOS\"}",
							upgradeServerBiosVersion, server.Spec.SystemURI),
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			}),
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
		))

		ensureBiosVersionConditionTransition(acc, biosVersion, server)

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

		// cleanup
		Expect(k8sClient.Delete(ctx, serverClaim)).To(Succeed())
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateReserved))),
		))
	})

	It("Should allow retry using annotation", func(ctx SpecContext) {
		By("Creating a BIOSVersion")
		biosVersion := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version: upgradeServerBiosVersion,
					Image: metalv1alpha1.ImageSpec{
						URI: fmt.Sprintf(
							"{\"updatedVersion\": \"%s\", \"ResourceURI\": \"%s\", \"Module\": \"BIOS\"}",
							upgradeServerBiosVersion, server.Spec.SystemURI),
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, biosVersion)).To(Succeed())

		By("Moving to Failed state")
		Eventually(UpdateStatus(biosVersion, func() {
			biosVersion.Status.State = metalv1alpha1.BIOSVersionStateFailed
		})).Should(Succeed())

		Eventually(Update(biosVersion, func() {
			biosVersion.Annotations = map[string]string{
				metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationRetryFailed,
			}
		})).Should(Succeed())

		Eventually(Object(biosVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateInProgress),
		)

		Eventually(Object(biosVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
		)

		// cleanup
		Expect(k8sClient.Delete(ctx, biosVersion)).To(Succeed())
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
				Namespace:    ns.Name,
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
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.SystemURI", Not(BeEmpty())),
			HaveField("Status.Manufacturer", Not(BeEmpty())),
		))

		By("Ensuring that the Server is in available state")
		Eventually(Object(server)).Should(
			HaveField("Status.State", Equal(metalv1alpha1.ServerStateDiscovery)),
		)
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

	})

	AfterEach(func(ctx SpecContext) {
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcObj)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())

		EnsureCleanState()
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
				BIOSVersionTemplate: metalv1alpha1.BIOSVersionTemplate{
					Version: upgradeServerBiosVersion,
					Image: metalv1alpha1.ImageSpec{
						URI: fmt.Sprintf(
							"{\"updatedVersion\": \"%s\", \"ResourceURI\": \"%s\", \"Module\": \"BIOS\"}",
							upgradeServerBiosVersion, server.Spec.SystemURI),
					},
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
			}),
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
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
			HaveField("Spec.ServerMaintenanceRef", &metalv1alpha1.ObjectReference{
				Kind:       "ServerMaintenance",
				Name:       serverMaintenance.Name,
				Namespace:  serverMaintenance.Namespace,
				UID:        serverMaintenance.UID,
				APIVersion: "metal.ironcore.dev/v1alpha1",
			}),
		))

		ensureBiosVersionConditionTransition(acc, biosVersion, server)

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
		Eventually(Object(server)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})
})

func ensureBiosVersionConditionTransition(acc *conditionutils.Accessor, biosVersion *metalv1alpha1.BIOSVersion, server *metalv1alpha1.Server) {
	GinkgoHelper()
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
		HaveField("Status.UpgradeTask.URI", "/redfish/v1/TaskService/Tasks/dummyBIOSTask"), // from BIOSVersion Update task from mock server
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
	Eventually(func(g Gomega) int {
		g.Expect(Get(biosVersion)()).To(Succeed())
		return len(biosVersion.Status.Conditions)
	}).Should(BeNumerically(">=", 4))
	Eventually(func(g Gomega) bool {
		g.Expect(Get(biosVersion)()).To(Succeed())
		g.Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeCompleted, rebootComplete)).To(BeTrue())
		return rebootComplete.Status == metav1.ConditionTrue
	}).Should(BeTrue())

	By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeVerficationCondition'")
	verificationComplete := &metav1.Condition{}
	Eventually(func(g Gomega) int {
		g.Expect(Get(biosVersion)()).To(Succeed())
		return len(biosVersion.Status.Conditions)
	}).Should(BeNumerically(">=", 5))
	Eventually(func(g Gomega) bool {
		g.Expect(Get(biosVersion)()).To(Succeed())
		g.Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeVerficationCondition, verificationComplete)).To(BeTrue())
		return verificationComplete.Status == metav1.ConditionTrue
	}).Should(BeTrue())
}
