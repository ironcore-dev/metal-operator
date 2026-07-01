// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("ServerReadinessRuleServer Controller", func() {
	SetupTest(nil)

	Context("Continuous Mode", func() {
		It("should add and remove the taint then the server matches", func(ctx SpecContext) {
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
			DeferCleanup(k8sClient.Delete, rule)

			By("waiting for the server to be tainted as it does not have the condition")
			Eventually(Object(server)).Should(HaveField("Spec.Taints", Equal([]metalv1alpha1.Taint{
				{Key: "SomeTaint", Value: "SomeValue", Effect: metalv1alpha1.TaintEffectNoBind},
			})))

			By("setting the condition")
			Eventually(UpdateStatus(server, func() {
				server.Status.Conditions = append(server.Status.Conditions, metav1.Condition{
					Type:               "CustomType",
					Status:             metav1.ConditionTrue,
					Reason:             "CustomReason",
					Message:            "CustomMessage",
					LastTransitionTime: metav1.Now(),
				})
			})).Should(Succeed())

			By("waiting for the server to loose the taint")
			Eventually(Object(server)).Should(HaveField("Spec.Taints", BeEmpty()))

			By("setting the condition to false")
			Eventually(UpdateStatus(server, func() {
				server.Status.Conditions = append(server.Status.Conditions, metav1.Condition{
					Type:               "CustomType",
					Status:             metav1.ConditionFalse,
					Reason:             "CustomReason",
					Message:            "CustomMessage",
					LastTransitionTime: metav1.Now(),
				})
			})).Should(Succeed())

			By("waiting for the server to be tainted again")
			Eventually(Object(server)).Should(HaveField("Spec.Taints", Equal([]metalv1alpha1.Taint{
				{Key: "SomeTaint", Value: "SomeValue", Effect: metalv1alpha1.TaintEffectNoBind},
			})))
		})
	})

	Context("BootstrapOnly Mode", func() {
		It("should add and remove the taint then the server matches", func(ctx SpecContext) {
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
					EnforcementMode: metalv1alpha1.EnforcementModeBootstrapOnly,
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
			DeferCleanup(k8sClient.Delete, rule)

			By("waiting for the server to be tainted as it does not have the condition")
			Eventually(Object(server)).Should(HaveField("Spec.Taints", Equal([]metalv1alpha1.Taint{
				{Key: "SomeTaint", Value: "SomeValue", Effect: metalv1alpha1.TaintEffectNoBind},
			})))

			By("setting the condition")
			Eventually(UpdateStatus(server, func() {
				server.Status.Conditions = append(server.Status.Conditions, metav1.Condition{
					Type:               "CustomType",
					Status:             metav1.ConditionTrue,
					Reason:             "CustomReason",
					Message:            "CustomMessage",
					LastTransitionTime: metav1.Now(),
				})
			})).Should(Succeed())

			By("waiting for the server to loose the taint and have the annotation")
			Eventually(Object(server)).Should(SatisfyAll(
				HaveField("Annotations", HaveKeyWithValue(serverReadinessRuleBootstrapCompletedAnnotationPrefix+rule.Name, "true")),
				HaveField("Spec.Taints", BeEmpty()),
			))

			By("setting the condition to false")
			Eventually(UpdateStatus(server, func() {
				server.Status.Conditions = append(server.Status.Conditions, metav1.Condition{
					Type:               "CustomType",
					Status:             metav1.ConditionFalse,
					Reason:             "CustomReason",
					Message:            "CustomMessage",
					LastTransitionTime: metav1.Now(),
				})
			})).Should(Succeed())

			By("asserting the server does not get tainted again")
			Consistently(Object(server)).Should(HaveField("Spec.Taints", BeEmpty()))
		})
	})
})
