// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

// NetworkInterface represents a network interface on a server,
// including its IP and MAC addresses.
type NetworkInterface struct {
	Name          string   `json:"name"`
	IpAddresses   []string `json:"ipAddresses"`
	MACAddress    string   `json:"macAddress"`
	CarrierStatus string   `json:"carrierStatus"`
}

// Server represents a server with a list of network interfaces.
type Server struct {
	SystemInfo        DMI                `json:"systemInfo,omitempty"`
	CPU               []CPUInfo          `json:"cpu,omitempty"`
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`
	LLDP              []LLDPInterface    `json:"lldp,omitempty"`
	Storage           []BlockDevice      `json:"storage,omitempty"`
	Memory            []MemoryDevice     `json:"memory,omitempty"`
	NICs              []NIC              `json:"nics,omitempty"`
	PCIDevices        []PCIDevice        `json:"pciDevices,omitempty"`
}
