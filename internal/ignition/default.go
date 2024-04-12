// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package ignition

import (
	"bytes"
	"fmt"
	"text/template"
)

// ContainerConfig holds the Docker image and flags.
type ContainerConfig struct {
	Image string
	Flags string
}

// defaultIgnitionTemplate is a Go template for the default Ignition configuration.
var defaultIgnitionTemplate = `
ignition_version: "3.0.0"
systemd:
  units:
    - name: docker.service
      enabled: true
    - name: metalprobe.service
      enabled: true
      contents: |
        [Unit]
        Description=Run My Docker Container
        Requires=docker.service
        After=docker.service

        [Service]
        Restart=always
        ExecStartPre=-/usr/bin/docker stop metalprobe
        ExecStartPre=-/usr/bin/docker rm metalprobe
        ExecStartPre=/usr/bin/docker pull {{.Image}}
        ExecStart=/usr/bin/docker run --name metalprobe {{.Flags}} {{.Image}}
        ExecStop=/usr/bin/docker stop metalprobe

        [Install]
        WantedBy=multi-user.target
storage:
  files: []

passwd: {}
`

// GenerateDefaultIgnitionData renders the defaultIgnitionTemplate with the given ContainerConfig.
func GenerateDefaultIgnitionData(config ContainerConfig) ([]byte, error) {
	tmpl, err := template.New("defaultIgnition").Parse(defaultIgnitionTemplate)
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
