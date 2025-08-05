// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

// NetworkInterface represents a network interface on a server,
// including its IP and MAC addresses.
type NetworkInterface struct {
	Name       string `json:"name"`
	IPAddress  string `json:"ipAddress"`
	MACAddress string `json:"macAddress"`
	DeviceName string `json:"deviceName"`
	Model      string `json:"model"`
	Speed      int32  `json:"speed"`
}

// Server represents a server with a list of network interfaces.
type Server struct {
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`
}
