// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"net/netip"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("BIOSSettingsSet Controller", func() {
	var (
		MockServerIPAddrs = []netip.AddrPort{
			netip.MustParseAddrPort(fmt.Sprintf("%s:%d", MockServerIP, MockServerPort)),
			netip.MustParseAddrPort(fmt.Sprintf("%s:%d", MockServerIP, MockServerPort+1)),
			netip.MustParseAddrPort(fmt.Sprintf("%s:%d", MockServerIP, MockServerPort+2)),
		}
		server01  *metalv1alpha1.Server
		server02  *metalv1alpha1.Server
		server03  *metalv1alpha1.Server
		bmcSecret *metalv1alpha1.BMCSecret
	)
	ns := SetupTest(MockServerIPAddrs)

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
		server01 = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-maintenance-01-",
				Labels: map[string]string{
					"metal.ironcore.dev/Manufacturer": "foo",
				},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: int32(MockServerIPAddrs[0].Port()),
					},
					Address: MockServerIPAddrs[0].Addr().String(),
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
				GenerateName: "test-maintenance-02-",
				Labels: map[string]string{
					"metal.ironcore.dev/Manufacturer": "bar",
				},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: int32(MockServerIPAddrs[1].Port()),
					},
					Address: MockServerIPAddrs[1].Addr().String(),
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
				GenerateName: "test-maintenance-03-",
				Labels: map[string]string{
					"metal.ironcore.dev/Manufacturer": "bar",
				},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: int32(MockServerIPAddrs[2].Port()),
					},
					Address: MockServerIPAddrs[2].Addr().String(),
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server03)).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(k8sClient.Delete(ctx, server01)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server02)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server03)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())

		EnsureCleanState()
	})

	It("Should successfully reconcile the resource", func(ctx SpecContext) {
		By("Reconciling the created resource")
		biosSettingsSet := &metalv1alpha1.BIOSSettingsSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-biossettings-set-",
				Namespace:    ns.Name,
			},
			// settings mocked at
			// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
			Spec: metalv1alpha1.BIOSSettingsSetSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version:                 defaultMockUpServerBiosVersion,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{
						{Settings: map[string]string{"ProcCores": "2"}, Priority: 1, Name: "one"},
					},
				},
				ServerSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "bar",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsSet)).To(Succeed())

		By("Checking if the biosSettings has been created")
		biosSettings02 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsSet.Name + "-" + server02.Name,
			},
		}
		Eventually(Get(biosSettings02)).Should(Succeed())

		By("Checking the biosSettings02 have completed")
		Eventually(Object(biosSettings02)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Spec.Version", biosSettingsSet.Spec.BIOSSettingsTemplate.Version),
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BIOSSettingsSet",
				Name:               biosSettingsSet.Name,
				UID:                biosSettingsSet.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
		))

		By("Checking if the 2nd BIOSSettings has been created")
		biosSettings03 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsSet.Name + "-" + server03.Name,
			},
		}
		Eventually(Get(biosSettings03)).Should(Succeed())

		By("Checking the biosSettings03 have completed")
		Eventually(Object(biosSettings03)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("Spec.Version", biosSettingsSet.Spec.BIOSSettingsTemplate.Version),
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BIOSSettingsSet",
				Name:               biosSettingsSet.Name,
				UID:                biosSettingsSet.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
		))

		By("Checking if the status has been updated")
		Eventually(Object(biosSettingsSet)).Should(SatisfyAll(
			HaveField("Status.FullyLabeledServers", BeNumerically("==", 2)),
			HaveField("Status.AvailableBIOSSettings", BeNumerically("==", 2)),
			HaveField("Status.InProgressBIOSSettings", BeNumerically("==", 0)),
			HaveField("Status.CompletedBIOSSettings", BeNumerically("==", 2)),
			HaveField("Status.FailedBIOSSettings", BeNumerically("==", 0)),
		))

		By("Deleting the resource")
		Expect(k8sClient.Delete(ctx, biosSettingsSet)).To(Succeed())
		Eventually(Get(biosSettingsSet)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, biosSettings03)).To(Succeed())
		Eventually(Get(biosSettings03)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, biosSettings02)).To(Succeed())
		Eventually(Get(biosSettings02)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(server01)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
		Eventually(Object(server02)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
		Eventually(Object(server03)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})

	It("Should successfully reconcile the resource when server are deleted/created", func(ctx SpecContext) {
		By("Create resource")
		biosSettingsSet := &metalv1alpha1.BIOSSettingsSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-biossettings-set-",
				Namespace:    ns.Name,
			},
			// settings mocked at
			// metal-operator/bmc/mock/server/data/Registries/BiosAttributeRegistry.v1_0_0.json
			Spec: metalv1alpha1.BIOSSettingsSetSpec{
				BIOSSettingsTemplate: metalv1alpha1.BIOSSettingsTemplate{
					Version:                 defaultMockUpServerBiosVersion,
					ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
					SettingsFlow: []metalv1alpha1.SettingsFlowItem{
						{Settings: map[string]string{"AdminPhone": "foo-bar"}, Priority: 10, Name: "foo-bar"},
					},
				},
				ServerSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"metal.ironcore.dev/Manufacturer": "bar",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, biosSettingsSet)).To(Succeed())

		By("Checking if the BIOSSettings has been created")
		biosSettings02 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsSet.Name + "-" + server02.Name,
			},
		}
		Eventually(Get(biosSettings02)).Should(Succeed())

		By("Checking if the 2nd BIOSSettings has been created")
		biosSettings03 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsSet.Name + "-" + server03.Name,
			},
		}
		Eventually(Get(biosSettings03)).Should(Succeed())

		By("Checking if the status has been updated")
		Eventually(Object(biosSettingsSet)).Should(SatisfyAll(
			HaveField("Status.FullyLabeledServers", BeNumerically("==", 2)),
			HaveField("Status.AvailableBIOSSettings", BeNumerically("==", 2)),
			HaveField("Status.FailedBIOSSettings", BeNumerically("==", 0)),
		))

		By("Checking the biosSettings02 have completed")
		Eventually(Object(biosSettings02)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BIOSSettingsSet",
				Name:               biosSettingsSet.Name,
				UID:                biosSettingsSet.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
		))

		By("Checking the biosSettings03 have completed")
		Eventually(Object(biosSettings03)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BIOSSettingsSet",
				Name:               biosSettingsSet.Name,
				UID:                biosSettingsSet.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
		))

		By("Checking if the status has been updated")
		Eventually(Object(biosSettingsSet)).Should(SatisfyAll(
			HaveField("Status.FullyLabeledServers", BeNumerically("==", 2)),
			HaveField("Status.AvailableBIOSSettings", BeNumerically("==", 2)),
			HaveField("Status.CompletedBIOSSettings", BeNumerically("==", 2)),
			HaveField("Status.InProgressBIOSSettings", BeNumerically("==", 0)),
			HaveField("Status.FailedBIOSSettings", BeNumerically("==", 0)),
		))

		By("Deleting the server02")
		Expect(k8sClient.Delete(ctx, server02)).To(Succeed())
		Eventually(Get(server02)).ShouldNot(Succeed())

		By("Checking if the BIOSSettings have been deleted")
		Eventually(Get(biosSettings02)).ShouldNot(Succeed())
		Eventually(Get(biosSettings03)).Should(Succeed())

		By("Checking if the status has been updated")
		Eventually(Object(biosSettingsSet)).Should(SatisfyAll(
			HaveField("Status.FullyLabeledServers", BeNumerically("==", 1)),
			HaveField("Status.AvailableBIOSSettings", BeNumerically("==", 1)),
			HaveField("Status.CompletedBIOSSettings", BeNumerically("==", 1)),
			HaveField("Status.InProgressBIOSSettings", BeNumerically("==", 0)),
			HaveField("Status.FailedBIOSSettings", BeNumerically("==", 0)),
		))

		By("creating the server02")
		server02.ResourceVersion = ""
		server02.Spec.BIOSSettingsRef = nil
		Expect(k8sClient.Create(ctx, server02)).Should(Succeed())

		By("Checking if the BIOSSettings have been created")
		Eventually(Get(biosSettings02)).Should(Succeed())
		Eventually(Get(biosSettings03)).Should(Succeed())

		By("Checking the biosSettings02 have completed")
		Eventually(Object(biosSettings02)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		)

		By("Checking if the status has been updated")
		Eventually(Object(biosSettingsSet)).Should(SatisfyAll(
			HaveField("Status.FullyLabeledServers", BeNumerically("==", 2)),
			HaveField("Status.AvailableBIOSSettings", BeNumerically("==", 2)),
			HaveField("Status.CompletedBIOSSettings", BeNumerically("==", 2)),
			HaveField("Status.InProgressBIOSSettings", BeNumerically("==", 0)),
			HaveField("Status.FailedBIOSSettings", BeNumerically("==", 0)),
		))

		By("Updating the label of server01")
		Eventually(Update(server01, func() {
			server01.Labels = map[string]string{
				"metal.ironcore.dev/Manufacturer": "bar",
			}
		})).Should(Succeed())

		By("Checking if the 3rd BIOSSettings has been created")
		biosSettings01 := &metalv1alpha1.BIOSSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name: biosSettingsSet.Name + "-" + server01.Name,
			},
		}
		Eventually(Get(biosSettings01)).Should(Succeed())

		By("Checking if the status has been updated")
		Eventually(Object(biosSettingsSet)).Should(SatisfyAll(
			HaveField("Status.FullyLabeledServers", BeNumerically("==", 3)),
			HaveField("Status.AvailableBIOSSettings", BeNumerically("==", 3)),
			HaveField("Status.FailedBIOSSettings", BeNumerically("==", 0)),
		))

		By("Checking the biosSettings01 have completed")
		Eventually(Object(biosSettings01)).Should(
			HaveField("Status.State", metalv1alpha1.BIOSSettingsStateApplied),
		)

		By("Checking if the status has been updated")
		Eventually(Object(biosSettingsSet)).Should(SatisfyAll(
			HaveField("Status.FullyLabeledServers", BeNumerically("==", 3)),
			HaveField("Status.AvailableBIOSSettings", BeNumerically("==", 3)),
			HaveField("Status.CompletedBIOSSettings", BeNumerically("==", 3)),
			HaveField("Status.InProgressBIOSSettings", BeNumerically("==", 0)),
			HaveField("Status.FailedBIOSSettings", BeNumerically("==", 0)),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, biosSettingsSet)).To(Succeed())
		Eventually(Get(biosSettingsSet)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, biosSettings01)).To(Succeed())
		Eventually(Get(biosSettings01)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, biosSettings02)).To(Succeed())
		Eventually(Get(biosSettings02)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, biosSettings03)).To(Succeed())
		Eventually(Get(biosSettings03)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(server01)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
		Eventually(Object(server02)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
		Eventually(Object(server03)).Should(
			HaveField("Status.State", Not(Equal(metalv1alpha1.ServerStateMaintenance))),
		)
	})
})
