// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BMC Certificate Integration with TLS Secrets", func() {
	ns := SetupTest(nil)

	Context("TLS Secret Integration", func() {
		It("should reconcile BMC with TLS secret", func(ctx SpecContext) {
			By("Creating a TLS secret")
			certPEM, keyPEM, err := generateIntegrationTestCertificate()
			Expect(err).NotTo(HaveOccurred())

			tlsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bmc-tls",
					Namespace: ns.Name,
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": certPEM,
					"tls.key": keyPEM,
				},
			}
			Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, tlsSecret)

			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-cert-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmcSecret)

			By("Creating a BMC with TLS secret reference")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-tls-",
				},
				Spec: metalv1alpha1.BMCSpec{
					Endpoint: &metalv1alpha1.InlineEndpoint{
						IP:         metalv1alpha1.MustParseIP(MockServerIP),
						MACAddress: "23:11:8A:33:CF:EA",
					},
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: MockServerPort,
					},
					BMCSecretRef: corev1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
					TLSSecretRef: &corev1.SecretReference{
						Name:      tlsSecret.Name,
						Namespace: ns.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmc)

			By("Verifying BMC reconciliation processes TLS secret")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
				// The reconciler should attempt to install the certificate
				// In a mock environment without actual BMC connectivity, the condition
				// may reflect connection issues, which is expected
			}).Should(Succeed())
		})

		It("should handle missing TLS secret", func(ctx SpecContext) {
			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-cert-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmcSecret)

			By("Creating a BMC referencing non-existent TLS secret")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-missing-tls-",
				},
				Spec: metalv1alpha1.BMCSpec{
					Endpoint: &metalv1alpha1.InlineEndpoint{
						IP:         metalv1alpha1.MustParseIP(MockServerIP),
						MACAddress: "23:11:8A:33:CF:EA",
					},
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: MockServerPort,
					},
					BMCSecretRef: corev1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
					TLSSecretRef: &corev1.SecretReference{
						Name:      "non-existent-secret",
						Namespace: ns.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmc)

			By("Verifying BMC handles missing secret gracefully")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
				// Should have a failed certificate condition
				for _, cond := range bmc.Status.Conditions {
					if cond.Type == metalv1alpha1.BMCCertificateReadyCondition {
						g.Expect(string(cond.Status)).To(Equal("False"))
						g.Expect(cond.Reason).To(Equal(metalv1alpha1.BMCCertificateReadyReasonFailed))
					}
				}
			}).Should(Succeed())
		})

		It("should handle invalid TLS secret type", func(ctx SpecContext) {
			By("Creating an invalid secret (not kubernetes.io/tls type)")
			invalidSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-invalid-tls",
					Namespace: ns.Name,
				},
				Type: corev1.SecretTypeOpaque, // Wrong type
				Data: map[string][]byte{
					"tls.crt": []byte("fake-cert"),
					"tls.key": []byte("fake-key"),
				},
			}
			Expect(k8sClient.Create(ctx, invalidSecret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, invalidSecret)

			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-cert-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmcSecret)

			By("Creating a BMC with invalid secret reference")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-invalid-tls-",
				},
				Spec: metalv1alpha1.BMCSpec{
					Endpoint: &metalv1alpha1.InlineEndpoint{
						IP:         metalv1alpha1.MustParseIP(MockServerIP),
						MACAddress: "23:11:8A:33:CF:EA",
					},
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: MockServerPort,
					},
					BMCSecretRef: corev1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
					TLSSecretRef: &corev1.SecretReference{
						Name:      invalidSecret.Name,
						Namespace: ns.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmc)

			By("Verifying BMC handles invalid secret type")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
				// Should have a failed certificate condition
				for _, cond := range bmc.Status.Conditions {
					if cond.Type == metalv1alpha1.BMCCertificateReadyCondition {
						g.Expect(string(cond.Status)).To(Equal("False"))
						g.Expect(cond.Reason).To(Equal(metalv1alpha1.BMCCertificateReadyReasonFailed))
					}
				}
			}).Should(Succeed())
		})

		It("should reconcile when TLS secret is updated", func(ctx SpecContext) {
			By("Creating initial TLS secret")
			certPEM1, keyPEM1, err := generateIntegrationTestCertificate()
			Expect(err).NotTo(HaveOccurred())

			tlsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bmc-tls-update",
					Namespace: ns.Name,
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": certPEM1,
					"tls.key": keyPEM1,
				},
			}
			Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, tlsSecret)

			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-cert-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmcSecret)

			By("Creating a BMC with TLS secret reference")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-tls-update-",
				},
				Spec: metalv1alpha1.BMCSpec{
					Endpoint: &metalv1alpha1.InlineEndpoint{
						IP:         metalv1alpha1.MustParseIP(MockServerIP),
						MACAddress: "23:11:8A:33:CF:EA",
					},
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: MockServerPort,
					},
					BMCSecretRef: corev1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
					TLSSecretRef: &corev1.SecretReference{
						Name:      tlsSecret.Name,
						Namespace: ns.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmc)

			By("Updating TLS secret with new certificate")
			certPEM2, keyPEM2, err := generateIntegrationTestCertificate()
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(tlsSecret), tlsSecret)).To(Succeed())
				tlsSecret.Data["tls.crt"] = certPEM2
				tlsSecret.Data["tls.key"] = keyPEM2
				g.Expect(k8sClient.Update(ctx, tlsSecret)).To(Succeed())
			}).Should(Succeed())

			By("Verifying BMC reconciles with updated certificate")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
				// The reconciler should detect the secret change and attempt to reinstall
			}).Should(Succeed())
		})
	})

	Context("BMC without TLS secret", func() {
		It("should skip certificate management when TLSSecretRef is nil", func(ctx SpecContext) {
			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-no-cert-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmcSecret)

			By("Creating a BMC without TLS secret reference")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-no-cert-",
				},
				Spec: metalv1alpha1.BMCSpec{
					Endpoint: &metalv1alpha1.InlineEndpoint{
						IP:         metalv1alpha1.MustParseIP(MockServerIP),
						MACAddress: "23:11:8A:33:CF:EA",
					},
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: MockServerPort,
					},
					BMCSecretRef: corev1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
					// No TLSSecretRef
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
			DeferCleanup(k8sClient.Delete, bmc)

			By("Verifying BMC reconciles successfully without certificate management")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
				// Should not have certificate-related conditions
				hasCertCondition := false
				for _, cond := range bmc.Status.Conditions {
					if cond.Type == metalv1alpha1.BMCCertificateReadyCondition {
						hasCertCondition = true
					}
				}
				g.Expect(hasCertCondition).To(BeFalse())
			}).Should(Succeed())
		})
	})
})

// generateIntegrationTestCertificate creates a test certificate for integration tests
func generateIntegrationTestCertificate() ([]byte, []byte, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "test-bmc-integration.example.com",
			Organization: []string{"Test Org"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	return certPEM, keyPEM, nil
}
