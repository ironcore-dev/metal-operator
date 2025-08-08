// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"
	"net"
	"strings"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

// isSLAAC checks if the given IPv6 address is a SLAAC address.
func isSLAAC(ip string) bool {
	return strings.Contains(ip, "ff:fe")
}

type NIC interface {
	Interfaces() ([]net.Interface, error)
	Addrs(iface *net.Interface) ([]net.Addr, error)
}

type nic struct{}

func NewNIC() NIC {
	return &nic{}
}

func (nic *nic) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

func (nic *nic) Addrs(iface *net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

type NetworkDataCollector interface {
	CollectNetworkData() ([]registry.NetworkInterface, error)
}

type networkDataCollector struct {
	netInterfaces NIC
	linuxNetData  LinuxNetworkData
}

func NewNetworkDataCollector(netInterfaces NIC, linuxNetData LinuxNetworkData) NetworkDataCollector {
	return &networkDataCollector{netInterfaces: netInterfaces, linuxNetData: linuxNetData}
}

// collectNetworkData collects the IP and MAC addresses of the host's network interfaces,
// ignoring loopback and tunnel (tun) devices.
func (n *networkDataCollector) CollectNetworkData() ([]registry.NetworkInterface, error) {
	interfaces, err := n.netInterfaces.Interfaces()
	if err != nil {
		return nil, err
	}

	var networkInterfaces []registry.NetworkInterface
	for _, iface := range interfaces {
		// Skip loopback, interfaces without a MAC address, tun devices, docker interface
		if iface.Flags&net.FlagLoopback != 0 ||
			iface.HardwareAddr.String() == "" ||
			strings.HasPrefix(iface.Name, "tun") ||
			strings.HasPrefix(iface.Name, "docker0") ||
			iface.Flags&net.FlagUp == 0 { // Filter out interfaces that are down
			continue
		}

		addrs, err := n.netInterfaces.Addrs(&iface)
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Filter out SLAAC addresses
			if ip.To4() == nil && isSLAAC(ip.String()) {
				continue
			}

			pciAddress := n.linuxNetData.GetNetworkDevicePCIAddress(iface.Name)
			speed := n.linuxNetData.GetNetworkDeviceSpeed(iface.Name)
			deviceData := n.linuxNetData.GetNetworkDeviceModaliasData(iface.Name)

			model := ""
			if deviceData != nil {
				model = fmt.Sprintf("%s %s", deviceData.vendorID, deviceData.productID)
			}

			networkInterface := registry.NetworkInterface{
				Name:       iface.Name,
				IPAddress:  ip.String(),
				MACAddress: iface.HardwareAddr.String(),
				PCIAddress: pciAddress,
				Model:      model,
				Speed:      speed,
			}
			networkInterfaces = append(networkInterfaces, networkInterface)
		}
	}

	return networkInterfaces, nil
}
