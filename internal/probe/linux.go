// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	pathSysClassNet   = "/sys/class/net"
	pathBusPciDevices = "/bus/pci/devices"
)

const (
	lengthModAlias = 53
)

type deviceModaliasData struct {
	vendorID     string
	productID    string
	subproductID string
	subvendorID  string
	class        string
	subclass     string
	progIface    string
}

func getNetworkDeviceMacAddress(device string) string {
	// For devices that have addr_assign_type != 0, return empty string since the MAC address is random.
	value, err := os.ReadFile(filepath.Join(pathSysClassNet, device, "addr_assign_type"))
	if err != nil {
		return ""
	}
	if strings.TrimSpace(string(value)) != "0" {
		return ""
	}
	value, err = os.ReadFile(filepath.Join(pathSysClassNet, device, "address"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(value))
}

func getNetworkDeviceSpeed(device string) string {
	speed, err := os.ReadFile(filepath.Join(pathSysClassNet, device, "speed"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(speed))
}

func getNetworkDevicePath(device string) string {
	netDeviceLink, err := os.Readlink(filepath.Join(pathSysClassNet, device)) // e.g., ../../devices/pci0000:00/0000:00:1f.6/net/eth0
	if err != nil {
		return ""
	}

	devicePath := filepath.Clean(filepath.Join(pathSysClassNet, netDeviceLink)) // e.g., /sys/devices/pci0000:00/0000:00:1f.6/net/eth0
	if strings.Contains(devicePath, "devices/virtual/net") {
		return "" // This is a virtual network device, no PCI address.
	}

	deviceLink, err := os.Readlink(filepath.Join(devicePath, "device")) // e.g., ../../0000:00:1f.6
	if err != nil {
		return ""
	}

	return filepath.Clean(filepath.Join(devicePath, deviceLink)) // e.g., /sys/devices/pci0000:00/0000:00:1f.6
}

func getNetworkDevicePCIAddress(device string) string {
	devicePath := getNetworkDevicePath(device)
	if devicePath == "" {
		return ""
	}

	deviceLink, err := os.Readlink(filepath.Join(devicePath, "subsystem")) // e.g., ../../../bus/pci
	if err != nil {
		return ""
	}

	if !strings.HasSuffix(deviceLink, "../../../bus/pci") {
		return "" // Not a PCI device.
	}

	return filepath.Base(devicePath) // e.g., 0000:00:1f.6
}

func getNetworkDeviceModaliasData(device string) *deviceModaliasData {
	pciAddress := getNetworkDevicePCIAddress(device)
	if pciAddress == "" {
		return nil
	}

	value, err := os.ReadFile(filepath.Join(pathBusPciDevices, pciAddress, "modalias"))
	if err != nil {
		return nil
	}

	modalias := strings.TrimSpace(string(value))
	if len(modalias) != lengthModAlias {
		return nil
	}

	// e.g, /sys/devices/pci0000:00/0000:00:03.0/0000:03:00.0/modalias
	// -> pci:v000010DEd00001C82sv00001043sd00008613bc03sc00i00
	//
	// pci -- PCI device
	// v000010DE -- PCI vendor ID
	// d00001C82 -- PCI device ID (the product/model ID)
	// sv00001043 -- PCI subsystem vendor ID
	// sd00008613 -- PCI subsystem device ID (subdevice product/model ID)
	// bc03 -- PCI base class
	// sc00 -- PCI subclass
	// i00 -- programming interface

	if strings.ToLower(modalias[0:3]) != "pci" {
		return nil
	}

	vendorID := strings.ToLower(modalias[9:13])
	productID := strings.ToLower(modalias[18:22])
	subvendorID := strings.ToLower(modalias[28:32])
	subproductID := strings.ToLower(modalias[38:42])
	class := strings.ToLower(modalias[44:46])
	subclass := strings.ToLower(modalias[48:50])
	progIface := strings.ToLower(modalias[51:53])

	return &deviceModaliasData{
		vendorID:     vendorID,
		productID:    productID,
		subproductID: subproductID,
		subvendorID:  subvendorID,
		class:        class,
		subclass:     subclass,
		progIface:    progIface,
	}
}
