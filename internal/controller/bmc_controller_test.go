// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMC Controller", func() {
	_ = SetupTest()

	var endpoint *metalv1alpha1.Endpoint

	BeforeEach(func(ctx SpecContext) {
		By("Creating an Endpoints object")
		endpoint = &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.EndpointSpec{
				// emulator BMC mac address
				MACAddress: "23:11:8A:33:CF:EA",
				IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
			},
		}
		Expect(k8sClient.Create(ctx, endpoint)).To(Succeed())
		DeferCleanup(k8sClient.Delete, endpoint)

		By("Ensuring that the BMC will be removed")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: endpoint.Name,
			},
		}
		DeferCleanup(k8sClient.Delete, bmc)

		By("Ensuring that the BMCSecret will be removed")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmc.Name,
			},
		}
		DeferCleanup(k8sClient.Delete, bmcSecret)

		By("Ensuring that the Server resource will be removed")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		DeferCleanup(k8sClient.Delete, server)
	})

	It("Should successfully reconcile the a BMC resource", func(ctx SpecContext) {
		By("Ensuring that the BMC resource has been created for an endpoint")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: endpoint.Name,
			},
		}
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "Endpoint",
				Name:               endpoint.Name,
				UID:                endpoint.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Status.IP", metalv1alpha1.MustParseIP("127.0.0.1")),
			HaveField("Status.MACAddress", "23:11:8A:33:CF:EA"),
			HaveField("Status.Model", "Joo Janta 200"),
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			HaveField("Status.PowerState", metalv1alpha1.OnPowerState),
			HaveField("Status.FirmwareVersion", "1.45.455b66-rev4"),
		))

		By("Ensuring that the Server resource has been created")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BMC",
				Name:               bmc.Name,
				UID:                bmc.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.BMCRef.Name", endpoint.Name),
		))
	})

	It("Should successfully reconcile the a BMC resource with inline access information", func(ctx SpecContext) {
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
		DeferCleanup(k8sClient.Delete, bmcSecret)

		By("Creating a BMC resource")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
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
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.IP", metalv1alpha1.MustParseIP("127.0.0.1")),
			HaveField("Status.MACAddress", "23:11:8A:33:CF:EA"),
			HaveField("Status.Model", "Joo Janta 200"),
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			HaveField("Status.PowerState", metalv1alpha1.OnPowerState),
			HaveField("Status.FirmwareVersion", "1.45.455b66-rev4"),
		))

		By("Ensuring that the Server resource has been created")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BMC",
				Name:               bmc.Name,
				UID:                bmc.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.BMCRef.Name", bmc.Name),
		))
	})

})
