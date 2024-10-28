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

	It("Should deny if the BMC has EndpointRef and Access spec fields", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-invalid",
			},
			Spec: BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
				Access: &Access{
					Address: "localhost",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(HaveOccurred())
		Eventually(Get(bmc)).Should(Satisfy(errors.IsNotFound))
	})

	It("Should deny if the BMC has no EndpointRef and Access spec fields", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-empty",
			},
			Spec: BMCSpec{},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(HaveOccurred())
		Eventually(Get(bmc)).Should(Satisfy(errors.IsNotFound))
	})

	It("Should admit if the BMC has an EndpointRef but no Access spec field", func() {
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

	It("Should admit if the BMC is changing EndpointRef to Access spec field", func() {
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
			bmc.Spec.Access = &Access{
				Address: "localhost",
			}
		})).Should(Succeed())
	})

	It("Should admit if the BMC has no EndpointRef but an Access spec field", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: BMCSpec{
				Access: &Access{
					Address: "localhost",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)
	})

	It("Should deny if the BMC Access spec field has been removed", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: BMCSpec{
				Access: &Access{
					Address: "localhost",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		Eventually(Update(bmc, func() {
			bmc.Spec.Access = nil
		})).Should(Not(Succeed()))

		Eventually(Object(bmc)).Should(SatisfyAll(HaveField(
			"Spec.Access.Address", "localhost")))
	})

	It("Should admit if the BMC has is changing to an EndpointRef from an Access spec field", func() {
		bmc := &BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: BMCSpec{
				Access: &Access{
					Address: "localhost",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		Eventually(Update(bmc, func() {
			bmc.Spec.EndpointRef = &v1.LocalObjectReference{Name: "foo"}
			bmc.Spec.Access = nil
		})).Should(Succeed())
	})

})
