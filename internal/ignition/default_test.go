// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package ignition

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIgnition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ignition Suite")
}

var _ = Describe("Ignition Template Generation", func() {
	var (
		testConfig Config
	)

	BeforeEach(func() {
		testConfig = Config{
			Image:        "test-image:latest",
			Flags:        "--test-flag=value",
			SSHPublicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC... test@example.com",
			PasswordHash: "$2a$10$abcdefghijklmnopqrstuvwxyz",
		}
	})

	Context("GenerateIgnitionDataFromFile", func() {
		It("should generate ignition data from file template", func() {
			customTemplate := `variant: fcos
version: "1.4.0"
systemd:
  units:
    - name: custom-service.service
      enabled: true
      contents: |-
        [Unit]
        Description=Custom Service
        [Service]
        ExecStart=/usr/bin/docker run {{.Image}} {{.Flags}}
        [Install]
        WantedBy=multi-user.target
passwd:
  users:
    - name: custom-user
      password_hash: {{.PasswordHash}}
      ssh_authorized_keys: [ {{.SSHPublicKey}} ]`

			tmpFile, err := os.CreateTemp("", "ignition-template-*.yaml")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.Remove, tmpFile.Name())

			_, err = tmpFile.WriteString(customTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpFile.Close()).To(Succeed())

			data, err := GenerateIgnitionDataFromFile(tmpFile.Name(), testConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).NotTo(BeEmpty())

			ignitionStr := string(data)
			Expect(ignitionStr).To(ContainSubstring("version: \"1.4.0\""))
			Expect(ignitionStr).To(ContainSubstring("custom-service.service"))
			Expect(ignitionStr).To(ContainSubstring("test-image:latest"))
			Expect(ignitionStr).To(ContainSubstring("custom-user"))
		})

		It("should return error when file does not exist", func() {
			_, err := GenerateIgnitionDataFromFile("/nonexistent/path/file.yaml", testConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read file"))
		})

		It("should return error when template is invalid", func() {
			invalidTemplate := `variant: fcos
systemd:
  units:
    - name: broken-service
      contents: {{.InvalidTemplate}}`

			tmpFile, err := os.CreateTemp("", "invalid-template-*.yaml")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.Remove, tmpFile.Name())

			_, err = tmpFile.WriteString(invalidTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpFile.Close()).To(Succeed())

			_, err = GenerateIgnitionDataFromFile(tmpFile.Name(), testConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("executing template failed"))
		})

		It("should return error when file path is empty", func() {
			_, err := GenerateIgnitionDataFromFile("", testConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("file path must be specified"))
		})
	})

	Context("generateIgnitionDataFromTemplate", func() {
		It("should render custom template correctly", func() {
			customTemplate := `
image: {{.Image}}
flags: {{.Flags}}
user_key: {{.SSHPublicKey}}
password: {{.PasswordHash}}`

			data, err := generateIgnitionDataFromTemplate(customTemplate, testConfig)
			Expect(err).NotTo(HaveOccurred())

			result := string(data)
			Expect(result).To(ContainSubstring("image: test-image:latest"))
			Expect(result).To(ContainSubstring("flags: --test-flag=value"))
			Expect(result).To(ContainSubstring("user_key: " + testConfig.SSHPublicKey))
			Expect(result).To(ContainSubstring("password: " + testConfig.PasswordHash))
		})

		It("should handle template parsing errors", func() {
			invalidTemplate := `{{.UnknownField}}`
			_, err := generateIgnitionDataFromTemplate(invalidTemplate, testConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("executing template failed"))
		})
	})
})
