// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"net"
	"strings"

	"github.com/afritzler/metal-operator/internal/api/registry"
)

// collectNetworkData collects the IP and MAC addresses of the host's network interfaces,
// ignoring loopback and tunnel (tun) devices.
func collectNetworkData() ([]registry.NetworkInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var networkInterfaces []registry.NetworkInterface
	for _, iface := range interfaces {
		// Skip loopback, interfaces without a MAC address, and tun devices.
		if iface.Flags&net.FlagLoopback != 0 || iface.HardwareAddr.String() == "" || strings.HasPrefix(iface.Name, "tun") {
			continue
		}

		addrs, err := iface.Addrs()
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

			networkInterface := registry.NetworkInterface{
				Name:       iface.Name,
				IPAddress:  ip.String(),
				MACAddress: iface.HardwareAddr.String(),
			}
			networkInterfaces = append(networkInterfaces, networkInterface)
		}
	}

	return networkInterfaces, nil
}
