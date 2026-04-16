// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("Variable templating", func() {

	// ─── substituteVars ────────────────────────────────────────────────────────

	Describe("substituteVars", func() {
		It("replaces a single placeholder", func() {
			Expect(substituteVars("hello $(NAME)", map[string]string{"NAME": "world"})).
				To(Equal("hello world"))
		})

		It("replaces multiple distinct placeholders in one string", func() {
			Expect(substituteVars("$(A)-$(B)", map[string]string{"A": "foo", "B": "bar"})).
				To(Equal("foo-bar"))
		})

		It("replaces the same placeholder appearing multiple times", func() {
			Expect(substituteVars("$(X)/$(X)", map[string]string{"X": "v"})).
				To(Equal("v/v"))
		})

		It("leaves unknown placeholders unchanged", func() {
			Expect(substituteVars("$(UNKNOWN)", map[string]string{"NAME": "world"})).
				To(Equal("$(UNKNOWN)"))
		})

		It("leaves the string unchanged when resolved map is nil", func() {
			Expect(substituteVars("$(NAME)", nil)).To(Equal("$(NAME)"))
		})

		It("leaves the string unchanged when resolved map is empty", func() {
			Expect(substituteVars("$(NAME)", map[string]string{})).To(Equal("$(NAME)"))
		})

		It("leaves the string unchanged when there are no placeholders", func() {
			Expect(substituteVars("plain-value", map[string]string{"X": "y"})).
				To(Equal("plain-value"))
		})

		// ── escape sequences ──────────────────────────────────────────────────

		It("$$(KEY) produces a literal $(KEY) — not expanded", func() {
			Expect(substituteVars("$$(NAME)", map[string]string{"NAME": "world"})).
				To(Equal("$(NAME)"))
		})

		It("mixes escaped and unescaped placeholders in one string", func() {
			// $(A) → expanded, $$(A) → literal $(A)
			Expect(substituteVars("$(A) and $$(A)", map[string]string{"A": "val"})).
				To(Equal("val and $(A)"))
		})

		It("handles multiple escaped placeholders for different keys", func() {
			Expect(substituteVars("$$(X) $$(Y)", map[string]string{"X": "1", "Y": "2"})).
				To(Equal("$(X) $(Y)"))
		})

		// ── special characters in values ──────────────────────────────────────

		It("works when the resolved value contains JSON-like characters", func() {
			// Simulates: key: "$(FOO)" where FOO = `{"a":1}`
			Expect(substituteVars("$(FOO)", map[string]string{"FOO": `{"a":1}`})).
				To(Equal(`{"a":1}`))
		})

		It("works when the resolved value contains quotes and colons", func() {
			Expect(substituteVars(`data: "$(VAL)"`, map[string]string{"VAL": `v`})).
				To(Equal(`data: "v"`))
		})

		It("works when the input string contains quotes around the placeholder", func() {
			Expect(substituteVars(`"$(KEY)"`, map[string]string{"KEY": "value"})).
				To(Equal(`"value"`))
		})

		It("works when value contains a dollar sign not part of a placeholder", func() {
			Expect(substituteVars("price: $100 and $(ITEM)", map[string]string{"ITEM": "apple"})).
				To(Equal("price: $100 and apple"))
		})
	})

	// ─── ApplyVariables ────────────────────────────────────────────────────────

	Describe("ApplyVariables", func() {
		It("returns the original map (same pointer) when resolved is nil", func() {
			orig := map[string]string{"k": "$(X)"}
			Expect(ApplyVariables(orig, nil)).To(BeIdenticalTo(orig))
		})

		It("returns the original map (same pointer) when resolved is empty", func() {
			orig := map[string]string{"k": "$(X)"}
			Expect(ApplyVariables(orig, map[string]string{})).To(BeIdenticalTo(orig))
		})

		It("substitutes placeholders across all map values", func() {
			result := ApplyVariables(
				map[string]string{"k1": "$(X)", "k2": "prefix-$(Y)"},
				map[string]string{"X": "val1", "Y": "val2"},
			)
			Expect(result).To(Equal(map[string]string{"k1": "val1", "k2": "prefix-val2"}))
		})

		It("leaves values with no placeholders untouched", func() {
			result := ApplyVariables(
				map[string]string{"k": "static"},
				map[string]string{"X": "v"},
			)
			Expect(result).To(HaveKeyWithValue("k", "static"))
		})

		It("does not modify map keys even if they look like placeholders", func() {
			result := ApplyVariables(
				map[string]string{"$(KEY)": "value"},
				map[string]string{"KEY": "replaced"},
			)
			Expect(result).To(HaveKeyWithValue("$(KEY)", "value"))
		})

		It("does not mutate the original settingsMap", func() {
			orig := map[string]string{"k": "$(X)"}
			ApplyVariables(orig, map[string]string{"X": "replaced"})
			Expect(orig["k"]).To(Equal("$(X)"))
		})

		It("honours escape sequences in values", func() {
			result := ApplyVariables(
				map[string]string{"k": "$$(X) and $(X)"},
				map[string]string{"X": "real"},
			)
			Expect(result).To(HaveKeyWithValue("k", "$(X) and real"))
		})
	})

	// ─── resolveFieldRef ───────────────────────────────────────────────────────

	Describe("resolveFieldRef", func() {
		It("resolves a deeply nested string field (spec.BMCRef.name)", func() {
			obj := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCRef: &corev1.LocalObjectReference{Name: "my-bmc"},
				},
			}
			val, err := resolveFieldRef(obj, "spec.BMCRef.name")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("my-bmc"))
		})

		It("resolves metadata.name", func() {
			obj := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{Name: "my-settings"},
			}
			val, err := resolveFieldRef(obj, "metadata.name")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("my-settings"))
		})

		It("resolves metadata.namespace", func() {
			obj := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "my-ns"},
			}
			val, err := resolveFieldRef(obj, "metadata.namespace")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("my-ns"))
		})

		It("returns an error when the fieldPath does not exist", func() {
			obj := &metalv1alpha1.BMCSettings{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
			_, err := resolveFieldRef(obj, "spec.nonexistent.field")
			Expect(err).To(HaveOccurred())
		})

		It("returns an error for an empty fieldPath", func() {
			obj := &metalv1alpha1.BMCSettings{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
			_, err := resolveFieldRef(obj, "")
			Expect(err).To(HaveOccurred())
		})
	})

	// ─── ResolveVariables (integration — requires k8s API) ────────────────────

	Describe("ResolveVariables", func() {
		ns := SetupTest(nil)

		owner := func(name string) *metalv1alpha1.BMCSettings {
			return &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCRef: &corev1.LocalObjectReference{Name: name + "-bmc"},
				},
			}
		}

		It("resolves a variable from a Secret key", func(ctx SpecContext) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, GenerateName: "rv-sec-"},
				Data:       map[string][]byte{"pass": []byte("s3cr3t")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, secret)

			resolved, err := ResolveVariables(ctx, k8sClient, owner("o1"), []metalv1alpha1.Variable{
				{Key: "PASS", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					SecretKeyRef: &metalv1alpha1.NamespacedKeySelector{
						Name: secret.Name, Namespace: ns.Name, Key: "pass",
					},
				}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(HaveKeyWithValue("PASS", "s3cr3t"))
		})

		It("resolves a variable from a ConfigMap key", func(ctx SpecContext) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, GenerateName: "rv-cm-"},
				Data:       map[string]string{"domain": "example.com"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			DeferCleanup(k8sClient.Delete, cm)

			resolved, err := ResolveVariables(ctx, k8sClient, owner("o2"), []metalv1alpha1.Variable{
				{Key: "DOMAIN", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					ConfigMapKeyRef: &metalv1alpha1.NamespacedKeySelector{
						Name: cm.Name, Namespace: ns.Name, Key: "domain",
					},
				}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(HaveKeyWithValue("DOMAIN", "example.com"))
		})

		It("resolves a variable from a fieldRef on the owner object", func(ctx SpecContext) {
			o := owner("my-bmc-settings")
			resolved, err := ResolveVariables(ctx, k8sClient, o, []metalv1alpha1.Variable{
				{Key: "HOST", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					FieldRef: &metalv1alpha1.FieldRefSelector{FieldPath: "spec.BMCRef.name"},
				}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(HaveKeyWithValue("HOST", "my-bmc-settings-bmc"))
		})

		It("chains variables: later variable value contains $(EARLIER) placeholder", func(ctx SpecContext) {
			// HOST comes from a Secret; URL from a ConfigMap whose value is "http://$(HOST)/api"
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, GenerateName: "rv-chain-sec-"},
				Data:       map[string][]byte{"host": []byte("192.168.1.1")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, secret)

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, GenerateName: "rv-chain-cm-"},
				Data:       map[string]string{"url": "http://$(HOST)/api"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			DeferCleanup(k8sClient.Delete, cm)

			resolved, err := ResolveVariables(ctx, k8sClient, owner("chain"), []metalv1alpha1.Variable{
				{Key: "HOST", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					SecretKeyRef: &metalv1alpha1.NamespacedKeySelector{
						Name: secret.Name, Namespace: ns.Name, Key: "host",
					},
				}},
				{Key: "URL", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					ConfigMapKeyRef: &metalv1alpha1.NamespacedKeySelector{
						Name: cm.Name, Namespace: ns.Name, Key: "url",
					},
				}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved["HOST"]).To(Equal("192.168.1.1"))
			Expect(resolved["URL"]).To(Equal("http://192.168.1.1/api"))
		})

		It("chains variables: Secret key name itself contains $(EARLIER) placeholder", func(ctx SpecContext) {
			// Mirrors the sample YAML: secretKeyRef.key = "$(BmcName)"
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, GenerateName: "rv-dynkey-sec-"},
				// The key is the resolved value of BmcName
				Data: map[string][]byte{"endpoint-sample": []byte("license-abc-123")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, secret)

			o := &metalv1alpha1.BMCSettings{
				ObjectMeta: metav1.ObjectMeta{Name: "dynkey-owner"},
				Spec: metalv1alpha1.BMCSettingsSpec{
					BMCRef: &corev1.LocalObjectReference{Name: "endpoint-sample"},
				},
			}

			resolved, err := ResolveVariables(ctx, k8sClient, o, []metalv1alpha1.Variable{
				{Key: "BmcName", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					FieldRef: &metalv1alpha1.FieldRefSelector{FieldPath: "spec.BMCRef.name"},
				}},
				{Key: "LicenseKey", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					SecretKeyRef: &metalv1alpha1.NamespacedKeySelector{
						Name:      secret.Name,
						Namespace: ns.Name,
						Key:       "$(BmcName)", // dynamic key — expanded to "endpoint-sample"
					},
				}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved["BmcName"]).To(Equal("endpoint-sample"))
			Expect(resolved["LicenseKey"]).To(Equal("license-abc-123"))
		})

		It("returns an error when the Secret does not exist", func(ctx SpecContext) {
			_, err := ResolveVariables(ctx, k8sClient, owner("err1"), []metalv1alpha1.Variable{
				{Key: "X", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					SecretKeyRef: &metalv1alpha1.NamespacedKeySelector{
						Name: "no-such-secret", Namespace: ns.Name, Key: "k",
					},
				}},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no-such-secret"))
		})

		It("returns an error when the Secret key does not exist", func(ctx SpecContext) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, GenerateName: "rv-missingkey-"},
				Data:       map[string][]byte{"present": []byte("v")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, secret)

			_, err := ResolveVariables(ctx, k8sClient, owner("err2"), []metalv1alpha1.Variable{
				{Key: "X", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					SecretKeyRef: &metalv1alpha1.NamespacedKeySelector{
						Name: secret.Name, Namespace: ns.Name, Key: "missing",
					},
				}},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing"))
		})

		It("returns an error when the ConfigMap does not exist", func(ctx SpecContext) {
			_, err := ResolveVariables(ctx, k8sClient, owner("err3"), []metalv1alpha1.Variable{
				{Key: "X", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					ConfigMapKeyRef: &metalv1alpha1.NamespacedKeySelector{
						Name: "no-such-cm", Namespace: ns.Name, Key: "k",
					},
				}},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no-such-cm"))
		})

		It("returns an error when the fieldRef path does not exist on the owner", func(ctx SpecContext) {
			_, err := ResolveVariables(ctx, k8sClient, owner("err4"), []metalv1alpha1.Variable{
				{Key: "X", ValueFrom: &metalv1alpha1.VariableSourceValueFrom{
					FieldRef: &metalv1alpha1.FieldRefSelector{FieldPath: "spec.nonexistent.path"},
				}},
			})
			Expect(err).To(HaveOccurred())
		})

		It("returns an empty map for an empty variable list", func(ctx SpecContext) {
			resolved, err := ResolveVariables(ctx, k8sClient, owner("empty"), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(BeEmpty())
		})
	})
})
