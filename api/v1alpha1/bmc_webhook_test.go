// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMC Webhook", func() {
	_ = SetupTest()

	It("Should deny if the BMC has EndpointRef and InlineEndpoint spec fields", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-invalid",
			},
			Spec: BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
				Endpoint: &InlineEndpoint{
					IP:         MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(HaveOccurred())
		Eventually(Get(bmc)).Should(Satisfy(errors.IsNotFound))
	})

	It("Should deny if the BMC has no EndpointRef and InlineEndpoint spec fields", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-empty",
			},
			Spec: BMCSpec{},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(HaveOccurred())
		Eventually(Get(bmc)).Should(Satisfy(errors.IsNotFound))
	})

	It("Should admit if the BMC has an EndpointRef but no InlineEndpoint spec field", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)
	})

	It("Should deny if the BMC EndpointRef spec field has been removed", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		Eventually(Update(bmc, func() {
			bmc.Spec.EndpointRef = nil
		})).Should(Not(Succeed()))

		Eventually(Object(bmc)).Should(SatisfyAll(HaveField(
			"Spec.EndpointRef", &v1.LocalObjectReference{Name: "foo"})))
	})

	It("Should admit if the BMC is changing EndpointRef to InlineEndpoint spec field", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		Eventually(Update(bmc, func() {
			bmc.Spec.EndpointRef = nil
			bmc.Spec.Endpoint = &InlineEndpoint{
				IP:         MustParseIP("127.0.0.1"),
				MACAddress: "aa:bb:cc:dd:ee:ff",
			}
		})).Should(Succeed())
	})

	It("Should admit if the BMC has no EndpointRef but an InlineEndpoint spec field", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: BMCSpec{
				Endpoint: &InlineEndpoint{
					IP:         MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)
	})

	It("Should deny if the BMC InlineEndpoint spec field has been removed", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: BMCSpec{
				Endpoint: &InlineEndpoint{
					IP:         MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		Eventually(Update(bmc, func() {
			bmc.Spec.Endpoint = nil
		})).Should(Not(Succeed()))

		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.Endpoint.IP", MustParseIP("127.0.0.1")),
			HaveField("Spec.Endpoint.MACAddress", "aa:bb:cc:dd:ee:ff"),
		))
	})

	It("Should admit if the BMC has is changing to an EndpointRef from an InlineEndpoint spec field", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: BMCSpec{
				Endpoint: &InlineEndpoint{
					IP:         MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		Eventually(Update(bmc, func() {
			bmc.Spec.EndpointRef = &v1.LocalObjectReference{Name: "foo"}
			bmc.Spec.Endpoint = nil
		})).Should(Succeed())
	})

})
