// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("BIOSVersionSet Controller", func() {
	Context("When reconciling a resource", func() {
		ns := SetupTest()

		var server01 *metalv1alpha1.Server
		var server02 *metalv1alpha1.Server
		var server03 *metalv1alpha1.Server
		var upgradeServerBiosVersion string

		BeforeEach(func(ctx SpecContext) {
			upgradeServerBiosVersion = "P80 v1.45 (12/06/2017)"
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

			By("Creating a Server01")
			server01 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-server01-",
					Labels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "foo",
					},
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
			Expect(k8sClient.Create(ctx, server01)).Should(Succeed())

			By("Creating a second Server02")
			server02 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-server02-",
					Labels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "bar",
					},
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
			Expect(k8sClient.Create(ctx, server02)).Should(Succeed())

			By("Creating a third Server03")
			server03 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-server03-",
					Labels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "bar",
					},
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
			Expect(k8sClient.Create(ctx, server03)).Should(Succeed())
		})

		AfterEach(func(ctx SpecContext) {
			DeleteAllMetalResources(ctx, ns.Name)
		})

		It("should successfully reconcile the resource", func(ctx SpecContext) {
			By("Created resource")
			biosVersionSet := &metalv1alpha1.BIOSVersionSet{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-biosversion-set-",
					Namespace:    ns.Name,
				},
				Spec: metalv1alpha1.BIOSVersionSetSpec{
					BiosVersionTemplate: metalv1alpha1.VersionUpdateSpec{
						Version:                 upgradeServerBiosVersion,
						Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					},
					ServerSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "bar",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, biosVersionSet)).To(Succeed())

			By("Checking if the BIOSVersion has been created")
			biosVersion02 := &metalv1alpha1.BIOSVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: biosVersionSet.Name + "-" + server02.Name,
				},
			}
			Eventually(Get(biosVersion02)).Should(Succeed())

			By("Checking if the 2nd BIOSVersion has been created")
			biosVersion03 := &metalv1alpha1.BIOSVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: biosVersionSet.Name + "-" + server03.Name,
				},
			}
			Eventually(Get(biosVersion03)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(biosVersionSet)).WithTimeout(10 * time.Second).Should(SatisfyAll(
				HaveField("Status.FullyLabeledServers", BeNumerically("==", 2)),
				HaveField("Status.AvailableBIOSVersion", BeNumerically("==", 2)),
				HaveField("Status.FailedBIOSVersion", BeNumerically("==", 0)),
			))

			By("Checking the biosVersion01 have completed")
			Eventually(Object(biosVersion02)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
				HaveField("Spec.Version", biosVersionSet.Spec.BiosVersionTemplate.Version),
				HaveField("Spec.Image.URI", biosVersionSet.Spec.BiosVersionTemplate.Image.URI),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BIOSVersionSet",
					Name:               biosVersionSet.Name,
					UID:                biosVersionSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking the biosVersion02 have completed")
			Eventually(Object(biosVersion03)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
				HaveField("Spec.Version", biosVersionSet.Spec.BiosVersionTemplate.Version),
				HaveField("Spec.Image.URI", biosVersionSet.Spec.BiosVersionTemplate.Image.URI),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BIOSVersionSet",
					Name:               biosVersionSet.Name,
					UID:                biosVersionSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking if the status has been updated")
			Eventually(Object(biosVersionSet)).WithTimeout(10 * time.Second).Should(SatisfyAll(
				HaveField("Status.FullyLabeledServers", BeNumerically("==", 2)),
				HaveField("Status.AvailableBIOSVersion", BeNumerically("==", 2)),
				HaveField("Status.CompletedBIOSVersion", BeNumerically("==", 2)),
				HaveField("Status.InProgressBIOSVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBIOSVersion", BeNumerically("==", 0)),
			))

			By("Deleting the resource")
			Expect(k8sClient.Delete(ctx, biosVersionSet)).To(Succeed())
		})

		It("should successfully reconcile the resource when server are deleted/created", func(ctx SpecContext) {
			By("Create resource")
			biosVersionSet := &metalv1alpha1.BIOSVersionSet{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-biosversion-set-",
					Namespace:    ns.Name,
				},
				Spec: metalv1alpha1.BIOSVersionSetSpec{
					BiosVersionTemplate: metalv1alpha1.VersionUpdateSpec{
						Version:                 upgradeServerBiosVersion,
						Image:                   metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
						ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					},
					ServerSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "bar",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, biosVersionSet)).To(Succeed())

			By("Checking if the BIOSVersion has been created")
			biosVersion02 := &metalv1alpha1.BIOSVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: biosVersionSet.Name + "-" + server02.Name,
				},
			}
			Eventually(Get(biosVersion02)).Should(Succeed())

			By("Checking if the 2nd BIOSVersion has been created")
			biosVersion03 := &metalv1alpha1.BIOSVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: biosVersionSet.Name + "-" + server03.Name,
				},
			}
			Eventually(Get(biosVersion03)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(biosVersionSet)).WithTimeout(10 * time.Second).Should(SatisfyAll(
				HaveField("Status.FullyLabeledServers", BeNumerically("==", 2)),
				HaveField("Status.AvailableBIOSVersion", BeNumerically("==", 2)),
				HaveField("Status.FailedBIOSVersion", BeNumerically("==", 0)),
			))

			By("Checking the biosVersion01 have completed")
			Eventually(Object(biosVersion02)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BIOSVersionSet",
					Name:               biosVersionSet.Name,
					UID:                biosVersionSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking the biosVersion02 have completed")
			Eventually(Object(biosVersion03)).Should(SatisfyAll(
				HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
				HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
					APIVersion:         "metal.ironcore.dev/v1alpha1",
					Kind:               "BIOSVersionSet",
					Name:               biosVersionSet.Name,
					UID:                biosVersionSet.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))

			By("Checking if the status has been updated")
			Eventually(Object(biosVersionSet)).WithTimeout(10 * time.Second).Should(SatisfyAll(
				HaveField("Status.FullyLabeledServers", BeNumerically("==", 2)),
				HaveField("Status.AvailableBIOSVersion", BeNumerically("==", 2)),
				HaveField("Status.CompletedBIOSVersion", BeNumerically("==", 2)),
				HaveField("Status.InProgressBIOSVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBIOSVersion", BeNumerically("==", 0)),
			))

			By("Deleting the server02")
			Expect(k8sClient.Delete(ctx, server02)).To(Succeed())

			By("Checking if the BIOSVersion have been deleted")
			Eventually(Get(biosVersion02)).ShouldNot(Succeed())
			Eventually(Get(biosVersion03)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(biosVersionSet)).WithTimeout(10 * time.Second).Should(SatisfyAll(
				HaveField("Status.FullyLabeledServers", BeNumerically("==", 1)),
				HaveField("Status.AvailableBIOSVersion", BeNumerically("==", 1)),
				HaveField("Status.CompletedBIOSVersion", BeNumerically("==", 1)),
				HaveField("Status.InProgressBIOSVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBIOSVersion", BeNumerically("==", 0)),
			))

			By("creating the server02")
			server02.ResourceVersion = ""
			Expect(k8sClient.Create(ctx, server02)).Should(Succeed())
			By("Checking if the BIOSVersion have been created")
			Eventually(Get(biosVersion02)).Should(Succeed())
			Eventually(Get(biosVersion03)).Should(Succeed())

			By("Checking the biosVersion01 have completed")
			Eventually(Object(biosVersion02)).Should(
				HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
			)

			By("Checking if the status has been updated")
			Eventually(Object(biosVersionSet)).WithTimeout(10 * time.Second).Should(SatisfyAll(
				HaveField("Status.FullyLabeledServers", BeNumerically("==", 2)),
				HaveField("Status.AvailableBIOSVersion", BeNumerically("==", 2)),
				HaveField("Status.CompletedBIOSVersion", BeNumerically("==", 2)),
				HaveField("Status.InProgressBIOSVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBIOSVersion", BeNumerically("==", 0)),
			))

			By("Updating the label of server01")
			Eventually(Update(server01, func() {
				server01.Labels = map[string]string{
					"metal.ironcore.dev/Manufacturer": "bar",
				}
			})).Should(Succeed())

			By("Checking if the 3rd BIOSVersion has been created")
			biosVersion01 := &metalv1alpha1.BIOSVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: biosVersionSet.Name + "-" + server01.Name,
				},
			}
			Eventually(Get(biosVersion01)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(biosVersionSet)).WithTimeout(10 * time.Second).Should(SatisfyAll(
				HaveField("Status.FullyLabeledServers", BeNumerically("==", 3)),
				HaveField("Status.AvailableBIOSVersion", BeNumerically("==", 3)),
				HaveField("Status.FailedBIOSVersion", BeNumerically("==", 0)),
			))

			By("Checking the biosVersion01 have completed")
			Eventually(Object(biosVersion01)).Should(
				HaveField("Status.State", metalv1alpha1.BIOSVersionStateCompleted),
			)

			By("Checking if the status has been updated")
			Eventually(Object(biosVersionSet)).WithTimeout(10 * time.Second).Should(SatisfyAll(
				HaveField("Status.FullyLabeledServers", BeNumerically("==", 3)),
				HaveField("Status.AvailableBIOSVersion", BeNumerically("==", 3)),
				HaveField("Status.CompletedBIOSVersion", BeNumerically("==", 3)),
				HaveField("Status.InProgressBIOSVersion", BeNumerically("==", 0)),
				HaveField("Status.FailedBIOSVersion", BeNumerically("==", 0)),
			))
		})
	})
})
