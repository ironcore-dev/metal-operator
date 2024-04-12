// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	})

	It("Should successfully reconcile the a BMC resource", func(ctx SpecContext) {
		By("Ensuring that the BMC resource has been created for an endpoint")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("bmc-%s", endpoint.Name),
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
			HaveField("Spec.BMCRef.Name", GetBMCNameFromEndpoint(endpoint)),
		))
	})
})
