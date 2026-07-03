// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

type NIC struct {
	Name            string   `json:"name"`
	MAC             string   `json:"mac"`
	PCIAddress      string   `json:"pciAddress"`
	Speed           string   `json:"speed"`
	LinkModes       []string `json:"linkModes"`
	SupportedPorts  []string `json:"supportedPorts"`
	FirmwareVersion string   `json:"firmwareVersion"`
	NUMANode        int      `json:"numaNode"`
	Vendor          string   `json:"vendor"`
	SubsystemVendor string   `json:"subsystemVendor"`
	Device          string   `json:"device"`
}
