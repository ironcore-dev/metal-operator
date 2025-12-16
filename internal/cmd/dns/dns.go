package dns

import (
	"fmt"
	"os"
)

// LoadTemplate loads the DNS record template from a mounted file
func LoadTemplate(templatePath string) (string, error) {
	// Try to read from the mounted file
	templateText, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read DNS record template file: %w", err)
	}

	if len(templateText) == 0 {
		return "", fmt.Errorf("DNS record template file is empty: %s", templatePath)
	}
	return string(templateText), nil
}
