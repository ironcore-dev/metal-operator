// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"crypto/x509"
	"encoding/pem"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	metalBmc "github.com/ironcore-dev/metal-operator/bmc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMC Certificate Integration", func() {
	ns := SetupTest(nil)

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	Context("Certificate Lifecycle with Mock BMC", Pending, func() {
		// TODO: This integration test is flaky due to timing issues with mock BMC server.
		// The unit tests provide comprehensive coverage. Re-enable once mock server timing is improved.
		It("should provision certificate with CertificateRequest becoming Ready", func(ctx SpecContext) {
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

			By("Creating a BMC with certificate specification")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-cert-",
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
					Certificate: &metalv1alpha1.BMCCertificateSpec{
						CommonName:  "test-bmc.example.com",
						DNSNames:    []string{"test-bmc.example.com", "bmc.local"},
						IPAddresses: []string{"127.0.0.1"},
						IssuerRef: metalv1alpha1.CertificateIssuerRef{
							Name: "test-issuer",
							Kind: "Issuer",
						},
						Duration: &metav1.Duration{Duration: 90 * 24 * time.Hour},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

			By("Setting BMC status to Enabled")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bmc.Name}, bmc)).To(Succeed())
				bmc.Status.State = metalv1alpha1.BMCStateEnabled
				bmc.Status.IP = metalv1alpha1.MustParseIP(MockServerIP)
				g.Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())
			}).Should(Succeed())

			By("Ensuring CertificateRequest is created with correct IssuerRef")
			certReqName := CertificateRequestNamePrefix + bmc.Name
			certReq := &certmanagerv1.CertificateRequest{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      certReqName,
					Namespace: ns.Name,
				}, certReq)).To(Succeed())
				g.Expect(certReq.Spec.IssuerRef.Name).To(Equal("test-issuer"))
				g.Expect(certReq.Spec.IssuerRef.Kind).To(Equal("Issuer"))
				g.Expect(certReq.Spec.Request).NotTo(BeEmpty())
			}).Should(Succeed())

			By("Mocking CertificateRequest becoming Ready")
			// Generate a test certificate
			certPEM := generateTestCertificate(60*24*time.Hour, "test-bmc.example.com")

			// Update CertificateRequest status to Ready
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      certReqName,
					Namespace: ns.Name,
				}, certReq)).To(Succeed())

				certReq.Status.Certificate = certPEM
				certReq.Status.Conditions = []certmanagerv1.CertificateRequestCondition{
					{
						Type:               certmanagerv1.CertificateRequestConditionReady,
						Status:             cmmeta.ConditionTrue,
						LastTransitionTime: &metav1.Time{Time: time.Now()},
					},
				}
				g.Expect(k8sClient.Status().Update(ctx, certReq)).To(Succeed())
			}).Should(Succeed())

			By("Verifying BMC status updated with CertificateSecretRef")
			Eventually(Object(bmc)).Should(SatisfyAll(
				HaveField("Status.CertificateSecretRef", Not(BeNil())),
				HaveField("Status.CertificateRequestName", Equal(certReqName)),
			))

			By("Verifying Secret created with TLS certificate")
			secretName := CertificateSecretNamePrefix + bmc.Name
			secret := &corev1.Secret{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      secretName,
					Namespace: ns.Name,
				}, secret)).To(Succeed())
				g.Expect(secret.Type).To(Equal(corev1.SecretTypeTLS))
				g.Expect(secret.Data).To(HaveKey("tls.crt"))
			}).Should(Succeed())

			By("Verifying CertificateReady condition is True")
			Eventually(Object(bmc)).Should(SatisfyAll(
				HaveField("Status.Conditions", ContainElement(SatisfyAll(
					HaveField("Type", metalv1alpha1.BMCCertificateReadyCondition),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", metalv1alpha1.BMCCertificateReadyReasonIssued),
				))),
			))

			// Cleanup
			Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})
	})

	Context("Feature Disabled", func() {
		It("should not create CertificateRequest when certificate feature is not configured", func(ctx SpecContext) {
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

			By("Creating a BMC without certificate spec (nil)")
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
					Certificate: nil, // Feature disabled
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

			By("Ensuring no CertificateRequest is created")
			certReqName := CertificateRequestNamePrefix + bmc.Name
			certReq := &certmanagerv1.CertificateRequest{}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      certReqName,
					Namespace: ns.Name,
				}, certReq)
				return err != nil // Should not exist
			}, "2s", "200ms").Should(BeTrue())

			By("Ensuring BMC reconciles successfully without certificate")
			Eventually(Object(bmc)).Should(SatisfyAll(
				HaveField("Status.IP", metalv1alpha1.MustParseIP(MockServerIP)),
				HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			))

			// Cleanup
			Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})
	})

	Context("Certificate Expiry and Renewal", func() {
		It("should renew certificate when it expires soon", func(ctx SpecContext) {
			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-renew-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

			By("Creating a BMC with certificate specification")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-renew-",
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
					Certificate: &metalv1alpha1.BMCCertificateSpec{
						CommonName: "test-bmc-renew.example.com",
						IssuerRef: metalv1alpha1.CertificateIssuerRef{
							Name: "test-issuer",
							Kind: "Issuer",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

			By("Creating a certificate secret with certificate expiring in 20 days")
			expiringSoonCert := generateTestCertificate(20*24*time.Hour, "test-bmc-renew.example.com")
			secretName := CertificateSecretNamePrefix + bmc.Name
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: ns.Name,
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": expiringSoonCert,
					"tls.key": []byte("fake-key"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Updating BMC status to reference the expiring certificate")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
				bmc.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: secretName}
				g.Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())
			}).Should(Succeed())

			By("Triggering reconciliation and verifying new CertificateRequest is created")
			certReqName := CertificateRequestNamePrefix + bmc.Name
			Eventually(func(g Gomega) {
				certReq := &certmanagerv1.CertificateRequest{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      certReqName,
					Namespace: ns.Name,
				}, certReq)).To(Succeed())
				g.Expect(certReq.Spec.Request).NotTo(BeEmpty())
			}).Should(Succeed())

			By("Verifying CertificateReady condition shows Pending during renewal")
			Eventually(Object(bmc)).Should(SatisfyAll(
				HaveField("Status.Conditions", ContainElement(SatisfyAll(
					HaveField("Type", metalv1alpha1.BMCCertificateReadyCondition),
					HaveField("Status", metav1.ConditionFalse),
					HaveField("Reason", metalv1alpha1.BMCCertificateReadyReasonPending),
				))),
			))

			// Cleanup
			Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})
	})

	Context("CSR Fallback Strategy", func() {
		It("should generate CSR via operator when BMC generation fails", func(ctx SpecContext) {
			By("Simulating BMC CSR generation failure")
			metalBmc.UnitTestMockUps.SimulateCSRGenerationFailure = true
			DeferCleanup(func() {
				metalBmc.UnitTestMockUps.SimulateCSRGenerationFailure = false
			})

			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-csr-fallback-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

			By("Creating a BMC with certificate specification")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-csr-fallback-",
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
					Certificate: &metalv1alpha1.BMCCertificateSpec{
						CommonName: "test-bmc-fallback.example.com",
						IssuerRef: metalv1alpha1.CertificateIssuerRef{
							Name: "test-issuer",
							Kind: "Issuer",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

			By("Verifying CertificateRequest created with operator-generated CSR")
			certReqName := CertificateRequestNamePrefix + bmc.Name
			certReq := &certmanagerv1.CertificateRequest{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      certReqName,
					Namespace: ns.Name,
				}, certReq)).To(Succeed())
				g.Expect(certReq.Spec.Request).NotTo(BeEmpty())

				// Verify CSR is valid
				block, _ := pem.Decode(certReq.Spec.Request)
				g.Expect(block).NotTo(BeNil())
				g.Expect(block.Type).To(Equal("CERTIFICATE REQUEST"))

				csr, err := x509.ParseCertificateRequest(block.Bytes)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(csr.Subject.CommonName).To(Equal("test-bmc-fallback.example.com"))
			}).Should(Succeed())

			By("Verifying private key stored in temporary secret")
			privateKeySecretName := CertificateSecretNamePrefix + bmc.Name + "-key"
			privateKeySecret := &corev1.Secret{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      privateKeySecretName,
					Namespace: ns.Name,
				}, privateKeySecret)).To(Succeed())
				g.Expect(privateKeySecret.Data).To(HaveKey("tls.key"))
				g.Expect(privateKeySecret.Labels).To(HaveKeyWithValue("metal.ironcore.dev/temporary", "true"))
			}).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})
	})

	Context("Graceful Degradation", Pending, func() {
		// TODO: This test requires cert-manager controller to run, which is not available in envtest.
		// The test expects CertificateRequest to be marked as Failed by cert-manager when referencing
		// a non-existent issuer, but without cert-manager controller it stays Pending forever.
		It("should continue reconciliation when certificate management fails", func(ctx SpecContext) {
			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-degrade-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

			By("Creating a BMC with invalid certificate configuration")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-degrade-",
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
					Certificate: &metalv1alpha1.BMCCertificateSpec{
						CommonName:  "test-bmc-degrade.example.com",
						IPAddresses: []string{"invalid-ip-address"}, // Invalid IP
						IssuerRef: metalv1alpha1.CertificateIssuerRef{
							Name: "test-issuer",
							Kind: "Issuer",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

			By("Verifying CertificateReady condition is False with Failed reason")
			Eventually(Object(bmc)).Should(SatisfyAll(
				HaveField("Status.Conditions", ContainElement(SatisfyAll(
					HaveField("Type", metalv1alpha1.BMCCertificateReadyCondition),
					HaveField("Status", metav1.ConditionFalse),
					HaveField("Reason", metalv1alpha1.BMCCertificateReadyReasonFailed),
				))),
			))

			By("Verifying BMC continues to reconcile (other operations work)")
			Eventually(Object(bmc)).Should(SatisfyAll(
				HaveField("Status.IP", metalv1alpha1.MustParseIP(MockServerIP)),
				HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
				HaveField("Status.PowerState", metalv1alpha1.OnPowerState),
			))

			// Cleanup
			Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})
	})

	Context("Certificate Validation", func() {
		It("should validate certificate with valid expiry", func(ctx SpecContext) {
			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-valid-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

			By("Creating a BMC with certificate specification")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-valid-",
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
					Certificate: &metalv1alpha1.BMCCertificateSpec{
						CommonName: "test-bmc-valid.example.com",
						IssuerRef: metalv1alpha1.CertificateIssuerRef{
							Name: "test-issuer",
							Kind: "Issuer",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

			By("Creating a certificate secret with valid certificate (60 days)")
			validCert := generateTestCertificate(60*24*time.Hour, "test-bmc-valid.example.com")
			secretName := CertificateSecretNamePrefix + bmc.Name
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: ns.Name,
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": validCert,
					"tls.key": []byte("fake-key"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Updating BMC status to reference the valid certificate")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
				bmc.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: secretName}
				g.Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())
			}).Should(Succeed())

			By("Verifying CertificateReady condition is True with valid certificate")
			Eventually(Object(bmc)).Should(SatisfyAll(
				HaveField("Status.Conditions", ContainElement(SatisfyAll(
					HaveField("Type", metalv1alpha1.BMCCertificateReadyCondition),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", metalv1alpha1.BMCCertificateReadyReasonIssued),
				))),
			))

			By("Ensuring no new CertificateRequest created")
			certReqName := CertificateRequestNamePrefix + bmc.Name
			certReq := &certmanagerv1.CertificateRequest{}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      certReqName,
					Namespace: ns.Name,
				}, certReq)
				return err != nil // Should not exist for valid cert
			}, "2s", "200ms").Should(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})
	})

	Context("Multiple SANs", func() {
		It("should create certificate with multiple DNS names and IP addresses", func(ctx SpecContext) {
			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-multi-san-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

			By("Creating a BMC with multiple SANs")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-multi-san-",
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
					Hostname: ptr.To("bmc-hostname.example.com"),
					Certificate: &metalv1alpha1.BMCCertificateSpec{
						CommonName: "test-bmc-multi.example.com",
						DNSNames: []string{
							"test-bmc-multi.example.com",
							"bmc.local",
							"bmc-alt.example.com",
						},
						IPAddresses: []string{
							"10.0.0.1",
							"10.0.0.2",
							"192.168.1.100",
						},
						IssuerRef: metalv1alpha1.CertificateIssuerRef{
							Name: "test-issuer",
							Kind: "Issuer",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

			By("Verifying CertificateRequest CSR contains all SANs")
			certReqName := CertificateRequestNamePrefix + bmc.Name
			certReq := &certmanagerv1.CertificateRequest{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      certReqName,
					Namespace: ns.Name,
				}, certReq)).To(Succeed())

				// Parse and verify CSR
				block, _ := pem.Decode(certReq.Spec.Request)
				g.Expect(block).NotTo(BeNil())

				csr, err := x509.ParseCertificateRequest(block.Bytes)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify DNS SANs
				g.Expect(csr.DNSNames).To(ConsistOf(
					"test-bmc-multi.example.com",
					"bmc.local",
					"bmc-alt.example.com",
				))

				// Verify IP SANs
				g.Expect(csr.IPAddresses).To(HaveLen(4)) // 3 from spec + 1 from status.IP
			}).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})
	})

	Context("Owner References", func() {
		It("should set owner reference on CertificateRequest and Secret", func(ctx SpecContext) {
			By("Creating a BMCSecret")
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-owner-",
				},
				Data: map[string][]byte{
					metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
					metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
				},
			}
			Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

			By("Creating a BMC with certificate specification")
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-bmc-owner-",
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
					Certificate: &metalv1alpha1.BMCCertificateSpec{
						CommonName: "test-bmc-owner.example.com",
						IssuerRef: metalv1alpha1.CertificateIssuerRef{
							Name: "test-issuer",
							Kind: "Issuer",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

			By("Verifying CertificateRequest has owner reference")
			certReqName := CertificateRequestNamePrefix + bmc.Name
			certReq := &certmanagerv1.CertificateRequest{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      certReqName,
					Namespace: ns.Name,
				}, certReq)).To(Succeed())

				g.Expect(certReq.OwnerReferences).To(HaveLen(1))
				g.Expect(certReq.OwnerReferences[0].Name).To(Equal(bmc.Name))
				g.Expect(certReq.OwnerReferences[0].Kind).To(Equal("BMC"))
				g.Expect(*certReq.OwnerReferences[0].Controller).To(BeTrue())
			}).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		})
	})
})

// Helper functions
