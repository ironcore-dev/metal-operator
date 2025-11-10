// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

type BlockDevice struct {
	Path              string `json:"path"`
	Name              string `json:"name"`
	Rotational        bool   `json:"rotational"`
	Removable         bool   `json:"removable"`
	ReadOnly          bool   `json:"readOnly"`
	Vendor            string `json:"vendor"`
	Model             string `json:"model"`
	Serial            string `json:"serial"`
	WWID              string `json:"wwid"`
	PhysicalBlockSize uint64 `json:"physicalBlockSize"`
	LogicalBlockSize  uint64 `json:"logicalBlockSize"`
	HWSectorSize      uint64 `json:"hWSectorSize"`
	SizeBytes         uint64 `json:"sizeBytes"`
	NUMANodeID        int    `json:"numaNodeID"`
	// TODO: do we need to gather health info and other stats?
}
