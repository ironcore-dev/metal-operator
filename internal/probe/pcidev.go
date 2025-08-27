// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"github.com/jaypipes/ghw"
)

func collectPCIDevicesInfoData() ([]registry.PCIDevice, error) {
	pci, err := ghw.PCI()
	if err != nil {
		return []registry.PCIDevice{}, fmt.Errorf("could not get PCI info: %w", err)
	}
	
	pciDevs := []registry.PCIDevice{}
	for _, p := range pci.Devices {
		nid := -1
		if p.Node != nil {
			nid = p.Node.ID
		}
		pciDevs = append(pciDevs, registry.PCIDevice{
			Address:    p.Address,
			Vendor:     p.Vendor.Name,
			VendorID:   p.Vendor.ID,
			Product:    p.Product.Name,
			ProductID:  p.Product.ID,
			NumaNodeID: nid,
		})
	}
	return pciDevs, nil
}
