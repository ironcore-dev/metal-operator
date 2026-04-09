// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package ignition

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
)

// Config holds the Docker image and flags.
type Config struct {
	Image        string
	Flags        string
	SSHPublicKey string
	PasswordHash string
}

// GenerateIgnitionDataFromFile renders an ignition template from a file with the given Config.
func GenerateIgnitionDataFromFile(filePath string, config Config) ([]byte, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path must be specified")
	}

	templateContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	trimmedContent := bytes.TrimSpace(templateContent)
	if len(trimmedContent) == 0 {
		return nil, fmt.Errorf("ignition template must not be empty")
	}

	return generateIgnitionDataFromTemplate(string(trimmedContent), config)
}

// generateIgnitionDataFromTemplate is a helper function that renders any template with the given Config.
func generateIgnitionDataFromTemplate(templateContent string, config Config) ([]byte, error) {
	tmpl, err := template.New("ignition").Parse(templateContent)
	if err != nil {
		return nil, fmt.Errorf("parsing template failed: %w", err)
	}

	var out bytes.Buffer
	err = tmpl.Execute(&out, config)
	if err != nil {
		return nil, fmt.Errorf("executing template failed: %w", err)
	}

	return out.Bytes(), nil
}

// ValidateIgnitionTemplatePath validates that the ignition template file exists and can be parsed.
func ValidateIgnitionTemplatePath(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("ignition template path is empty")
	}

	// Check file exists and is readable
	templateContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("cannot read ignition template file: %w", err)
	}

	trimmedContent := bytes.TrimSpace(templateContent)
	if len(trimmedContent) == 0 {
		return fmt.Errorf("ignition template must not be empty")
	}

	// Validate template syntax by attempting to parse it
	_, err = template.New("ignition-validation").Parse(string(trimmedContent))
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	return nil
}
