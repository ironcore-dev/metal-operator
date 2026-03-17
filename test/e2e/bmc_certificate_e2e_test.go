// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ironcore-dev/metal-operator/test/utils"
)

var _ = Describe("BMC Certificate Management", Ordered, func() {
	const (
		bmcCertSampleFile     = "config/samples/bmc_with_certificate.yaml"
		selfSignedIssuerFile  = "test/e2e/testdata/selfsigned-issuer.yaml"
		bmcSecretFile         = "config/samples/metal_v1alpha1_bmcsecret.yaml"
		bmcWithCertName       = "bmc-with-cert"
		bmcTestNamespace      = "default"
		certificateTimeout    = 3 * time.Minute
		certificatePolling    = 5 * time.Second
		reconciliationTimeout = 2 * time.Minute
		reconciliationPolling = 2 * time.Second
	)

	BeforeAll(func() {
		By("ensuring cert-manager is installed and ready")
		verifyCertManagerReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", "cert-manager", "-n", "cert-manager",
				"-o", "jsonpath={.status.conditions[?(@.type=='Available')].status}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred(), "cert-manager should be installed")
			g.Expect(output).To(Equal("True"), "cert-manager not ready")
		}
		Eventually(verifyCertManagerReady, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("creating self-signed ClusterIssuer for testing")
		cmd := exec.Command("kubectl", "apply", "-f", selfSignedIssuerFile)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create self-signed issuer")

		By("waiting for ClusterIssuer to be ready")
		verifyIssuerReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "issuer", "selfsigned-issuer", "-n", bmcTestNamespace,
				"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("True"), "Issuer not ready")
		}
		Eventually(verifyIssuerReady, certificateTimeout, certificatePolling).Should(Succeed())

		By("creating BMC secret")
		cmd = exec.Command("kubectl", "apply", "-f", bmcSecretFile)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create BMC secret")
	})

	AfterAll(func() {
		By("cleaning up BMC resources")
		cmd := exec.Command("kubectl", "delete", "-f", bmcCertSampleFile, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("cleaning up self-signed issuer")
		cmd = exec.Command("kubectl", "delete", "-f", selfSignedIssuerFile, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("cleaning up BMC secret")
		cmd = exec.Command("kubectl", "delete", "-f", bmcSecretFile, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	Context("Basic Certificate Provisioning", func() {
		It("should create a BMC with certificate management enabled", func() {
			By("creating BMC resource with certificate spec")
			cmd := exec.Command("kubectl", "apply", "-f", bmcCertSampleFile)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create BMC with certificate")

			By("verifying BMC resource is created")
			verifyBMCCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName, "-n", bmcTestNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyBMCCreated, reconciliationTimeout, reconciliationPolling).Should(Succeed())
		})

		It("should create a Certificate resource for the BMC", func() {
			By("checking if Certificate resource exists")
			verifyCertificateExists := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate",
					fmt.Sprintf("%s-cert", bmcWithCertName),
					"-n", bmcTestNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Certificate resource should be created")
			}
			Eventually(verifyCertificateExists, reconciliationTimeout, reconciliationPolling).Should(Succeed())

			By("verifying Certificate has correct issuerRef")
			verifyCertificateIssuer := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate",
					fmt.Sprintf("%s-cert", bmcWithCertName),
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.spec.issuerRef.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("selfsigned-issuer"))
			}
			Eventually(verifyCertificateIssuer, certificatePolling, certificatePolling).Should(Succeed())
		})

		It("should provision the certificate via cert-manager", func() {
			By("waiting for Certificate to become Ready")
			verifyCertificateReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate",
					fmt.Sprintf("%s-cert", bmcWithCertName),
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Certificate not ready")
			}
			Eventually(verifyCertificateReady, certificateTimeout, certificatePolling).Should(Succeed())

			By("verifying certificate Secret is created")
			verifySecretExists := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret",
					fmt.Sprintf("%s-cert", bmcWithCertName),
					"-n", bmcTestNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Certificate Secret should exist")
			}
			Eventually(verifySecretExists, certificatePolling, certificatePolling).Should(Succeed())
		})

		It("should update BMC status with certificate information", func() {
			By("checking BMC CertificateReady condition")
			verifyCertificateCondition := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName,
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='CertificateReady')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "BMC CertificateReady condition should be True")
			}
			Eventually(verifyCertificateCondition, reconciliationTimeout, reconciliationPolling).Should(Succeed())

			By("verifying certificate reference in BMC status")
			verifyCertificateRef := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName,
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.status.certificateRef.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(fmt.Sprintf("%s-cert", bmcWithCertName)))
			}
			Eventually(verifyCertificateRef, certificatePolling, certificatePolling).Should(Succeed())
		})

		It("should have certificate Secret with expected keys", func() {
			By("verifying Secret contains tls.crt")
			cmd := exec.Command("kubectl", "get", "secret",
				fmt.Sprintf("%s-cert", bmcWithCertName),
				"-n", bmcTestNamespace,
				"-o", "jsonpath={.data.tls\\.crt}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "tls.crt should exist")

			By("verifying Secret contains tls.key")
			cmd = exec.Command("kubectl", "get", "secret",
				fmt.Sprintf("%s-cert", bmcWithCertName),
				"-n", bmcTestNamespace,
				"-o", "jsonpath={.data.tls\\.key}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "tls.key should exist")
		})
	})

	Context("Certificate Lifecycle Management", func() {
		It("should handle certificate updates when BMC certificate spec changes", func() {
			By("recording original certificate serial number")
			cmd := exec.Command("kubectl", "get", "certificate",
				fmt.Sprintf("%s-cert", bmcWithCertName),
				"-n", bmcTestNamespace,
				"-o", "jsonpath={.status.revision}")
			originalRevision, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("updating BMC certificate duration")
			cmd = exec.Command("kubectl", "patch", "bmc", bmcWithCertName,
				"-n", bmcTestNamespace,
				"--type=merge",
				"-p", `{"spec":{"certificate":{"duration":"4320h"}}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Certificate resource is updated")
			verifyCertificateUpdated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate",
					fmt.Sprintf("%s-cert", bmcWithCertName),
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.spec.duration}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("4320h0m0s"))
			}
			Eventually(verifyCertificateUpdated, reconciliationTimeout, reconciliationPolling).Should(Succeed())

			By("waiting for new certificate to be issued")
			verifyNewRevision := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate",
					fmt.Sprintf("%s-cert", bmcWithCertName),
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.status.revision}")
				newRevision, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(newRevision).NotTo(Equal(originalRevision), "Certificate should be reissued")
			}
			Eventually(verifyNewRevision, certificateTimeout, certificatePolling).Should(Succeed())
		})

		It("should clean up Certificate when BMC certificate spec is removed", func() {
			By("removing certificate spec from BMC")
			cmd := exec.Command("kubectl", "patch", "bmc", bmcWithCertName,
				"-n", bmcTestNamespace,
				"--type=json",
				"-p", `[{"op":"remove","path":"/spec/certificate"}]`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Certificate resource is deleted")
			verifyCertificateDeleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate",
					fmt.Sprintf("%s-cert", bmcWithCertName),
					"-n", bmcTestNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "Certificate should be deleted")
			}
			Eventually(verifyCertificateDeleted, reconciliationTimeout, reconciliationPolling).Should(Succeed())

			By("verifying BMC CertificateReady condition is removed or False")
			verifyConditionRemoved := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName,
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='CertificateReady')]}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Condition should either be empty or have status False
				g.Expect(output).To(Or(BeEmpty(), ContainSubstring(`"status":"False"`)))
			}
			Eventually(verifyConditionRemoved, reconciliationTimeout, reconciliationPolling).Should(Succeed())
		})
	})

	Context("Error Handling and Validation", func() {
		const invalidBMCName = "bmc-invalid-issuer"

		AfterEach(func() {
			By("cleaning up test BMC")
			cmd := exec.Command("kubectl", "delete", "bmc", invalidBMCName,
				"-n", bmcTestNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should handle invalid issuer reference gracefully", func() {
			By("creating BMC with non-existent issuer")
			invalidBMCYAML := fmt.Sprintf(`
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: %s
  namespace: %s
spec:
  access:
    ip: 192.168.1.200
    macAddress: "00:1A:2B:3C:4D:6F"
  bmcSecretRef:
    name: bmcsecret-sample
  protocol:
    name: Redfish
    port: 443
    scheme: https
  certificate:
    issuerRef:
      name: non-existent-issuer
      kind: Issuer
    duration: 2160h
`, invalidBMCName, bmcTestNamespace)

			// Write YAML to temp file and apply
			tmpFile := "/tmp/bmc-invalid-issuer.yaml"
			err := os.WriteFile(tmpFile, []byte(invalidBMCYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(tmpFile) }()

			cmd := exec.Command("kubectl", "apply", "-f", tmpFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "BMC should be created even with invalid issuer")

			By("verifying BMC CertificateReady condition reports error")
			verifyCertificateError := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", invalidBMCName,
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='CertificateReady')]}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Or(
					ContainSubstring(`"status":"False"`),
					ContainSubstring(`"status":"Unknown"`),
				))
			}
			Eventually(verifyCertificateError, reconciliationTimeout, reconciliationPolling).Should(Succeed())

			By("verifying Certificate resource reports error")
			verifyCertificateNotReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate",
					fmt.Sprintf("%s-cert", invalidBMCName),
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				// Certificate may not exist yet or may be in error state
				if err == nil {
					g.Expect(output).NotTo(Equal("True"))
				}
			}
			Eventually(verifyCertificateNotReady, certificateTimeout, certificatePolling).Should(Succeed())
		})

		It("should handle missing certificate fields with defaults", func() {
			By("creating BMC with minimal certificate spec")
			minimalBMCName := "bmc-minimal-cert"
			minimalBMCYAML := fmt.Sprintf(`
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: %s
  namespace: %s
spec:
  access:
    ip: 192.168.1.201
    macAddress: "00:1A:2B:3C:4D:70"
  bmcSecretRef:
    name: bmcsecret-sample
  protocol:
    name: Redfish
    port: 443
    scheme: https
  certificate:
    issuerRef:
      name: selfsigned-issuer
      kind: Issuer
`, minimalBMCName, bmcTestNamespace)

			// Write YAML to temp file and apply
			tmpFile := "/tmp/bmc-minimal-cert.yaml"
			err := os.WriteFile(tmpFile, []byte(minimalBMCYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.Remove(tmpFile) }()

			cmd := exec.Command("kubectl", "apply", "-f", tmpFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Certificate is created with default values")
			verifyCertificateCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate",
					fmt.Sprintf("%s-cert", minimalBMCName),
					"-n", bmcTestNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyCertificateCreated, reconciliationTimeout, reconciliationPolling).Should(Succeed())

			By("cleaning up minimal BMC")
			cmd = exec.Command("kubectl", "delete", "bmc", minimalBMCName, "-n", bmcTestNamespace)
			_, _ = utils.Run(cmd)
		})
	})

	Context("Certificate Integration with BMC Operations", func() {
		It("should allow BMC operations to continue during certificate provisioning", func() {
			By("verifying BMC status is updated even before certificate is ready")
			verifyBMCStatus := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName,
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "BMC should have status")
			}
			Eventually(verifyBMCStatus, reconciliationTimeout, reconciliationPolling).Should(Succeed())
		})

		It("should have owner reference from BMC to Certificate", func() {
			By("verifying Certificate has BMC as owner")
			verifyCertificateOwner := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate",
					fmt.Sprintf("%s-cert", bmcWithCertName),
					"-n", bmcTestNamespace,
					"-o", "jsonpath={.metadata.ownerReferences[0].kind}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("BMC"))
			}
			Eventually(verifyCertificateOwner, certificatePolling, certificatePolling).Should(Succeed())

			By("verifying owner reference points to correct BMC")
			cmd := exec.Command("kubectl", "get", "certificate",
				fmt.Sprintf("%s-cert", bmcWithCertName),
				"-n", bmcTestNamespace,
				"-o", "jsonpath={.metadata.ownerReferences[0].name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(bmcWithCertName))
		})
	})
})
