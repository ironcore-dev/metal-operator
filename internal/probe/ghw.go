// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"

	"k8s.io/utils/ptr"

	"github.com/jaypipes/ghw"
)

type NetDeviceData interface {
	GetModel(ifaceName string) string
	GetSpeed(ifaceName string) string
	GetRevision(ifaceName string) string
}

type networkDeviceData struct {
	netInfo *ghw.NetworkInfo
	pciInfo *ghw.PCIInfo
}

func NewNetworkDeviceData() NetDeviceData {
	netInfo, _ := ghw.Network()
	pciInfo, _ := ghw.PCI()
	return &networkDeviceData{
		netInfo: netInfo,
		pciInfo: pciInfo,
	}
}

func (n *networkDeviceData) GetModel(ifaceName string) string {
	pciAddress := n.findPCIAddressByInterfaceName(ifaceName)
	if n.pciInfo != nil && pciAddress != "" {
		device := n.pciInfo.GetDevice(pciAddress)
		if device != nil {
			return fmt.Sprintf("%s %s", device.Vendor.Name, device.Product.Name)
		}
	}
	return ""
}

func (n *networkDeviceData) GetSpeed(ifaceName string) string {
	nic := n.findNICByInterfaceName(ifaceName)
	if nic != nil {
		return nic.Speed
	}
	return ""
}

func (n *networkDeviceData) GetRevision(ifaceName string) string {
	pciAddress := n.findPCIAddressByInterfaceName(ifaceName)
	if n.pciInfo != nil && pciAddress != "" {
		device := n.pciInfo.GetDevice(pciAddress)
		if device != nil {
			return device.Revision
		}
	}
	return ""
}

func (n *networkDeviceData) findPCIAddressByInterfaceName(ifaceName string) string {
	nic := n.findNICByInterfaceName(ifaceName)
	if nic != nil {
		return ptr.Deref(nic.PCIAddress, "")
	}
	return ""
}

func (n *networkDeviceData) findNICByInterfaceName(ifaceName string) *ghw.NIC {
	if n.netInfo != nil {
		for _, nic := range n.netInfo.NICs {
			if nic.Name == ifaceName {
				return nic
			}
		}
	}
	return nil
}
