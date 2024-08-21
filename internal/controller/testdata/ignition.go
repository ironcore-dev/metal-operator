// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package testdata

var (
	DefaultIgnition = `variant: fcos
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
        ExecStart=/usr/bin/apt-get install docker.io -y
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
        ExecStartPre=/usr/bin/docker pull foo:latest
        ExecStart=/usr/bin/docker run --network host --privileged --name metalprobe foo:latest --registry-url=http://localhost:30000 --server-uuid=38947555-7742-3448-3784-823347823834
        ExecStop=/usr/bin/docker stop metalprobe
        [Install]
        WantedBy=multi-user.target
storage:
  files: []
passwd: {}
`
)
