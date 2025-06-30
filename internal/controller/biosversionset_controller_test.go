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

			By("Creating a Server")
			server01 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-maintenance-",
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

			By("Creating a second Server")
			server02 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-maintenance-",
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

			By("Creating a third Server")
			server03 = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-maintenance-",
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
			By("Reconciling the created resource")

			biosVersionSet := &metalv1alpha1.BIOSVersionSet{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-biosversion-set-",
					Namespace:    ns.Name,
				},
				Spec: metalv1alpha1.BIOSVersionSetSpec{
					Version: defaultMockUpServerBiosVersion,
					ServerSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "bar",
						},
					},
					ServerMaintenancePolicy: "Enforced",

					Image: metalv1alpha1.ImageSpec{URI: upgradeServerBiosVersion},
				},
			}
			Expect(k8sClient.Create(ctx, biosVersionSet)).To(Succeed())

			By("Checking if the BIOSVersion has been created")
			biosVersion01 := &metalv1alpha1.BIOSVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: biosVersionSet.Name + "-" + server02.Name,
				},
			}
			Eventually(Get(biosVersion01)).Should(Succeed())

			By("Checking if the 2nd BIOSVersion has been created")
			biosVersion02 := &metalv1alpha1.BIOSVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: biosVersionSet.Name + "-" + server03.Name,
				},
			}
			Eventually(Get(biosVersion02)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(biosVersionSet)).WithTimeout(10 * time.Second).Should(SatisfyAll(
				HaveField("Status.TotalServers", BeNumerically("==", 2)),
				HaveField("Status.TotalVersionResource", BeNumerically("==", 2)),
				HaveField("Status.InProgress", BeNumerically("==", 0)),
				HaveField("Status.Completed", BeNumerically("==", 2)),
				HaveField("Status.Failed", BeNumerically("==", 0)),
			))

			By("Deleting the resource")
			Expect(k8sClient.Delete(ctx, biosVersionSet)).To(Succeed())

			By("Checking if the BIOSVersion have been deleted")
			Eventually(Get(biosVersion01)).ShouldNot(Succeed())
			Eventually(Get(biosVersion02)).ShouldNot(Succeed())
		})
	})
})
