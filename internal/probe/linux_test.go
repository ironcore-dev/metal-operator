// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Linux network device probe functions", func() {
	var (
		tmpSysClassNet   string
		tmpBusPciDevices string
		tmpSysDevices    string

		cleanup func()
	)

	const (
		deviceName = "eth0"
		pciID      = "pci0000:00"
		pciAddress = "0000:00:1f.6"
	)

	BeforeEach(func() {
		// Setup temporary directories to simulate /sys/class/net and /bus/pci/devices
		tmpDir := GinkgoT().TempDir()

		tmpSysClassNet = filepath.Join(tmpDir, "sys", "class", "net")
		Expect(os.MkdirAll(tmpSysClassNet, 0755)).To(Succeed())

		tmpBusPciDevices = filepath.Join(tmpDir, "bus", "pci", "devices")
		Expect(os.MkdirAll(tmpBusPciDevices, 0755)).To(Succeed())

		tmpSysDevices = filepath.Join(tmpDir, "sys", "devices")
		Expect(os.MkdirAll(tmpSysDevices, 0755)).To(Succeed())

		// Patch probe package constants for test
		probePathSysClassNet := pathSysClassNet
		probePathBusPciDevices := pathBusPciDevices
		pathSysClassNet = tmpSysClassNet
		pathBusPciDevices = tmpBusPciDevices

		cleanup = func() {
			pathSysClassNet = probePathSysClassNet
			pathBusPciDevices = probePathBusPciDevices
		}
	})

	AfterEach(func() {
		if cleanup != nil {
			cleanup()
		}
	})

	Describe("getNetworkDeviceSpeed", func() {
		BeforeEach(func() {
			sysDevicesNetDir := filepath.Join(tmpSysDevices, pciID, pciAddress, "net", deviceName)
			Expect(os.MkdirAll(sysDevicesNetDir, 0755)).To(Succeed())

			netDeviceNetPath := filepath.Join(tmpSysClassNet, deviceName)
			symLink := fmt.Sprintf("../../devices/%s/%s/net/%s", pciID, pciAddress, deviceName)
			Expect(os.Symlink(symLink, netDeviceNetPath)).To(Succeed())
		})

		It("returns speed value", func() {
			Expect(os.WriteFile(filepath.Join(tmpSysClassNet, deviceName, "speed"), []byte("1000\n"), 0644)).To(Succeed())
			Expect(getNetworkDeviceSpeed(deviceName)).To(Equal("1000"))
		})

		It("returns empty string if speed file is missing", func() {
			Expect(getNetworkDeviceSpeed(deviceName)).To(BeEmpty())
		})
	})

	Describe("getNetworkDevicePath", func() {
		It("returns PCI device path for physical device", func() {
			sysDevicesNetDir := filepath.Join(tmpSysDevices, pciID, pciAddress, "net", deviceName)
			Expect(os.MkdirAll(sysDevicesNetDir, 0755)).To(Succeed())

			netDeviceNetPath := filepath.Join(tmpSysClassNet, deviceName)
			symLink := fmt.Sprintf("../../devices/%s/%s/net/%s", pciID, pciAddress, deviceName)
			Expect(os.Symlink(symLink, netDeviceNetPath)).To(Succeed())

			sysDeviceNetPath := filepath.Join(sysDevicesNetDir, "device")
			symLink = fmt.Sprintf("../../../%s", pciAddress)
			Expect(os.Symlink(symLink, sysDeviceNetPath)).To(Succeed())

			Expect(getNetworkDevicePath(deviceName)).To(HaveSuffix("/sys/devices/pci0000:00/0000:00:1f.6"))
		})

		It("returns empty string for virtual device", func() {
			netDevicePath := filepath.Join(tmpSysClassNet, deviceName)
			target := fmt.Sprintf("../../devices/virtual/net/%s", deviceName)
			Expect(os.Symlink(target, netDevicePath)).To(Succeed())
			Expect(getNetworkDevicePath(deviceName)).To(BeEmpty())
		})

		It("returns empty string if symlink is missing", func() {
			Expect(getNetworkDevicePath(deviceName)).To(BeEmpty())
		})

		It("returns empty string if device symlink is missing", func() {
			sysDevicesDir := filepath.Join(tmpSysDevices, pciID, pciAddress, "net", deviceName)
			Expect(os.MkdirAll(sysDevicesDir, 0755)).To(Succeed())

			netDevicePath := filepath.Join(tmpSysClassNet, deviceName)
			symLink := fmt.Sprintf("../../devices/%s/%s/net/%s", pciID, pciAddress, deviceName)
			Expect(os.Symlink(symLink, netDevicePath)).To(Succeed())

			Expect(getNetworkDevicePath(deviceName)).To(BeEmpty())
		})
	})

	Describe("getNetworkDevicePCIAddress", func() {
		BeforeEach(func() {
			sysDevicesNetDir := filepath.Join(tmpSysDevices, pciID, pciAddress, "net", deviceName)
			Expect(os.MkdirAll(sysDevicesNetDir, 0755)).To(Succeed())

			netDeviceNetPath := filepath.Join(tmpSysClassNet, deviceName)
			symLink := fmt.Sprintf("../../devices/%s/%s/net/%s", pciID, pciAddress, deviceName)
			Expect(os.Symlink(symLink, netDeviceNetPath)).To(Succeed())

			sysDeviceNetPath := filepath.Join(sysDevicesNetDir, "device")
			symLink = fmt.Sprintf("../../../%s", pciAddress)
			Expect(os.Symlink(symLink, sysDeviceNetPath)).To(Succeed())
		})

		It("returns PCI address for valid device", func() {
			sysDevicePath := filepath.Join(tmpSysDevices, pciID, pciAddress)
			Expect(os.Symlink("../../../bus/pci", filepath.Join(sysDevicePath, "subsystem"))).To(Succeed())

			Expect(getNetworkDevicePCIAddress(deviceName)).To(Equal("0000:00:1f.6"))
		})

		It("returns empty string if not PCI device", func() {
			sysDevicePath := filepath.Join(tmpSysDevices, pciID, pciAddress)
			Expect(os.Symlink("../../../bus/usb", filepath.Join(sysDevicePath, "subsystem"))).To(Succeed())

			Expect(getNetworkDevicePCIAddress(deviceName)).To(BeEmpty())
		})
	})

	Describe("getNetworkDeviceModaliasData", func() {
		It("returns nil if PCI address is not found", func() {
			Expect(getNetworkDeviceModaliasData(deviceName)).To(BeNil())
		})

		Context("when modalias file exists", func() {
			var pciDeviceDir string

			BeforeEach(func() {
				sysDevicesNetDir := filepath.Join(tmpSysDevices, pciID, pciAddress, "net", deviceName)
				Expect(os.MkdirAll(sysDevicesNetDir, 0755)).To(Succeed())

				netDeviceNetPath := filepath.Join(tmpSysClassNet, deviceName)
				symLink := fmt.Sprintf("../../devices/%s/%s/net/%s", pciID, pciAddress, deviceName)
				Expect(os.Symlink(symLink, netDeviceNetPath)).To(Succeed())

				sysDeviceNetPath := filepath.Join(sysDevicesNetDir, "device")
				symLink = fmt.Sprintf("../../../%s", pciAddress)
				Expect(os.Symlink(symLink, sysDeviceNetPath)).To(Succeed())

				sysDevicePath := filepath.Join(tmpSysDevices, pciID, pciAddress)
				Expect(os.Symlink("../../../bus/pci", filepath.Join(sysDevicePath, "subsystem"))).To(Succeed())

				pciDeviceDir = filepath.Join(tmpBusPciDevices, "0000:00:1f.6")
				Expect(os.MkdirAll(pciDeviceDir, 0755)).To(Succeed())
			})

			It("returns modalias data for valid PCI device", func() {
				modalStr := "pci:v000010DEd00001C82sv00001043sd00008613bc03sc00i00"
				Expect(modalStr).To(HaveLen(53))
				Expect(os.WriteFile(filepath.Join(pciDeviceDir, "modalias"), []byte(modalStr), 0644)).To(Succeed())

				data := getNetworkDeviceModaliasData(deviceName)

				Expect(data).NotTo(BeNil())
				Expect(data.vendorID).To(Equal("10de"))
				Expect(data.productID).To(Equal("1c82"))
				Expect(data.subvendorID).To(Equal("1043"))
				Expect(data.subproductID).To(Equal("8613"))
				Expect(data.class).To(Equal("03"))
				Expect(data.subclass).To(Equal("00"))
				Expect(data.progIface).To(Equal("00"))
			})

			It("returns nil if modalias file is missing", func() {
				Expect(getNetworkDeviceModaliasData(deviceName)).To(BeNil())
			})

			It("returns nil if modalias string wrong length", func() {
				modalStr := "pci:tooshort"
				Expect(os.WriteFile(filepath.Join(pciDeviceDir, "modalias"), []byte(modalStr), 0644)).To(Succeed())

				Expect(getNetworkDeviceModaliasData(deviceName)).To(BeNil())
			})

			It("returns nil if modalias string not for pci device", func() {
				modalStr := "usb:v1D6Bp0001d0206dc09dsc00dp00ic09isc00ip00"
				Expect(os.WriteFile(filepath.Join(pciDeviceDir, "modalias"), []byte(modalStr), 0644)).To(Succeed())

				Expect(getNetworkDeviceModaliasData(deviceName)).To(BeNil())
			})
		})
	})
})
