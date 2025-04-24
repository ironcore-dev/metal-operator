// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/ironcore-dev/controller-utils/conditionutils"
	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BIOSVersion Controller", func() {
	ns := SetupTest()
	ns.Name = "default"

	var server *metalv1alpha1.Server

	BeforeEach(func(ctx SpecContext) {
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
				UUID:       "38947555-7742-3448-3784-823347823834",
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
		Expect(k8sClient.Create(ctx, server)).Should(Succeed())

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
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
				BIOSVersionSpec: metalv1alpha1.VersionSpec{
					Version: "123.5",
					Image:   metalv1alpha1.ImageSpec{URI: "123.5"},
				},
				ServerRef:                   &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
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

	It("should mark Failed if BIOS version is lower", func(ctx SpecContext) {

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
				BIOSVersionSpec: metalv1alpha1.VersionSpec{
					Version: "123.4",
					Image:   metalv1alpha1.ImageSpec{URI: "123.5"},
				},
				ServerRef:                   &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, biosVersion)).To(Succeed())

		By("Ensuring that BIOS upgrade has completed")
		Eventually(Object(biosVersion)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSVersionStateFailed),
		)

		By("Ensuring that BIOS upgrade Conditions has not created")
		Consistently(Object(biosVersion)).Should(SatisfyAny(
			HaveField("Status.Conditions", BeNil()),
		))

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
				BIOSVersionSpec: metalv1alpha1.VersionSpec{
					Version: "123.7",
					Image:   metalv1alpha1.ImageSpec{URI: "123.7"},
				},
				ServerRef:                   &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
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

		By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeIssued'")
		condIssue := &metav1.Condition{}
		Eventually(
			func() int {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: biosVersion.Name}, biosVersion)).To(Succeed())
				return len(biosVersion.Status.Conditions)
			}).Should(BeNumerically(">=", 1))
		Eventually(
			func() bool {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: biosVersion.Name}, biosVersion)).To(Succeed())
				Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeIssued, condIssue)).To(BeTrue())
				return condIssue.Status == metav1.ConditionTrue
			}).Should(BeTrue())

		By("Ensuring that BIOSVersion has updated the taskStatus with taskURI")
		Eventually(Object(biosVersion)).Should(
			HaveField("Status.UpgradeTaskStatus.TaskURI", "dummyTask"),
		)

		By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeCompleted'")
		condComplete := &metav1.Condition{}
		Eventually(
			func() int {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: biosVersion.Name}, biosVersion)).To(Succeed())
				return len(biosVersion.Status.Conditions)
			}).Should(BeNumerically(">=", 2))
		Eventually(
			func() bool {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: biosVersion.Name}, biosVersion)).To(Succeed())
				Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeCompleted, condComplete)).To(BeTrue())
				return condComplete.Status == metav1.ConditionTrue
			}).Should(BeTrue())

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

		serverClaim := transitionServerToReserved(ctx, ns, server, metalv1alpha1.PowerOn)

		By("Creating a BIOSVersion")
		biosVersion := &metalv1alpha1.BIOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BIOSVersionSpec{
				BIOSVersionSpec: metalv1alpha1.VersionSpec{
					Version: "123.8",
					Image:   metalv1alpha1.ImageSpec{URI: "123.8"},
				},
				ServerRef:                   &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicyType: metalv1alpha1.ServerMaintenancePolicyEnforced,
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

		By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeIssued'")
		condIssue := &metav1.Condition{}
		Eventually(
			func() int {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: biosVersion.Name}, biosVersion)).To(Succeed())
				return len(biosVersion.Status.Conditions)
			}).Should(BeNumerically(">=", 1))
		Eventually(
			func() bool {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: biosVersion.Name}, biosVersion)).To(Succeed())
				Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeIssued, condIssue)).To(BeTrue())
				return condIssue.Status == metav1.ConditionTrue
			}).Should(BeTrue())

		By("Ensuring that BIOSVersion has updated the taskStatus with taskURI")
		Eventually(Object(biosVersion)).Should(
			HaveField("Status.UpgradeTaskStatus.TaskURI", "dummyTask"),
		)

		By("Ensuring that BIOS Conditions have reached expected state 'biosVersionUpgradeCompleted'")
		condComplete := &metav1.Condition{}
		Eventually(
			func() int {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: biosVersion.Name}, biosVersion)).To(Succeed())
				return len(biosVersion.Status.Conditions)
			}).Should(BeNumerically(">=", 2))
		Eventually(
			func() bool {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: biosVersion.Name}, biosVersion)).To(Succeed())
				Expect(acc.FindSlice(biosVersion.Status.Conditions, biosVersionUpgradeCompleted, condComplete)).To(BeTrue())
				return condComplete.Status == metav1.ConditionTrue
			}).Should(BeTrue())

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

// func transitionServerToReserved(ctx SpecContext, ns *v1.Namespace, server *metalv1alpha1.Server, powerState metalv1alpha1.Power) *metalv1alpha1.ServerClaim {

// 	By("Creating an Ignition secret")
// 	ignitionSecret := &v1.Secret{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Namespace:    ns.Name,
// 			GenerateName: "test-",
// 		},
// 		Data: map[string][]byte{
// 			"foo": []byte("bar"),
// 		},
// 	}
// 	Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

// 	By("Creating a ServerClaim")
// 	serverClaim := &metalv1alpha1.ServerClaim{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Namespace:    ns.Name,
// 			GenerateName: "test-",
// 		},
// 		Spec: metalv1alpha1.ServerClaimSpec{
// 			Power:             powerState,
// 			ServerRef:         &v1.LocalObjectReference{Name: server.Name},
// 			IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
// 			Image:             "foo:bar",
// 		},
// 	}
// 	Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())

// 	By("Patching the Server to available state")
// 	Eventually(UpdateStatus(server, func() {
// 		server.Status.State = metalv1alpha1.ServerStateAvailable
// 	})).Should(Succeed())

// 	// unfortunately, ServerClaim force creates the bootconfig and that does not transition to completed state.
// 	// in reserved state, Hence, manually move bootconfig to completed to be able to put server in powerOn state.
// 	bootConfig := &metalv1alpha1.ServerBootConfiguration{}
// 	bootConfig.Name = serverClaim.Name
// 	bootConfig.Namespace = serverClaim.Namespace

// 	Eventually(Get(bootConfig)).Should(Succeed())

// 	By("Patching the Server to available state")
// 	Eventually(UpdateStatus(bootConfig, func() {
// 		bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
// 	})).Should(Succeed())

// 	Eventually(Get(server)).Should(Succeed())

// 	By("Ensuring that the Server has the spec and state")
// 	Eventually(Object(server)).Should(SatisfyAll(
// 		HaveField("Spec.ServerClaimRef.Name", serverClaim.Name),
// 		HaveField("Spec.Power", powerState),
// 		HaveField("Status.State", metalv1alpha1.ServerStateReserved),
// 	))
// 	By("Ensuring that the Server has the correct power state")
// 	Eventually(Object(server)).Should(SatisfyAll(
// 		HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(powerState)),
// 	))

// 	return serverClaim
// }
