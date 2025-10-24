// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

type PCIDevice struct {
	Address    string `json:"address,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	VendorID   string `json:"vendorID,omitempty"`
	Product    string `json:"product,omitempty"`
	ProductID  string `json:"productID,omitempty"`
	NumaNodeID int    `json:"numaNodeID,omitempty"`
}
