// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("BIOSSettingsSet Controller", func() {
	Context("When reconciling a resource", func() {
		ns := SetupTest()

		var server01 *metalv1alpha1.Server
		var server02 *metalv1alpha1.Server
		var server03 *metalv1alpha1.Server

		BeforeEach(func(ctx SpecContext) {
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

			biosSettingsSet := &metalv1alpha1.BIOSSettingsSet{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-biossettings-set-",
					Namespace:    ns.Name,
				},
				Spec: metalv1alpha1.BIOSSettingsSetSpec{
					Version: defaultMockUpServerBiosVersion,
					ServerSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"metal.ironcore.dev/Manufacturer": "bar",
						},
					},
					ServerMaintenancePolicy: "Enforced",

					SettingsFlow: []metalv1alpha1.SettingsFlowItem{
						{Settings: map[string]string{"fooreboot": "144"}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, biosSettingsSet)).To(Succeed())

			By("Checking if the biosSettings has been created")
			biosSettings01 := &metalv1alpha1.BIOSSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: biosSettingsSet.Name + "-" + server02.Name,
				},
			}
			Eventually(Get(biosSettings01)).Should(Succeed())

			By("Checking if the 2nd maintenance has been created")
			biosSettings02 := &metalv1alpha1.BIOSSettings{
				ObjectMeta: metav1.ObjectMeta{
					Name: biosSettingsSet.Name + "-" + server03.Name,
				},
			}
			Eventually(Get(biosSettings02)).Should(Succeed())

			By("Checking if the status has been updated")
			Eventually(Object(biosSettingsSet)).Should(SatisfyAll(
				HaveField("Status.TotalServers", BeNumerically("==", 2)),
				HaveField("Status.TotalSettings", BeNumerically("==", 2)),
				HaveField("Status.InProgress", BeNumerically("==", 0)),
				HaveField("Status.Completed", BeNumerically("==", 2)),
				HaveField("Status.Failed", BeNumerically("==", 0)),
			))

			By("Deleting the resource")
			Expect(k8sClient.Delete(ctx, biosSettingsSet)).To(Succeed())

			By("Checking if the BIOSSettings have been deleted")
			Eventually(Get(biosSettings01)).ShouldNot(Succeed())
			Eventually(Get(biosSettings02)).ShouldNot(Succeed())
		})
	})
})
