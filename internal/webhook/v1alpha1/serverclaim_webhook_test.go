// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("ServerClaim API Validation", func() {
	var (
		claim             *metalv1alpha1.ServerClaim
		claimWithSelector *metalv1alpha1.ServerClaim
	)

	BeforeEach(func() {
		By("Creating a new ServerClaim")
		claim = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    metav1.NamespaceDefault,
				GenerateName: "test-",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Updating the ServerRef to claim a Server")
		Eventually(Update(claim, func() {
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		})).Should(Succeed())

		By("Creating a new ServerClaim")
		claimWithSelector = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    metav1.NamespaceDefault,
				GenerateName: "test-",
			},
		}
		Expect(k8sClient.Create(ctx, claimWithSelector)).To(Succeed())

		By("Updating the ServerSelector to claim a Server")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			}
		})).Should(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())
		Expect(k8sClient.Delete(ctx, claimWithSelector)).To(Succeed())
	})

	It("should deny if the ServerRef changes", func() {
		By("Updating the ServerRef to claim a different Server")
		Eventually(Update(claim, func() {
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "bar"}
		})).Should(HaveOccurred())

		By("Ensuring that the ServerRef did not change")
		Consistently(Object(claim)).Should(HaveField("Spec.ServerRef.Name", Equal("foo")))
	})

	It("should allow a change of ServerClaim by not changing the ServerRef", func() {
		By("Updating the ServerClaim without changing the ServerRef")
		Eventually(Update(claim, func() {
			claim.Spec.Power = metalv1alpha1.PowerOn
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		})).Should(Succeed())

		By("Ensuring that the PowerState changed")
		Consistently(Object(claim)).Should(SatisfyAll(
			HaveField("Spec.Power", Equal(metalv1alpha1.PowerOn)),
			HaveField("Spec.ServerRef.Name", Equal("foo")),
		))
	})

	It("should deny if the ServerSelector changes", func() {
		By("Updating the ServerSelector to select different labels")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"bar": "foo",
				},
			}
		})).Should(HaveOccurred())

		By("Ensuring that the ServerSelector did not change")
		Consistently(Object(claimWithSelector)).Should(
			HaveField("Spec.ServerSelector.MatchLabels", Equal(map[string]string{"foo": "bar"})))
	})

	It("should allow a change of ServerClaim by not changing the ServerSelector", func() {
		By("Updating the ServerClaim without changing the ServerSelector")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.Power = metalv1alpha1.PowerOn
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			}
		})).Should(Succeed())

		By("Ensuring that the PowerState changed")
		Consistently(Object(claimWithSelector)).Should(SatisfyAll(
			HaveField("Spec.Power", Equal(metalv1alpha1.PowerOn)),
			HaveField("Spec.ServerSelector.MatchLabels", Equal(map[string]string{"foo": "bar"}))))
	})
})
