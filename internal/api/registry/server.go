// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

// NetworkInterface represents a network interface on a server,
// including its IP and MAC addresses.
type NetworkInterface struct {
	Name       string `json:"name"`
	IPAddress  string `json:"ipAddress"`
	MACAddress string `json:"macAddress"`
}

type DMI struct {
	BIOSInformation   BIOSInformation
	SystemInformation ServerInformation
	BoardInformation  BoardInformation
}

type BIOSInformation struct {
	Vendor  string
	Version string
	Date    string
}

type ServerInformation struct {
	Manufacturer string
	ProductName  string
	Version      string
	SerialNumber string
	UUID         string
	SKUNumber    string
	Family       string
}

type BoardInformation struct {
	Manufacturer string
	Product      string
	Version      string
	SerialNumber string
	AssetTag     string
}

// Server represents a server with a list of network interfaces.
type Server struct {
	SystemInfo        DMI                `json:"systemInfo,omitempty"`
	CPU               []CPUInfo          `json:"cpu,omitempty"`
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`
	LLDP              LLDP               `json:"lldp,omitempty"`
	Storage           []BlockDevice      `json:"storage,omitempty"`
}
