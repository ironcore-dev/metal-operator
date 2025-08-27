// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

type BlockDevice struct {
	Path string
	Name string
	//Type       string
	Rotational bool
	Removable  bool
	ReadOnly   bool
	Vendor     string
	Model      string
	Serial     string
	WWID       string
	//FirmwareRevision  string
	//State             string
	PhysicalBlockSize uint64
	LogicalBlockSize  uint64
	HWSectorSize      uint64
	Size              uint64
	NUMANodeID        uint64
	// TODO: do we need to gather health info and other stats?
}
