// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

var _ = Describe("BMCVersionSet Controller", func() {
	Context("When reconciling a resource", func() {
		ns := SetupTest()

		var bmc01 *metalv1alpha1.BMC
		var bmc02 *metalv1alpha1.BMC
		var bmc03 *metalv1alpha1.BMC
		var server01 *metalv1alpha1.Server
		var server02 *metalv1alpha1.Server
		var server03 *metalv1alpha1.Server
		var upgradeServerBMCVersion string

		BeforeEach(func(ctx SpecContext) {
			upgradeServerBMCVersion = "1.46.455b66-rev4"
			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

			By("Creating a bmc01")
			bmc01 = &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-01-",
					Namespace:    ns.Name,
					Labels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "foo",
					},
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
			Expect(k8sClient.Create(ctx, bmc01)).To(Succeed())

			By("Creating a bmc02")
			bmc02 = &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-02-",
					Namespace:    ns.Name,
					Labels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "bar",
					},
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
			Expect(k8sClient.Create(ctx, bmc02)).To(Succeed())

			By("Creating a bmc03")
			bmc03 = &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-03-",
					Namespace:    ns.Name,
					Labels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "bar",
					},
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
			Expect(k8sClient.Create(ctx, bmc03)).To(Succeed())

			By("Ensuring that the Server01 resource will be created")
			server01 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc01),
				},
			}
			Eventually(Get(server01)).Should(Succeed())

			By("Ensuring that the Server02 resource will be created")
			server02 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc02),
				},
			}
			Eventually(Get(server02)).Should(Succeed())
			By("Transitioning the Servers02 to Available state")
			TransitionServerFromInitialToAvailableState(ctx, k8sClient, server02, ns.Name)

			By("Ensuring that the Server03 resource will be created")
			server03 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc03),
				},
			}
			Eventually(Get(server03)).Should(Succeed())
			By("Transitioning the Servers03 to Available state")
			TransitionServerFromInitialToAvailableState(ctx, k8sClient, server03, ns.Name)
		})

		AfterEach(func(ctx SpecContext) {
			DeleteAllMetalResources(ctx, ns.Name)
			bmc.UnitTestMockUps.ResetBMCVersionUpdate()
		})
		It("should successfully reconcile the resource", func(ctx SpecContext) {
			By("Created resource")
			bmcVersionSet := &metalv1alpha1.BMCVersionSet{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmcversion-set-",
					Namespace:    ns.Name,
				},
				Spec: metalv1alpha1.BMCVersionSetSpec{
					BMCVersionTemplate: metalv1alpha1.BMCVersionTemplate{
						Version:                 upgradeServerBMCVersion,
						Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBMCVersion},
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					},
					BMCSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "bar",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmcVersionSet)).To(Succeed())

			By("Ensuring that the BMCVersion resource has been created")
			var bmcVersionList metalv1alpha1.BMCVersionList
			Eventually(ObjectList(&bmcVersionList)).Should(HaveField("Items", HaveLen(2)))

			By("Checking if the BMCVersion has been created")
			bmcVersion02 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcVersionList.Items[0].Name,
				},
			}
			Eventually(Get(bmcVersion02)).Should(Succeed())

			By("Checking if the 2nd BMCVersion has been created")
			bmcVersion03 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcVersionList.Items[1].Name,
				},
			}
			Eventually(Get(bmcVersion03)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(bmcVersionSet)).Should(SatisfyAll(
				HaveField("Status.FullyLabeledBMCs", BeNumerically("==", 2)),
				HaveField("Status.AvailableBMCVersion", BeNumerically("==", 2)),
				HaveField("Status.FailedBMCVersion", BeNumerically("==", 0)),
			))

			By("Checking the bmcVersion02 have completed")
			Eventually(Object(bmcVersion02)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
				HaveField("Spec.Version", bmcVersionSet.Spec.BMCVersionTemplate.Version),
				HaveField("Spec.Image.URI", bmcVersionSet.Spec.BMCVersionTemplate.Image.URI),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BMCVersionSet",
					Name:               bmcVersionSet.Name,
					UID:                bmcVersionSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking the bmcVersion03 have completed")
			Eventually(Object(bmcVersion03)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
				HaveField("Spec.Version", bmcVersionSet.Spec.BMCVersionTemplate.Version),
				HaveField("Spec.Image.URI", bmcVersionSet.Spec.BMCVersionTemplate.Image.URI),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BMCVersionSet",
					Name:               bmcVersionSet.Name,
					UID:                bmcVersionSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking if the status has been updated")
			Eventually(Object(bmcVersionSet)).Should(SatisfyAll(
				HaveField("Status.FullyLabeledBMCs", BeNumerically("==", 2)),
				HaveField("Status.AvailableBMCVersion", BeNumerically("==", 2)),
				HaveField("Status.CompletedBMCVersion", BeNumerically("==", 2)),
				HaveField("Status.InProgressBMCVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBMCVersion", BeNumerically("==", 0)),
			))

			By("Deleting the resource")
			Expect(k8sClient.Delete(ctx, bmcVersionSet)).To(Succeed())
		})

		It("should successfully reconcile the resource when BMC are deleted/created", func(ctx SpecContext) {
			By("Create resource")
			bmcVersionSet := &metalv1alpha1.BMCVersionSet{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmcsversion-set-",
					Namespace:    ns.Name,
				},
				Spec: metalv1alpha1.BMCVersionSetSpec{
					BMCVersionTemplate: metalv1alpha1.BMCVersionTemplate{
						Version:                 upgradeServerBMCVersion,
						Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBMCVersion},
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					},
					BMCSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "bar",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmcVersionSet)).To(Succeed())

			By("Ensuring that the BMCVersion resource has been created")
			var bmcVersionList metalv1alpha1.BMCVersionList
			Eventually(ObjectList(&bmcVersionList)).Should(HaveField("Items", HaveLen(2)))

			By("Checking if the BMCVersion has been created")
			bmcVersion02 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcVersionList.Items[0].Name,
				},
			}
			Eventually(Get(bmcVersion02)).Should(Succeed())

			By("Checking if the 2nd BMCVersion has been created")
			bmcVersion03 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcVersionList.Items[1].Name,
				},
			}
			Eventually(Get(bmcVersion03)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(bmcVersionSet)).Should(SatisfyAll(
				HaveField("Status.FullyLabeledBMCs", BeNumerically("==", 2)),
				HaveField("Status.AvailableBMCVersion", BeNumerically("==", 2)),
				HaveField("Status.FailedBMCVersion", BeNumerically("==", 0)),
			))

			By("Checking the bmcVersion02 have completed")
			Eventually(Object(bmcVersion02)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BMCVersionSet",
					Name:               bmcVersionSet.Name,
					UID:                bmcVersionSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking the bmcVersion03 have completed")
			Eventually(Object(bmcVersion03)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BMCVersionSet",
					Name:               bmcVersionSet.Name,
					UID:                bmcVersionSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking if the status has been updated")
			Eventually(Object(bmcVersionSet)).Should(SatisfyAll(
				HaveField("Status.FullyLabeledBMCs", BeNumerically("==", 2)),
				HaveField("Status.AvailableBMCVersion", BeNumerically("==", 2)),
				HaveField("Status.CompletedBMCVersion", BeNumerically("==", 2)),
				HaveField("Status.InProgressBMCVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBMCVersion", BeNumerically("==", 0)),
			))

			BMCToDelete := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcVersion02.Spec.BMCRef.Name,
				},
			}

			By(fmt.Sprintf("Deleting one of the BMC %v", BMCToDelete.Name))
			Eventually(Get(BMCToDelete)).To(Succeed())
			Expect(k8sClient.Delete(ctx, BMCToDelete)).To(Succeed())

			By("Checking if the BMCVersion have been deleted")
			Eventually(Get(bmcVersion02)).ShouldNot(Succeed())
			Eventually(Get(bmcVersion03)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(bmcVersionSet)).Should(SatisfyAll(
				HaveField("Status.FullyLabeledBMCs", BeNumerically("==", 1)),
				HaveField("Status.AvailableBMCVersion", BeNumerically("==", 1)),
				HaveField("Status.CompletedBMCVersion", BeNumerically("==", 1)),
				HaveField("Status.InProgressBMCVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBMCVersion", BeNumerically("==", 0)),
			))

			By("creating the deleted BMC")
			BMCToDelete.ResourceVersion = ""
			BMCToDelete.Spec.BMCSettingRef = nil
			BMCToDelete.Spec.Endpoint = bmc02.Spec.Endpoint
			Expect(k8sClient.Create(ctx, BMCToDelete)).Should(Succeed())

			By("Ensuring that the BMCVersion resource has been created")
			Eventually(ObjectList(&bmcVersionList)).Should(HaveField("Items", HaveLen(2)))

			By("Checking if the BMCVersion has been created")
			bmcVersion02 = &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcVersionList.Items[0].Name,
				},
			}
			Eventually(Get(bmcVersion02)).Should(Succeed())

			By("Checking if the 2nd BMCVersion has been created")
			bmcVersion03 = &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcVersionList.Items[1].Name,
				},
			}
			Eventually(Get(bmcVersion03)).Should(Succeed())

			By("Checking the bmcVersion02 have completed")
			Eventually(Object(bmcVersion02)).Should(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
			)

			By("Checking if the status has been updated")
			Eventually(Object(bmcVersionSet)).Should(SatisfyAll(
				HaveField("Status.FullyLabeledBMCs", BeNumerically("==", 2)),
				HaveField("Status.AvailableBMCVersion", BeNumerically("==", 2)),
				HaveField("Status.CompletedBMCVersion", BeNumerically("==", 2)),
				HaveField("Status.InProgressBMCVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBMCVersion", BeNumerically("==", 0)),
			))

			By("Updating the label of BMC01")
			Eventually(Update(bmc01, func() {
				bmc01.Labels = map[string]string{
					"metal.ironcore.dev/Manufacturer": "bar",
				}
			})).Should(Succeed())

			By("Ensuring that the BMCVersion resource has been created")
			Eventually(ObjectList(&bmcVersionList)).Should(HaveField("Items", HaveLen(3)))

			By("Checking if the 3rd BMCVersion has been created")
			bmcVersion01 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcVersionList.Items[2].Name,
				},
			}
			Eventually(Get(bmcVersion01)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(bmcVersionSet)).Should(SatisfyAll(
				HaveField("Status.FullyLabeledBMCs", BeNumerically("==", 3)),
				HaveField("Status.AvailableBMCVersion", BeNumerically("==", 3)),
				HaveField("Status.FailedBMCVersion", BeNumerically("==", 0)),
			))

			By("Checking the bmcVersion01 have completed")
			Eventually(Object(bmcVersion01)).Should(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
			)

			By("Checking if the status has been updated")
			Eventually(Object(bmcVersionSet)).Should(SatisfyAll(
				HaveField("Status.FullyLabeledBMCs", BeNumerically("==", 3)),
				HaveField("Status.AvailableBMCVersion", BeNumerically("==", 3)),
				HaveField("Status.CompletedBMCVersion", BeNumerically("==", 3)),
				HaveField("Status.InProgressBMCVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBMCVersion", BeNumerically("==", 0)),
			))
		})

		It("Should correctly handle ServerMaintenanceRefs merging for existing and new BMCVersion", func(ctx SpecContext) {
			By("Creating ServerClaims03 and transitioning the Server03 to Reserved state")
			serverClaim03 := CreateServerClaim(ctx, k8sClient, *server03, ns.Name, nil, metalv1alpha1.PowerOff, "foo:bar")
			TransitionServerToReservedState(ctx, k8sClient, serverClaim03, server03, ns.Name)
			Eventually(Object(server03)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

			By("Creating ServerClaims02 and transitioning the Server02 to Reserved state")
			serverClaim02 := CreateServerClaim(ctx, k8sClient, *server02, ns.Name, nil, metalv1alpha1.PowerOff, "foo:bar")
			TransitionServerToReservedState(ctx, k8sClient, serverClaim02, server02, ns.Name)
			Eventually(Object(server02)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

			By("Creating 1 manual ServerMaintenance objects with OwnerApproval for server03")
			serverMaintenance03 := &metalv1alpha1.ServerMaintenance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    ns.Name,
					GenerateName: "manual-maintenance-03-",
				},
				Spec: metalv1alpha1.ServerMaintenanceSpec{
					ServerRef: &v1.LocalObjectReference{Name: server03.Name},
					Policy:    metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				},
			}
			Expect(k8sClient.Create(ctx, serverMaintenance03)).To(Succeed())

			By("Creating template refs for the manual ServerMaintenance objects")
			templateRef03 := metalv1alpha1.ServerMaintenanceRefItem{
				ServerMaintenanceRef: &v1.ObjectReference{
					Kind:       "ServerMaintenance",
					Name:       serverMaintenance03.Name,
					Namespace:  serverMaintenance03.Namespace,
					UID:        serverMaintenance03.UID,
					APIVersion: metalv1alpha1.GroupVersion.String(),
				},
			}

			By("Creating BMCVersionSet with the manual ServerMaintenance ref")
			bmcVersionSet := &metalv1alpha1.BMCVersionSet{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmcsversion-set-",
					Namespace:    ns.Name,
				},
				Spec: metalv1alpha1.BMCVersionSetSpec{
					BMCVersionTemplate: metalv1alpha1.BMCVersionTemplate{
						Version:                 upgradeServerBMCVersion,
						Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBMCVersion},
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
						ServerMaintenanceRefs: []metalv1alpha1.ServerMaintenanceRefItem{
							templateRef03,
						},
					},
					BMCSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "bar",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmcVersionSet)).To(Succeed())

			By("Ensuring that the BMCVersion resource has been created")
			var bmcVersionList metalv1alpha1.BMCVersionList
			Eventually(ObjectList(&bmcVersionList)).Should(HaveField("Items", HaveLen(2)))

			// get the server01 BMCVersion
			var BMC03VersionName string
			var BMC02VersionName string
			for _, bmcversion := range bmcVersionList.Items {
				if bmcversion.Spec.BMCRef.Name == bmc03.Name {
					BMC03VersionName = bmcversion.Name
				}
				if bmcversion.Spec.BMCRef.Name == bmc02.Name {
					BMC02VersionName = bmcversion.Name
				}
			}
			Expect(BMC03VersionName).ToNot(BeEmpty(), "Failed to find BMCVersion for server03's BMC")
			Expect(BMC02VersionName).ToNot(BeEmpty(), "Failed to find BMCVersion for server02's BMC")

			By("Checking if the bmcSettings for bmc01 and bmc02 was generated with correct ServerMaintenanceRefs")
			bmcVersion03 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: BMC03VersionName,
				},
			}
			Eventually(Get(bmcVersion03)).Should(Succeed())

			Eventually(Object(bmcVersion03)).Should(SatisfyAll(
				HaveField("Spec.BMCVersionTemplate.ServerMaintenanceRefs", ContainElement(templateRef03)),
				HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
			), fmt.Sprintf("BMCVersion %v should contain the ServerMaintenanceRef03", bmcVersion03.Spec))
			bmcVersion02 := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: BMC02VersionName,
				},
			}
			Eventually(Get(bmcVersion02)).Should(Succeed())

			Eventually(Object(bmcVersion02)).Should(SatisfyAll(
				HaveField("Spec.BMCVersionTemplate.ServerMaintenanceRefs", Not(ContainElement(templateRef03))),
				HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
			))

			// modifying the BMCVersionSet to trigger Patching of a BMCVersionTemplate for both BMCs
			// but the TemplateRef01 should only be added to the new BMCVersion for bmc01
			By("Modifying the BMCVersionSet to trigger creation of new BMCVersions")
			Eventually(Update(bmcVersionSet, func() {
				bmcVersionSet.Spec.BMCVersionTemplate.UpdatePolicy = ptr.To(metalv1alpha1.UpdatePolicyForce)
			})).To(Succeed())

			Eventually(Object(bmcVersion03)).Should(SatisfyAll(
				HaveField("Spec.BMCVersionTemplate.ServerMaintenanceRefs", ContainElement(templateRef03)),
				HaveField("Spec.BMCVersionTemplate.UpdatePolicy", Equal(ptr.To(metalv1alpha1.UpdatePolicyForce))),
			))

			Eventually(Object(bmcVersion02)).Should(SatisfyAll(
				HaveField("Spec.BMCVersionTemplate.ServerMaintenanceRefs", Not(ContainElement(templateRef03))),
				HaveField("Spec.BMCVersionTemplate.UpdatePolicy", Equal(ptr.To(metalv1alpha1.UpdatePolicyForce))),
			))

			By("Approving the maintenance")
			Eventually(Update(serverClaim03, func() {
				metautils.SetAnnotation(serverClaim03, metalv1alpha1.ServerMaintenanceApprovalKey, "true")
			})).Should(Succeed())

			Eventually(Object(bmcVersion03)).Should(SatisfyAny(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
			))

			Eventually(Object(bmcVersion03)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
			))
			Eventually(Update(serverClaim02, func() {
				metautils.SetAnnotation(serverClaim02, metalv1alpha1.ServerMaintenanceApprovalKey, "true")
			})).Should(Succeed())

			Eventually(Object(bmcVersion02)).Should(SatisfyAny(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateInProgress),
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
			))

			Eventually(Object(bmcVersion02)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BMCVersionStateCompleted),
			))

			By("Deleting the BMCVersion")
			Expect(k8sClient.Delete(ctx, bmcVersion03)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bmcVersion02)).To(Succeed())
			By("Deleting the BMCVersionSet")
			Expect(k8sClient.Delete(ctx, bmcVersionSet)).To(Succeed())
		})
	})
})
