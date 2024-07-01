// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("ServerBootConfiguration Controller", func() {
	ns := SetupTest()

	var server *metalv1alpha1.Server

	BeforeEach(func(ctx SpecContext) {
		By("Creating a Server object")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Annotations: map[string]string{
					metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationIgnore,
				},
			},
			Spec: metalv1alpha1.ServerSpec{},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)
	})

	It("should successfully add the boot configuration ref to server", func(ctx SpecContext) {
		By("By creating a server boot configuration")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      server.Name,
			},
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: v1.LocalObjectReference{Name: server.Name},
				Image:     "foo:latest",
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(k8sClient.Delete, config)

		Eventually(Object(config)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
		))
	})
})
