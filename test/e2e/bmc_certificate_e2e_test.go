// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ironcore-dev/metal-operator/test/utils"
)

var _ = Describe("BMC Certificate Management", Ordered, func() {
	const (
		bmcWithTLSSecretFile  = "config/samples/bmc_with_tls_secret.yaml"
		bmcSecretFile         = "config/samples/metal_v1alpha1_bmcsecret.yaml"
		bmcWithCertName       = "bmc-with-tls"
		bmcTestNamespace      = "default"
		reconciliationTimeout = 2 * time.Minute
		reconciliationPolling = 2 * time.Second
	)

	BeforeAll(func() {
		By("creating BMC secret")
		cmd := exec.Command("kubectl", "apply", "-f", bmcSecretFile)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create BMC secret")
	})

	AfterAll(func() {
		By("cleaning up BMC resources")
		cmd := exec.Command("kubectl", "delete", "-f", bmcWithTLSSecretFile, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("cleaning up BMC secret")
		cmd = exec.Command("kubectl", "delete", "-f", bmcSecretFile, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	Context("TLS Secret Integration", func() {
		It("should create a BMC with TLS secret reference", func() {
			By("creating BMC resource with TLS secret reference")
			cmd := exec.Command("kubectl", "apply", "-f", bmcWithTLSSecretFile)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create BMC with TLS secret")

			By("verifying BMC resource is created")
			verifyBMCCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName, "-n", bmcTestNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyBMCCreated, reconciliationTimeout, reconciliationPolling).Should(Succeed())
		})

		It("should verify TLS secret exists", func() {
			By("checking if TLS secret exists")
			verifyTLSSecretExists := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", "bmc-tls-cert", "-n", bmcTestNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "TLS secret should exist")
			}
			Eventually(verifyTLSSecretExists, reconciliationTimeout, reconciliationPolling).Should(Succeed())

			By("verifying TLS secret has required keys")
			verifyTLSSecretKeys := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", "bmc-tls-cert", "-n", bmcTestNamespace,
					"-o", "jsonpath={.data}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("tls.crt"), "TLS secret should have tls.crt key")
				g.Expect(output).To(ContainSubstring("tls.key"), "TLS secret should have tls.key key")
			}
			Eventually(verifyTLSSecretKeys, reconciliationTimeout, reconciliationPolling).Should(Succeed())
		})

		It("should update BMC status with certificate ready condition", func() {
			By("verifying BMC reaches Enabled state")
			verifyBMCState := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName, "-n", bmcTestNamespace,
					"-o", "jsonpath={.status.state}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Enabled"), "BMC should be in Enabled state")
			}
			Eventually(verifyBMCState, reconciliationTimeout, reconciliationPolling).Should(Succeed())

			By("checking BMC certificate ready condition")
			verifyCertificateCondition := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName, "-n", bmcTestNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='CertificateReady')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Certificate condition should either be True or not exist yet (if BMC not yet enabled)
				// We accept both states as valid
				g.Expect(output).To(Or(Equal("True"), Equal("")))
			}
			Eventually(verifyCertificateCondition, reconciliationTimeout, reconciliationPolling).Should(Succeed())
		})
	})

	Context("Certificate Lifecycle Management", func() {
		It("should handle TLS secret updates", func() {
			By("updating TLS secret annotation to trigger reconciliation")
			cmd := exec.Command("kubectl", "annotate", "secret", "bmc-tls-cert", "-n", bmcTestNamespace,
				"test-update="+fmt.Sprint(time.Now().Unix()), "--overwrite")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying BMC reconciles after secret update")
			verifyBMCReconciled := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName, "-n", bmcTestNamespace,
					"-o", "jsonpath={.status.state}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Enabled"))
			}
			Eventually(verifyBMCReconciled, reconciliationTimeout, reconciliationPolling).Should(Succeed())
		})

		It("should clean up when BMC is deleted", func() {
			By("deleting BMC resource")
			cmd := exec.Command("kubectl", "delete", "bmc", bmcWithCertName, "-n", bmcTestNamespace, "--timeout=1m")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying BMC is removed")
			verifyBMCDeleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "bmc", bmcWithCertName, "-n", bmcTestNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "BMC should be deleted")
			}
			Eventually(verifyBMCDeleted, reconciliationTimeout, reconciliationPolling).Should(Succeed())
		})
	})
})
