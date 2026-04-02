// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("BMC Certificate Management", func() {
	var (
		ctx        context.Context
		reconciler *BMCReconciler
		k8sClient  client.Client
		bmcObj     *metalv1alpha1.BMC
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(metalv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(certmanagerv1.AddToScheme(scheme)).To(Succeed())

		// Create BMC object with certificate configuration
		bmcObj = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc",
			},
			Spec: metalv1alpha1.BMCSpec{
				Certificate: &metalv1alpha1.BMCCertificateSpec{
					CommonName:  "test-bmc.example.com",
					DNSNames:    []string{"test-bmc.example.com", "bmc.local"},
					IPAddresses: []string{"192.168.1.100"},
					IssuerRef: metalv1alpha1.CertificateIssuerRef{
						Name: "test-issuer",
						Kind: "Issuer",
					},
					Duration: &metav1.Duration{Duration: 90 * 24 * time.Hour}, // 90 days
				},
			},
			Status: metalv1alpha1.BMCStatus{
				IP: metalv1alpha1.MustParseIP("192.168.1.100"),
			},
		}

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(bmcObj).
			WithStatusSubresource(bmcObj).
			Build()

		reconciler = &BMCReconciler{
			Client:           k8sClient,
			Scheme:           scheme,
			ManagerNamespace: "default",
		}
	})

	Context("CSR Generation Tests", func() {
		It("should generate valid operator CSR with RSA 2048", func() {
			csrPEM, privateKey, err := reconciler.generateOperatorCSR(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(csrPEM).NotTo(BeEmpty())
			Expect(privateKey).NotTo(BeNil())

			// Verify private key is RSA 2048
			Expect(privateKey.N.BitLen()).To(Equal(DefaultRSAKeySize))

			// Parse CSR
			block, _ := pem.Decode(csrPEM)
			Expect(block).NotTo(BeNil())
			Expect(block.Type).To(Equal("CERTIFICATE REQUEST"))

			csr, err := x509.ParseCertificateRequest(block.Bytes)
			Expect(err).NotTo(HaveOccurred())

			// Verify CSR content
			Expect(csr.Subject.CommonName).To(Equal("test-bmc.example.com"))
			Expect(csr.DNSNames).To(ConsistOf("test-bmc.example.com", "bmc.local"))
			// Check IP address using semantic equality (IPv4 vs IPv6-mapped IPv4)
			expectedIP := net.ParseIP("192.168.1.100")
			Expect(csr.IPAddresses).To(HaveLen(1))
			Expect(csr.IPAddresses[0].Equal(expectedIP)).To(BeTrue(), "IP address should match 192.168.1.100")
			Expect(csr.SignatureAlgorithm).To(Equal(x509.SHA256WithRSA))
		})

		It("should generate valid CSR PEM encoding", func() {
			csrPEM, _, err := reconciler.generateOperatorCSR(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			// Verify PEM format
			block, rest := pem.Decode(csrPEM)
			Expect(block).NotTo(BeNil())
			Expect(rest).To(BeEmpty(), "should only have one PEM block")

			// Verify it can be parsed
			_, err = x509.ParseCertificateRequest(block.Bytes)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should include BMC endpoint IP in CSR IP addresses", func() {
			csrPEM, _, err := reconciler.generateOperatorCSR(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			block, _ := pem.Decode(csrPEM)
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			Expect(err).NotTo(HaveOccurred())

			// Should include both spec IP and status IP
			expectedIP := net.ParseIP("192.168.1.100")
			Expect(csr.IPAddresses).To(HaveLen(1))
			Expect(csr.IPAddresses[0].Equal(expectedIP)).To(BeTrue(), "IP address should match 192.168.1.100")
		})

		It("should use correct CommonName priority", func() {
			// Test: spec.certificate.commonName takes priority
			cn := reconciler.getCertificateCommonName(bmcObj)
			Expect(cn).To(Equal("test-bmc.example.com"))

			// Test: hostname fallback
			bmcWithHostname := bmcObj.DeepCopy()
			bmcWithHostname.Spec.Certificate.CommonName = ""
			bmcWithHostname.Spec.Hostname = ptr.To("hostname.local")
			cn = reconciler.getCertificateCommonName(bmcWithHostname)
			Expect(cn).To(Equal("hostname.local"))

			// Test: dnsNames fallback
			bmcWithDNS := bmcObj.DeepCopy()
			bmcWithDNS.Spec.Certificate.CommonName = ""
			bmcWithDNS.Spec.Hostname = nil
			cn = reconciler.getCertificateCommonName(bmcWithDNS)
			Expect(cn).To(Equal("test-bmc.example.com")) // First DNS name

			// Test: BMC name fallback
			bmcMinimal := bmcObj.DeepCopy()
			bmcMinimal.Spec.Certificate.CommonName = ""
			bmcMinimal.Spec.Hostname = nil
			bmcMinimal.Spec.Certificate.DNSNames = nil
			cn = reconciler.getCertificateCommonName(bmcMinimal)
			Expect(cn).To(Equal("test-bmc")) // BMC name
		})
	})

	Context("buildCSRParameters", func() {
		It("should convert spec to CSR parameters correctly", func() {
			params := reconciler.buildCSRParameters(bmcObj)

			Expect(params.CommonName).To(Equal("test-bmc.example.com"))
			Expect(params.KeyPairAlgorithm).To(Equal("RSA"))
			Expect(params.KeyBitLength).To(Equal(DefaultRSAKeySize))
			Expect(params.AlternativeNames).To(ConsistOf(
				"test-bmc.example.com",
				"bmc.local",
				"192.168.1.100",
			))
		})

		It("should handle empty DNS names and IP addresses", func() {
			bmcMinimal := bmcObj.DeepCopy()
			bmcMinimal.Spec.Certificate.DNSNames = nil
			bmcMinimal.Spec.Certificate.IPAddresses = nil

			params := reconciler.buildCSRParameters(bmcMinimal)

			Expect(params.CommonName).To(Equal("test-bmc.example.com"))
			Expect(params.AlternativeNames).To(BeEmpty())
		})
	})

	Context("CertificateRequest Creation", func() {
		It("should create CertificateRequest with correct IssuerRef", func() {
			csrPEM, privateKey, err := reconciler.generateOperatorCSR(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			certReq, err := reconciler.createOrGetCertificateRequest(ctx, bmcObj, csrPEM, privateKey)
			Expect(err).NotTo(HaveOccurred())
			Expect(certReq).NotTo(BeNil())

			// Verify IssuerRef
			Expect(certReq.Spec.IssuerRef.Name).To(Equal("test-issuer"))
			Expect(certReq.Spec.IssuerRef.Kind).To(Equal("Issuer"))
		})

		It("should create CertificateRequest with proper labels", func() {
			csrPEM, privateKey, err := reconciler.generateOperatorCSR(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			certReq, err := reconciler.createOrGetCertificateRequest(ctx, bmcObj, csrPEM, privateKey)
			Expect(err).NotTo(HaveOccurred())

			// Verify labels
			Expect(certReq.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "metal-operator"))
			Expect(certReq.Labels).To(HaveKeyWithValue("metal.ironcore.dev/bmc", "test-bmc"))
		})

		It("should set owner reference to BMC", func() {
			csrPEM, privateKey, err := reconciler.generateOperatorCSR(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			certReq, err := reconciler.createOrGetCertificateRequest(ctx, bmcObj, csrPEM, privateKey)
			Expect(err).NotTo(HaveOccurred())

			// Verify owner reference
			Expect(certReq.OwnerReferences).To(HaveLen(1))
			Expect(certReq.OwnerReferences[0].Name).To(Equal("test-bmc"))
			Expect(certReq.OwnerReferences[0].Kind).To(Equal("BMC"))
			Expect(*certReq.OwnerReferences[0].Controller).To(BeTrue())
		})

		It("should embed CSR in spec correctly", func() {
			csrPEM, privateKey, err := reconciler.generateOperatorCSR(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			certReq, err := reconciler.createOrGetCertificateRequest(ctx, bmcObj, csrPEM, privateKey)
			Expect(err).NotTo(HaveOccurred())

			// Verify CSR is embedded
			Expect(certReq.Spec.Request).To(Equal(csrPEM))

			// Verify it's valid PEM
			block, _ := pem.Decode(certReq.Spec.Request)
			Expect(block).NotTo(BeNil())
		})

		It("should set duration if specified", func() {
			csrPEM, privateKey, err := reconciler.generateOperatorCSR(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			certReq, err := reconciler.createOrGetCertificateRequest(ctx, bmcObj, csrPEM, privateKey)
			Expect(err).NotTo(HaveOccurred())

			// Verify duration
			Expect(certReq.Spec.Duration).NotTo(BeNil())
			Expect(certReq.Spec.Duration.Duration).To(Equal(90 * 24 * time.Hour))
		})

		It("should return existing CertificateRequest if already exists", func() {
			csrPEM, privateKey, err := reconciler.generateOperatorCSR(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			// Create first time
			certReq1, err := reconciler.createOrGetCertificateRequest(ctx, bmcObj, csrPEM, privateKey)
			Expect(err).NotTo(HaveOccurred())

			// Create second time - should return existing
			certReq2, err := reconciler.createOrGetCertificateRequest(ctx, bmcObj, csrPEM, privateKey)
			Expect(err).NotTo(HaveOccurred())

			Expect(certReq2.Name).To(Equal(certReq1.Name))
		})
	})

	Context("Certificate Validation", func() {
		It("should validate certificate with valid expiry (> 30 days)", func() {
			// Create a valid certificate that expires in 60 days
			cert := generateTestCertificate(60*24*time.Hour, "test-bmc.example.com")
			secret := createCertificateSecret(cert)

			// Create client with secret
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcObj, secret).
				WithStatusSubresource(bmcObj).
				Build()
			reconciler.Client = k8sClient

			bmcObj.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: "test-secret"}

			valid, err := reconciler.verifyCertificateValidity(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())
		})

		It("should invalidate certificate expiring soon (< 30 days)", func() {
			// Create certificate that expires in 20 days
			cert := generateTestCertificate(20*24*time.Hour, "test-bmc.example.com")
			secret := createCertificateSecret(cert)

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcObj, secret).
				WithStatusSubresource(bmcObj).
				Build()
			reconciler.Client = k8sClient

			bmcObj.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: "test-secret"}

			valid, err := reconciler.verifyCertificateValidity(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeFalse())
		})

		It("should invalidate expired certificate", func() {
			// Create certificate that expired 10 days ago
			cert := generateTestCertificate(-10*24*time.Hour, "test-bmc.example.com")
			secret := createCertificateSecret(cert)

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcObj, secret).
				WithStatusSubresource(bmcObj).
				Build()
			reconciler.Client = k8sClient

			bmcObj.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: "test-secret"}

			valid, err := reconciler.verifyCertificateValidity(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeFalse())
		})

		It("should handle missing certificate secret", func() {
			bmcObj.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: "nonexistent-secret"}

			valid, err := reconciler.verifyCertificateValidity(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeFalse())
		})

		It("should return false when CertificateSecretRef is nil", func() {
			bmcObj.Status.CertificateSecretRef = nil

			valid, err := reconciler.verifyCertificateValidity(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeFalse())
		})
	})

	Context("getCertificateIPAddresses", func() {
		It("should parse IP addresses correctly", func() {
			ips, err := reconciler.getCertificateIPAddresses(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(ips).To(HaveLen(1))
			Expect(ips[0].String()).To(Equal("192.168.1.100"))
		})

		It("should include BMC endpoint IP", func() {
			bmcObj.Spec.Certificate.IPAddresses = []string{"10.0.0.1"}
			bmcObj.Status.IP = metalv1alpha1.MustParseIP("192.168.1.100")

			ips, err := reconciler.getCertificateIPAddresses(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			// Should have both: spec IP and status IP
			Expect(ips).To(HaveLen(2))
			ipStrings := []string{ips[0].String(), ips[1].String()}
			Expect(ipStrings).To(ConsistOf("10.0.0.1", "192.168.1.100"))
		})

		It("should deduplicate IP addresses", func() {
			// Same IP in both spec and status
			bmcObj.Spec.Certificate.IPAddresses = []string{"192.168.1.100"}
			bmcObj.Status.IP = metalv1alpha1.MustParseIP("192.168.1.100")

			ips, err := reconciler.getCertificateIPAddresses(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			// Should only have one entry
			Expect(ips).To(HaveLen(1))
			Expect(ips[0].String()).To(Equal("192.168.1.100"))
		})

		It("should return error for invalid IP address", func() {
			bmcObj.Spec.Certificate.IPAddresses = []string{"invalid-ip"}

			_, err := reconciler.getCertificateIPAddresses(ctx, bmcObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid IP address"))
		})
	})

	Context("CertificateRequest Status Checks", func() {
		It("should detect ready CertificateRequest", func() {
			certReq := &certmanagerv1.CertificateRequest{
				Status: certmanagerv1.CertificateRequestStatus{
					Certificate: []byte("fake-cert-data"),
					Conditions: []certmanagerv1.CertificateRequestCondition{
						{
							Type:   certmanagerv1.CertificateRequestConditionReady,
							Status: cmmeta.ConditionTrue,
						},
					},
				},
			}

			Expect(reconciler.isCertificateRequestReady(certReq)).To(BeTrue())
		})

		It("should detect non-ready CertificateRequest", func() {
			certReq := &certmanagerv1.CertificateRequest{
				Status: certmanagerv1.CertificateRequestStatus{
					Conditions: []certmanagerv1.CertificateRequestCondition{
						{
							Type:   certmanagerv1.CertificateRequestConditionReady,
							Status: cmmeta.ConditionFalse,
						},
					},
				},
			}

			Expect(reconciler.isCertificateRequestReady(certReq)).To(BeFalse())
		})

		It("should detect failed CertificateRequest", func() {
			certReq := &certmanagerv1.CertificateRequest{
				Status: certmanagerv1.CertificateRequestStatus{
					Conditions: []certmanagerv1.CertificateRequestCondition{
						{
							Type:   certmanagerv1.CertificateRequestConditionReady,
							Status: cmmeta.ConditionFalse,
							Reason: "Failed",
						},
					},
				},
			}

			Expect(reconciler.isCertificateRequestFailed(certReq)).To(BeTrue())
		})

		It("should not detect pending as failed", func() {
			certReq := &certmanagerv1.CertificateRequest{
				Status: certmanagerv1.CertificateRequestStatus{
					Conditions: []certmanagerv1.CertificateRequestCondition{
						{
							Type:   certmanagerv1.CertificateRequestConditionReady,
							Status: cmmeta.ConditionFalse,
							Reason: "Pending",
						},
					},
				},
			}

			Expect(reconciler.isCertificateRequestFailed(certReq)).To(BeFalse())
		})
	})

	Context("Condition Setting", func() {
		It("should set CertificateReady condition to True", func() {
			err := reconciler.setCertificateCondition(ctx, bmcObj, corev1.ConditionTrue,
				metalv1alpha1.BMCCertificateReadyReasonIssued, "Certificate installed successfully")
			Expect(err).NotTo(HaveOccurred())

			// Verify condition in status
			var updatedBMC metalv1alpha1.BMC
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(bmcObj), &updatedBMC)
			Expect(err).NotTo(HaveOccurred())

			condition := findCondition(updatedBMC.Status.Conditions)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal(metalv1alpha1.BMCCertificateReadyReasonIssued))
			Expect(condition.Message).To(Equal("Certificate installed successfully"))
		})

		It("should set CertificateReady condition to False with Pending reason", func() {
			err := reconciler.setCertificateCondition(ctx, bmcObj, corev1.ConditionFalse,
				metalv1alpha1.BMCCertificateReadyReasonPending, "Waiting for cert-manager")
			Expect(err).NotTo(HaveOccurred())

			var updatedBMC metalv1alpha1.BMC
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(bmcObj), &updatedBMC)
			Expect(err).NotTo(HaveOccurred())

			condition := findCondition(updatedBMC.Status.Conditions)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(metalv1alpha1.BMCCertificateReadyReasonPending))
		})

		It("should set CertificateReady condition to False with Failed reason", func() {
			err := reconciler.setCertificateCondition(ctx, bmcObj, corev1.ConditionFalse,
				metalv1alpha1.BMCCertificateReadyReasonFailed, "CSR generation failed")
			Expect(err).NotTo(HaveOccurred())

			var updatedBMC metalv1alpha1.BMC
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(bmcObj), &updatedBMC)
			Expect(err).NotTo(HaveOccurred())

			condition := findCondition(updatedBMC.Status.Conditions)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(metalv1alpha1.BMCCertificateReadyReasonFailed))
		})
	})

	Context("Error Handling", func() {
		It("should handle certificate errors with graceful degradation", func() {
			testErr := fmt.Errorf("test error: CSR generation failed")

			result, err := reconciler.handleCertificateError(ctx, bmcObj, testErr, "Failed to generate CSR")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Minute))

			// Verify condition is set to failed
			var updatedBMC metalv1alpha1.BMC
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(bmcObj), &updatedBMC)
			Expect(err).NotTo(HaveOccurred())

			condition := findCondition(updatedBMC.Status.Conditions)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(metalv1alpha1.BMCCertificateReadyReasonFailed))
			Expect(condition.Message).To(ContainSubstring("Failed to generate CSR"))
			Expect(condition.Message).To(ContainSubstring("test error"))
		})

		It("should requeue with retry interval on transient errors", func() {
			testErr := fmt.Errorf("temporary network error")

			result, err := reconciler.handleCertificateError(ctx, bmcObj, testErr, "BMC connection failed")
			Expect(err).NotTo(HaveOccurred())

			// Should requeue after 5 minutes
			Expect(result.RequeueAfter).To(Equal(5 * time.Minute))
		})
	})

	Context("Reconciliation Flow", func() {
		It("should return early when certificate feature is disabled", func() {
			bmcWithoutCert := bmcObj.DeepCopy()
			bmcWithoutCert.Spec.Certificate = nil

			result, err := reconciler.reconcileCertificate(ctx, bmcWithoutCert)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})

		It("should skip provisioning when valid certificate exists", func() {
			// Create a valid certificate that expires in 60 days
			cert := generateTestCertificate(60*24*time.Hour, "test-bmc.example.com")
			secret := createCertificateSecret(cert)

			bmcObj.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: "test-secret"}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcObj, secret).
				WithStatusSubresource(bmcObj).
				Build()
			reconciler.Client = k8sClient

			result, err := reconciler.reconcileCertificate(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})

		It("should renew certificate when it expires soon", func() {
			// Create certificate that expires in 20 days
			cert := generateTestCertificate(20*24*time.Hour, "test-bmc.example.com")
			secret := createCertificateSecret(cert)

			bmcObj.Status.CertificateSecretRef = &corev1.LocalObjectReference{Name: "test-secret"}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcObj, secret).
				WithStatusSubresource(bmcObj).
				Build()
			reconciler.Client = k8sClient

			// Should proceed with renewal (not return early)
			_, err := reconciler.reconcileCertificate(ctx, bmcObj)
			// Will error trying to connect to BMC, but that's expected in unit test
			// We're just verifying it doesn't return early
			Expect(err).To(HaveOccurred()) // Expected: BMC connection error
		})
	})

	Context("Private Key Management", func() {
		It("should store and load private key correctly", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, DefaultRSAKeySize)
			Expect(err).NotTo(HaveOccurred())

			// Store private key
			err = reconciler.storePrivateKey(ctx, bmcObj, privateKey)
			Expect(err).NotTo(HaveOccurred())

			// Load private key
			loadedKey, err := reconciler.loadPrivateKey(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(loadedKey).NotTo(BeNil())

			// Verify keys match
			Expect(loadedKey.N).To(Equal(privateKey.N))
			Expect(loadedKey.E).To(Equal(privateKey.E))
		})

		It("should return nil when private key secret doesn't exist", func() {
			loadedKey, err := reconciler.loadPrivateKey(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(loadedKey).To(BeNil())
		})

		It("should delete private key secret successfully", func() {
			privateKey, err := rsa.GenerateKey(rand.Reader, DefaultRSAKeySize)
			Expect(err).NotTo(HaveOccurred())

			// Store private key
			err = reconciler.storePrivateKey(ctx, bmcObj, privateKey)
			Expect(err).NotTo(HaveOccurred())

			// Delete private key
			err = reconciler.deletePrivateKeySecret(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())

			// Verify it's deleted
			loadedKey, err := reconciler.loadPrivateKey(ctx, bmcObj)
			Expect(err).NotTo(HaveOccurred())
			Expect(loadedKey).To(BeNil())
		})
	})
})

// Helper functions

// createCertificateSecret creates a Kubernetes secret with the given certificate
func createCertificateSecret(certPEM []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": []byte("fake-key"),
		},
	}
}

// findCondition finds a condition in the conditions slice by type
func findCondition(conditions []metav1.Condition) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == metalv1alpha1.BMCCertificateReadyCondition {
			return &conditions[i]
		}
	}
	return nil
}
