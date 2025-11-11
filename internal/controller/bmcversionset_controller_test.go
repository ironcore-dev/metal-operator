// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
)

var _ = Describe("BMCVersionSet Controller", func() {
	var redfishMockServerAddr = []string{":8000", ":8001", ":8002"}
	var redfishMockServerPorts = []int{8000, 8001, 8002}
	ns := SetupTest(redfishMockServerAddr)

	var (
		bmc01                   *metalv1alpha1.BMC
		bmc02                   *metalv1alpha1.BMC
		bmc03                   *metalv1alpha1.BMC
		bmcSecret               *metalv1alpha1.BMCSecret
		upgradeServerBMCVersion string
	)

	BeforeEach(func(ctx SpecContext) {
		upgradeServerBMCVersion = "1.46.455b66-rev4"
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
					Port: int32(redfishMockServerPorts[0]),
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
					Port: int32(redfishMockServerPorts[1]),
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
					Port: int32(redfishMockServerPorts[2]),
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc03)).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		bmc.UnitTestMockUps.ResetBMCVersionUpdate()

		maintenanceList := &metalv1alpha1.ServerMaintenanceList{}
		Eventually(List(maintenanceList)).Should(Succeed())
		for _, maintenance := range maintenanceList.Items {
			Expect(k8sClient.Delete(ctx, &maintenance)).To(Succeed())
		}

		server01 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc01),
			},
		}
		Eventually(UpdateStatus(server01, func() {
			server01.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmc01)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server01)).To(Succeed())
		Eventually(Get(server01)).Should(Satisfy(apierrors.IsNotFound))

		server02 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc02),
			},
		}
		Eventually(UpdateStatus(server02, func() {
			server02.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmc02)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server02)).To(Succeed())
		Eventually(Get(server02)).Should(Satisfy(apierrors.IsNotFound))

		server03 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc03),
			},
		}
		Eventually(UpdateStatus(server03, func() {
			server03.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())
		Expect(k8sClient.Delete(ctx, bmc03)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server03)).To(Succeed())
		Eventually(Get(server03)).Should(Satisfy(apierrors.IsNotFound))

		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		EnsureCleanState()
	})

	It("Should successfully reconcile the resource", func(ctx SpecContext) {
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
		bmcVersionList := &metalv1alpha1.BMCVersionList{}
		Eventually(ObjectList(bmcVersionList)).Should(HaveField("Items", HaveLen(2)))

		By("Ensuring that the BMCVersion has been created")
		bmcVersion01 := &metalv1alpha1.BMCVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcVersionList.Items[0].Name,
			},
		}
		Eventually(Get(bmcVersion01)).Should(Succeed())

		By("Ensuring that the BMCVersion has been created")
		bmcVersion02 := &metalv1alpha1.BMCVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcVersionList.Items[1].Name,
			},
		}
		Eventually(Get(bmcVersion02)).Should(Succeed())

		By("Checking if the status has been updated")
		Eventually(Object(bmcVersionSet)).Should(SatisfyAll(
			HaveField("Status.FullyLabeledBMCs", BeNumerically("==", 2)),
			HaveField("Status.AvailableBMCVersion", BeNumerically("==", 2)),
			HaveField("Status.FailedBMCVersion", BeNumerically("==", 0)),
		))

		By("Checking the bmcVersion01 have completed")
		Eventually(Object(bmcVersion01)).Should(SatisfyAll(
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

		// cleanup
		Expect(k8sClient.Delete(ctx, bmcVersion01)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcVersion02)).To(Succeed())
	})

	It("Should successfully reconcile the resource when BMC are deleted/created", func(ctx SpecContext) {
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

		// cleanup
		Expect(k8sClient.Delete(ctx, bmcVersionSet)).Should(Succeed())
		Eventually(ObjectList(&bmcVersionList)).Should(HaveField("Items", HaveLen(3)))
		for i := range bmcVersionList.Items {
			bmcVersion := &metalv1alpha1.BMCVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: bmcVersionList.Items[i].Name,
				},
			}
			Expect(k8sClient.Delete(ctx, bmcVersion)).To(Succeed())
		}
	})
})
