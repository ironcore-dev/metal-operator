// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package ignition

import (
	"bytes"
	"fmt"
	"text/template"
)

// Config holds the Docker image and flags.
type Config struct {
	Image        string
	Flags        string
	SSHPublicKey string
	PasswordHash string
}

// defaultIgnitionTemplate is a Go template for the default Ignition configuration.
var defaultIgnitionTemplate = `variant: fcos
version: "1.3.0"
systemd:
  units:
    - name: docker-install.service
      enabled: true
      contents: |-
        [Unit]
        Description=Install Docker
        Before=metalprobe.service
        [Service]
        Restart=on-failure
        RestartSec=20
        Type=oneshot
        RemainAfterExit=yes
        ExecStart=/usr/bin/apt-get update
        ExecStart=/usr/bin/apt-get install docker.io docker-cli -y
        [Install]
        WantedBy=multi-user.target
    - name: docker.service
      enabled: true
    - name: metalprobe.service
      enabled: true
      contents: |-
        [Unit]
        Description=Run My Docker Container
        [Service]
        Restart=on-failure
        RestartSec=20
        ExecStartPre=-/usr/bin/docker stop metalprobe
        ExecStartPre=-/usr/bin/docker rm metalprobe
        ExecStartPre=/usr/bin/docker pull {{.Image}}
        ExecStart=/usr/bin/docker run --network host --privileged --name metalprobe {{.Image}} {{.Flags}}
        ExecStop=/usr/bin/docker stop metalprobe
        [Install]
        WantedBy=multi-user.target
storage:
  files: []
passwd:
  users:
    - name: metal
      password_hash: {{.PasswordHash}}
      groups: [ "wheel" ]
      ssh_authorized_keys: [ {{.SSHPublicKey}} ]
`

// GenerateDefaultIgnitionData renders the defaultIgnitionTemplate with the given Config.
func GenerateDefaultIgnitionData(config Config) ([]byte, error) {
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
