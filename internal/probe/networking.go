// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"net"
	"strings"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

// IsSLAAC checks if the given IPv6 address is a SLAAC address.
func IsSLAAC(ip string) bool {
	return strings.Contains(ip, "ff:fe")
}

// collectNetworkData collects the IP and MAC addresses of the host's network interfaces,
// including all interfaces with their up/down status.
func collectNetworkData() ([]registry.NetworkInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	networkInterfaces := make([]registry.NetworkInterface, 0, len(interfaces))
	for _, iface := range interfaces {
		// Skip only loopback, tun devices, and docker interface
		// But include all other interfaces regardless of up/down status
		if iface.Flags&net.FlagLoopback != 0 ||
			strings.HasPrefix(iface.Name, "tun") ||
			strings.HasPrefix(iface.Name, "docker0") {
			continue
		}

		// Determine if interface is up or down
		status := "down"
		if iface.Flags&net.FlagRunning != 0 {
			status = "up"
		}

		addrs, err := iface.Addrs()
		if err != nil {
			// If we can't get addresses, still include the interface with empty IP
			networkInterface := registry.NetworkInterface{
				Name:          iface.Name,
				IPAddress:     "",
				IPv6Addresses: []string{},
				MACAddress:    iface.HardwareAddr.String(),
				Status:        status,
			}
			networkInterfaces = append(networkInterfaces, networkInterface)
			continue
		}

		// If interface has no addresses, still include it
		if len(addrs) == 0 {
			networkInterface := registry.NetworkInterface{
				Name:          iface.Name,
				IPAddress:     "",
				IPv6Addresses: []string{},
				MACAddress:    iface.HardwareAddr.String(),
				Status:        status,
			}
			networkInterfaces = append(networkInterfaces, networkInterface)
			continue
		}

		// Collect IPv4 and IPv6 addresses separately
		var ipv4Address string
		var ipv6Addresses []string

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

			if ip.To4() != nil {
				// IPv4 address
				if ipv4Address == "" { // Take only the first IPv4 address
					ipv4Address = ip.String()
				}
			} else {
				// IPv6 address - now including SLAAC addresses
				ipv6Addresses = append(ipv6Addresses, ip.String())
			}
		}

		// Create network interface with all collected addresses
		networkInterface := registry.NetworkInterface{
			Name:          iface.Name,
			IPAddress:     ipv4Address,
			IPv6Addresses: ipv6Addresses,
			MACAddress:    iface.HardwareAddr.String(),
			Status:        status,
		}
		networkInterfaces = append(networkInterfaces, networkInterface)
	}

	return networkInterfaces, nil
}
