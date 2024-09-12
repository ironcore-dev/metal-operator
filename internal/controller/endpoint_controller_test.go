// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"encoding/base64"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("Endpoints Controller", func() {
	_ = SetupTest()

	It("should successfully create a BMC secret and BMC object from endpoint", func(ctx SpecContext) {
		By("Creating an Endpoints object")
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

		By("Ensuring that the BMC secret has been created")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: endpoint.Name,
			},
		}
		Eventually(Object(bmcSecret)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "Endpoint",
				Name:               endpoint.Name,
				UID:                endpoint.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Data", Equal(map[string][]byte{
				"username": []byte(base64.StdEncoding.EncodeToString([]byte("foo"))),
				"password": []byte(base64.StdEncoding.EncodeToString([]byte("bar"))),
			}))))
		DeferCleanup(k8sClient.Delete, bmcSecret)

		By("By ensuring that the BMC object has been created")
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
			HaveField("Spec.EndpointRef.Name", Equal(endpoint.Name)),
			HaveField("Spec.BMCSecretRef.Name", Equal(bmc.Name)),
			HaveField("Spec.Protocol", metalv1alpha1.Protocol{
				Name: metalv1alpha1.ProtocolRedfishLocal,
				Port: 8000,
			}),
			HaveField("Spec.ConsoleProtocol", &metalv1alpha1.ConsoleProtocol{
				Name: metalv1alpha1.ConsoleProtocolNameSSH,
				Port: 22,
			})))

		By("Removing the endpoint")
		Expect(k8sClient.Delete(ctx, endpoint)).To(Succeed())

		By("Ensuring that all subsequent objects have been removed")
		Eventually(Get(endpoint)).Should(Satisfy(apierrors.IsNotFound))
	})
})
