// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("ServerBIOS Controller", func() {
	_ = SetupTest()

	It("Should retrieve the BIOS version", func(ctx SpecContext) {
		By("Creating an Endpoint object")
		endpoint := &metalv1alpha1.Endpoint{
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
		Eventually(Get(endpoint)).Should(Succeed())
		DeferCleanup(k8sClient.Delete, endpoint)

		By("Ensuring that the BMC resource has been created for an endpoint")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: endpoint.Name,
			},
		}
		Eventually(Get(bmc)).Should(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		By("Ensuring that the BMCSecret will be removed")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmc.Name,
			},
		}
		Eventually(Get(bmcSecret)).Should(Succeed())
		DeferCleanup(k8sClient.Delete, bmcSecret)

		By("Ensuring that the Server resource has been created")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-system-0", bmc.Name),
			},
		}
		// todo: only check that the server has been created it's enough for serverBios reconciliation
		//  with only scanning invoked
		Eventually(Get(server)).Should(Succeed())
		// Eventually(Object(server)).Should(SatisfyAll(
		// 	HaveField("Finalizers", ContainElement(ServerFinalizer)),
		// 	HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
		// 		APIVersion:         "metal.ironcore.dev/v1alpha1",
		// 		Kind:               "BMC",
		// 		Name:               bmc.Name,
		// 		UID:                bmc.UID,
		// 		Controller:         ptr.To(true),
		// 		BlockOwnerDeletion: ptr.To(true),
		// 	})),
		// 	HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
		// 	HaveField("Spec.Power", metalv1alpha1.PowerOff),
		// 	HaveField("Spec.IndicatorLED", metalv1alpha1.IndicatorLED("")),
		// 	HaveField("Spec.ServerClaimRef", BeNil()),
		// 	HaveField("Status.Manufacturer", "Contoso"),
		// 	HaveField("Status.Model", "3500"),
		// 	HaveField("Status.SKU", "8675309"),
		// 	HaveField("Status.SerialNumber", "437XR1138R2"),
		// 	HaveField("Status.IndicatorLED", metalv1alpha1.OffIndicatorLED),
		// 	HaveField("Status.State", metalv1alpha1.ServerStateDiscovery),
		// 	HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
		// ))

		DeferCleanup(k8sClient.Delete, server)

		By("Creating a ServerBIOS object")
		serverBIOS := &metalv1alpha1.ServerBIOS{
			ObjectMeta: metav1.ObjectMeta{
				Name: server.Name,
			},
			Spec: metalv1alpha1.ServerBIOSSpec{
				ServerRef: v1.LocalObjectReference{Name: server.Name},
			},
		}
		Expect(k8sClient.Create(ctx, serverBIOS)).To(Succeed())
		DeferCleanup(k8sClient.Delete, serverBIOS)

		By("Ensuring that the BIOS version has been retrieved")
		Eventually(Object(serverBIOS)).Should(SatisfyAll(
			HaveField("Status.BIOS.Version", "P79 v1.45 (12/06/2017)"),
		))
	})
})
