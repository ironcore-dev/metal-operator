/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
        ExecStart=/usr/bin/docker run --network host --privileged --name metalprobe foo:latest --registry-url=http://localhost:12345 --server-uuid=38947555-7742-3448-3784-823347823834
        ExecStop=/usr/bin/docker stop metalprobe
        [Install]
        WantedBy=multi-user.target
storage:
  files: []
passwd: {}
`
)
