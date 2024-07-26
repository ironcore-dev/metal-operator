// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("ServerClaim Webhook", func() {
	ns := SetupTest()

	var claim *ServerClaim
	var claimWithSelector *ServerClaim

	BeforeEach(func(ctx SpecContext) {
		By("creating a new ServerClaim")
		claim = &ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "claim-",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())
		DeferCleanup(k8sClient.Delete, claim)

		By("updating the ServerRef to claim a Server")
		Eventually(Update(claim, func() {
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		})).Should(Succeed())

		By("creating a new ServerClaim")
		claimWithSelector = &ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "claim-",
			},
		}
		Expect(k8sClient.Create(ctx, claimWithSelector)).To(Succeed())
		DeferCleanup(k8sClient.Delete, claimWithSelector)

		By("updating the ServerSelector to claim a Server")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			}
		})).Should(Succeed())
	})

	It("Should deny if the ServerRef changes", func() {
		By("updating the ServerRef to claim a different Server")
		Eventually(Update(claim, func() {
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "bar"}
		})).Should(HaveOccurred())

		By("ensuring that the ServerRef did not change")
		Consistently(Object(claim)).Should(HaveField("Spec.ServerRef.Name", Equal("foo")))
	})

	It("Should allow a change of ServerClaim by not changing the ServerRef", func() {
		By("updating the ServerRef to claim a different Server")
		Eventually(Update(claim, func() {
			claim.Spec.Power = PowerOn
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		})).Should(Succeed())

		By("ensuring that the PowerState changed")
		Consistently(Object(claim)).Should(SatisfyAll(
			HaveField("Spec.Power", Equal(PowerOn)),
			HaveField("Spec.ServerRef.Name", Equal("foo")),
		))
	})

	It("Should deny if the ServerSelector changes", func() {
		By("updating the ServerRef to claim a different Server")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"bar": "foo",
				},
			}
		})).Should(HaveOccurred())

		By("ensuring that the ServerRef did not change")
		Consistently(Object(claimWithSelector)).Should(
			HaveField("Spec.ServerSelector.MatchLabels", Equal(map[string]string{"foo": "bar"})))
	})

	It("Should allow a change of ServerClaim by not changing the ServerSelector", func() {
		By("updating the ServerRef to claim a different Server")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.Power = PowerOn
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			}
		})).Should(Succeed())

		By("ensuring that the PowerState changed")
		Consistently(Object(claimWithSelector)).Should(SatisfyAll(
			HaveField("Spec.Power", Equal(PowerOn)),
			HaveField("Spec.ServerSelector.MatchLabels", Equal(map[string]string{"foo": "bar"}))))
	})
})
