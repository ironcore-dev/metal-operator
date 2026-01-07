// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/ironcore-dev/metal-operator/internal/controller"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("Endpoint Webhook", func() {
	var (
		obj       *metalv1alpha1.Endpoint
		oldObj    *metalv1alpha1.Endpoint
		validator EndpointCustomValidator
	)

	BeforeEach(func() {
		obj = &metalv1alpha1.Endpoint{}
		oldObj = &metalv1alpha1.Endpoint{}
		validator = EndpointCustomValidator{
			Client: k8sClient,
		}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		By("Deleting the Endpoint")
		Expect(k8sClient.DeleteAllOf(ctx, &metalv1alpha1.Endpoint{})).To(Succeed())

		By("Ensuring clean state")
		controller.EnsureCleanState()
	})

	Context("When creating or updating an Endpoint under Validating Webhook", func() {
		It("Should deny creation if an Endpoint has a duplicate MAC address", func(ctx SpecContext) {
			By("Creating an Endpoint")
			endpoint := &metalv1alpha1.Endpoint{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.EndpointSpec{
					IP:         metalv1alpha1.MustParseIP("1.1.1.1"),
					MACAddress: "foo",
				},
			}
			Expect(k8sClient.Create(ctx, endpoint)).To(Succeed())

			By("Creating an Endpoint with existing MAC address")
			existingEndpoint := &metalv1alpha1.Endpoint{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.EndpointSpec{
					IP:         metalv1alpha1.MustParseIP("2.2.2.2"),
					MACAddress: "foo",
				},
			}
			Expect(validator.ValidateCreate(ctx, existingEndpoint)).Error().To(HaveOccurred())
		})

		It("Should allow creation if an Endpoint has a unique MAC address", func(ctx SpecContext) {
			By("Creating an Endpoint")
			endpoint := &metalv1alpha1.Endpoint{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.EndpointSpec{
					IP:         metalv1alpha1.MustParseIP("1.1.1.1"),
					MACAddress: "foo",
				},
			}
			Expect(k8sClient.Create(ctx, endpoint)).ToNot(HaveOccurred())

			By("Creating an Endpoint with non-existing MAC address")
			existingEndpoint := &metalv1alpha1.Endpoint{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.EndpointSpec{
					IP:         metalv1alpha1.MustParseIP("2.2.2.2"),
					MACAddress: "bar",
				},
			}
			Expect(validator.ValidateCreate(ctx, existingEndpoint)).Error().ToNot(HaveOccurred())
		})

		It("Should deny update of an Endpoint with existing MAC address", func() {
			By("Creating an Endpoint")
			endpoint := &metalv1alpha1.Endpoint{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.EndpointSpec{
					IP:         metalv1alpha1.MustParseIP("1.1.1.1"),
					MACAddress: "foo",
				},
			}
			Expect(k8sClient.Create(ctx, endpoint)).To(Succeed())

			By("Creating an Endpoint with different MAC address")
			existingEndpoint := &metalv1alpha1.Endpoint{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.EndpointSpec{
					IP:         metalv1alpha1.MustParseIP("2.2.2.2"),
					MACAddress: "bar",
				},
			}
			Expect(k8sClient.Create(ctx, existingEndpoint)).To(Succeed())

			By("Updating an Endpoint to conflicting MAC address")
			updatedEndpoint := endpoint.DeepCopy()
			updatedEndpoint.Spec.MACAddress = "bar"
			Expect(validator.ValidateUpdate(ctx, endpoint, updatedEndpoint)).Error().To(HaveOccurred())
		})

		It("Should allow update an IP address of the same Endpoint", func() {
			By("Creating an Endpoint")
			existingEndpoint := &metalv1alpha1.Endpoint{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
				Spec: metalv1alpha1.EndpointSpec{
					IP:         metalv1alpha1.MustParseIP("1.1.1.1"),
					MACAddress: "foo",
				},
			}
			Expect(k8sClient.Create(ctx, existingEndpoint)).To(Succeed())

			By("Updating an Endpoint IP address")
			updatedEndpoint := existingEndpoint.DeepCopy()
			updatedEndpoint.Spec.IP = metalv1alpha1.MustParseIP("2.2.2.2")
			Expect(validator.ValidateUpdate(ctx, existingEndpoint, updatedEndpoint)).Error().ToNot(HaveOccurred())
		})
	})
})
