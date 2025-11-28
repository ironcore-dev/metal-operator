// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package ignition

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIgnition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ignition Suite")
}

var _ = Describe("Ignition Template Generation", func() {
	var (
		ctx           context.Context
		fakeClient    client.Client
		testConfig    Config
		testNamespace = "test-namespace"
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(v1.AddToScheme(scheme)).To(Succeed())
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		testConfig = Config{
			Image:        "test-image:latest",
			Flags:        "--test-flag=value",
			SSHPublicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC... test@example.com",
			PasswordHash: "$2a$10$abcdefghijklmnopqrstuvwxyz",
		}
	})

	Context("GenerateDefaultIgnitionData", func() {
		It("should generate valid ignition data from hardcoded template", func() {
			data, err := GenerateDefaultIgnitionData(testConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).NotTo(BeEmpty())

			// Verify the template was rendered with our config values
			ignitionStr := string(data)
			Expect(ignitionStr).To(ContainSubstring("test-image:latest"))
			Expect(ignitionStr).To(ContainSubstring("--test-flag=value"))
			Expect(ignitionStr).To(ContainSubstring(testConfig.SSHPublicKey))
			Expect(ignitionStr).To(ContainSubstring(testConfig.PasswordHash))
			Expect(ignitionStr).To(ContainSubstring("variant: fcos"))
			Expect(ignitionStr).To(ContainSubstring("version: \"1.3.0\""))
		})

		It("should succeed even with missing template fields", func() {
			// The hardcoded template is robust and doesn't fail on missing fields
			config := Config{
				Image:        "",
				Flags:        "",
				SSHPublicKey: "",
				PasswordHash: "",
			}
			data, err := GenerateDefaultIgnitionData(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).NotTo(BeEmpty())

			ignitionStr := string(data)
			Expect(ignitionStr).To(ContainSubstring("variant: fcos"))
		})
	})

	Context("GenerateIgnitionDataFromConfigMap", func() {
		It("should generate ignition data from ConfigMap template", func() {
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

			configMap := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-ignition",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"template": customTemplate,
				},
			}
			Expect(fakeClient.Create(ctx, configMap)).To(Succeed())

			data, err := GenerateIgnitionDataFromConfigMap(
				ctx, fakeClient, testNamespace, "custom-ignition", "template", testConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).NotTo(BeEmpty())

			ignitionStr := string(data)
			Expect(ignitionStr).To(ContainSubstring("version: \"1.4.0\""))
			Expect(ignitionStr).To(ContainSubstring("custom-service.service"))
			Expect(ignitionStr).To(ContainSubstring("test-image:latest"))
			Expect(ignitionStr).To(ContainSubstring("custom-user"))
		})

		It("should return error when ConfigMap does not exist", func() {
			_, err := GenerateIgnitionDataFromConfigMap(
				ctx, fakeClient, testNamespace, "nonexistent-configmap", "template", testConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get ConfigMap"))
		})

		It("should return error when ConfigMap key does not exist", func() {
			configMap := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"other-key": "some-data",
				},
			}
			Expect(fakeClient.Create(ctx, configMap)).To(Succeed())

			_, err := GenerateIgnitionDataFromConfigMap(
				ctx, fakeClient, testNamespace, "test-configmap", "nonexistent-key", testConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("key nonexistent-key not found"))
		})

		It("should return error when template is invalid", func() {
			invalidTemplate := `variant: fcos
systemd:
  units:
    - name: broken-service
      contents: {{.InvalidTemplate}}`

			configMap := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-template",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"template": invalidTemplate,
				},
			}
			Expect(fakeClient.Create(ctx, configMap)).To(Succeed())

			_, err := GenerateIgnitionDataFromConfigMap(
				ctx, fakeClient, testNamespace, "invalid-template", "template", testConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("executing template failed"))
		})

		It("should return error when ConfigMap name is empty", func() {
			_, err := GenerateIgnitionDataFromConfigMap(
				ctx, fakeClient, testNamespace, "", "template", testConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ConfigMap name and key must be specified"))
		})

		It("should return error when ConfigMap key is empty", func() {
			_, err := GenerateIgnitionDataFromConfigMap(
				ctx, fakeClient, testNamespace, "test-configmap", "", testConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ConfigMap name and key must be specified"))
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
