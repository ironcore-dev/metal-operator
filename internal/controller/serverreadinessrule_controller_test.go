// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("ServerReadinessRule Controller", func() {
	SetupTest(nil)

	It("should delete the annotation from all servers if the rule gets deleted", func(ctx SpecContext) {
		By("creating a server")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
				Labels: map[string]string{
					"foo": "bar",
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).Should(Succeed())
		DeferCleanup(k8sClient.Delete, server)

		By("creating a server readiness rule")
		rule := &metalv1alpha1.ServerReadinessRule{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "rule-",
			},
			Spec: metalv1alpha1.ServerReadinessRuleSpec{
				Conditions: []metalv1alpha1.ConditionRequirement{
					{
						Type:           "CustomType",
						RequiredStatus: metav1.ConditionTrue,
					},
				},
				EnforcementMode: metalv1alpha1.EnforcementModeContinuous,
				Taint: metalv1alpha1.Taint{
					Key:    "SomeTaint",
					Value:  "SomeValue",
					Effect: metalv1alpha1.TaintEffectNoBind,
				},
				ServerSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo": "bar",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, rule)).Should(Succeed())

		By("waiting for the rule to have a finalizer")
		Eventually(Object(rule)).Should(HaveField("Finalizers", Equal([]string{serverReadinessRuleFinalizer})))

		By("deleting the rule")
		Expect(k8sClient.Delete(ctx, rule)).Should(Succeed())

		By("waiting for the rule to be gone")
		Eventually(Get(rule)).Should(Satisfy(apierrors.IsNotFound))

		By("asserting the server not to have the taint anymore")
		Expect(Object(server)()).Should(HaveField("Spec.Taints", BeEmpty()))
	})
})
