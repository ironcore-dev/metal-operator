// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"github.com/jaypipes/ghw"

	"github.com/safchain/ethtool"
)

func collectNICInfoData() ([]registry.NIC, error) {
	nics := []registry.NIC{}

	nicinfo, err := ghw.Network()
	if err != nil {
		return []registry.NIC{}, fmt.Errorf("could not get network info: %w", err)
	}

	ethHandle, err := ethtool.NewEthtool()
	if err != nil {
		panic(err.Error())
	}
	defer ethHandle.Close()

	for _, nic := range nicinfo.NICs {
		pci := "unknown"
		if nic.PCIAddress != nil {
			pci = *nic.PCIAddress
		}
		drvInfo, err := ethHandle.DriverInfo(nic.Name)
		if err != nil {
			return []registry.NIC{}, fmt.Errorf("failed to get driver info: %w", err)
		}
		nics = append(nics, registry.NIC{
			Name:            nic.Name,
			MAC:             nic.MACAddress,
			PCIAddress:      pci,
			Speed:           nic.Speed,
			LinkModes:       nic.SupportedLinkModes,
			SupportedPorts:  nic.SupportedPorts,
			FirmwareVersion: drvInfo.FwVersion,
		})
	}
	return nics, nil
}
