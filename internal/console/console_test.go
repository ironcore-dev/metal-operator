// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package console

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Console Access", func() {
	_ = SetupTest()

	It("Should successfully construct console config for Server with inline configuration", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmcSecret)

		By("Creating a Server object")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-console",
			},
			Spec: metalv1alpha1.ServerSpec{
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{},
					Address:  "10.0.0.1",
					BMCSecretRef: corev1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)

		config, err := GetConfigForServerName(ctx, k8sClient, server.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(Equal(&Config{
			BMCAddress: "10.0.0.1",
			Username:   "foo",
			Password:   "bar",
		}))
	})

	It("Should successfully construct console config for Server with a BMC ref", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				"username": []byte("foo"),
				"password": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmcSecret)

		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.BMCSpec{
				BMCSecretRef: corev1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
				Endpoint: &metalv1alpha1.InlineEndpoint{
					MACAddress: "aa:bb:cc:dd",
					IP:         metalv1alpha1.MustParseIP("10.0.0.1"),
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmc)

		By("Creating a Server object")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-console",
			},
			Spec: metalv1alpha1.ServerSpec{
				BMCRef: &corev1.LocalObjectReference{
					Name: bmc.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)

		config, err := GetConfigForServerName(ctx, k8sClient, server.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(Equal(&Config{
			BMCAddress: "10.0.0.1",
			Username:   "foo",
			Password:   "bar",
		}))
	})
})
