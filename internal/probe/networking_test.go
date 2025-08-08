// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNetworking(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Networking Suite")
}

var _ = Describe("Networking", func() {
	var (
		origNetInterfaces                func() ([]net.Interface, error)
		origGetAddrs                     func(net.Interface) ([]net.Addr, error)
		origGetNetworkDevicePCIAddress   func(string) string
		origGetNetworkDeviceSpeed        func(string) string
		origGetNetworkDeviceModaliasData func(string) *deviceModaliasData
	)

	BeforeEach(func() {
		origNetInterfaces = probeNetInterfaces
		origGetAddrs = probeGetAddrs
		origGetNetworkDevicePCIAddress = probeGetNetworkDevicePCIAddress
		origGetNetworkDeviceSpeed = probeGetNetworkDeviceSpeed
		origGetNetworkDeviceModaliasData = probeGetNetworkDeviceModaliasData
	})

	AfterEach(func() {
		probeNetInterfaces = origNetInterfaces
		probeGetAddrs = origGetAddrs
		probeGetNetworkDevicePCIAddress = origGetNetworkDevicePCIAddress
		probeGetNetworkDeviceSpeed = origGetNetworkDeviceSpeed
		probeGetNetworkDeviceModaliasData = origGetNetworkDeviceModaliasData
	})

	Describe("isSLAAC", func() {
		It("should detect SLAAC IPv6 addresses", func() {
			Expect(isSLAAC("fe80::a00:27ff:fe4e:66a1")).To(BeTrue())
			Expect(isSLAAC("192.168.1.1")).To(BeFalse())
			Expect(isSLAAC("fe80::a00:27ab:cd12")).To(BeFalse())
		})
	})

	Describe("collectNetworkData", func() {
		It("should return empty slice when no interfaces", func() {
			probeNetInterfaces = func() ([]net.Interface, error) {
				return []net.Interface{}, nil
			}
			result, err := collectNetworkData()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should propagate error from net.Interfaces", func() {
			probeNetInterfaces = func() ([]net.Interface, error) {
				return nil, errors.New("fail")
			}
			result, err := collectNetworkData()
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should filter out loopback, tun, docker0, down, and SLAAC addresses", func() {
			probeNetInterfaces = func() ([]net.Interface, error) {
				return []net.Interface{
					{Name: "lo", Flags: net.FlagLoopback, HardwareAddr: net.HardwareAddr{0x00}},
					{Name: "tun0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x01}},
					{Name: "docker0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x02}},
					{Name: "down0", Flags: 0, HardwareAddr: net.HardwareAddr{0x03}},
					{Name: "eth0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x04}},
				}, nil
			}
			probeGetAddrs = func(iface net.Interface) ([]net.Addr, error) {
				switch iface.Name {
				case "eth0":
					return []net.Addr{
						&net.IPNet{IP: net.ParseIP("192.168.1.10")},
						&net.IPNet{IP: net.ParseIP("fe80::a00:27ff:fe4e:66a1")},
					}, nil
				default:
					return []net.Addr{}, nil
				}
			}
			probeGetNetworkDevicePCIAddress = func(string) string { return "0000:00:1f.6" }
			probeGetNetworkDeviceSpeed = func(string) string { return "1000" }
			probeGetNetworkDeviceModaliasData = func(string) *deviceModaliasData {
				return &deviceModaliasData{vendorID: "10de", productID: "1c82"}
			}

			result, err := collectNetworkData()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal(registry.NetworkInterface{
				Name:       "eth0",
				IPAddress:  "192.168.1.10",
				MACAddress: "04",
				PCIAddress: "0000:00:1f.6",
				Model:      "10de 1c82",
				Speed:      "1000",
			}))
		})

		It("should handle interface with only IPv6 non-SLAAC address", func() {
			probeNetInterfaces = func() ([]net.Interface, error) {
				return []net.Interface{
					{Name: "eth1", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x05}},
				}, nil
			}
			probeGetAddrs = func(iface net.Interface) ([]net.Addr, error) {
				return []net.Addr{
					&net.IPNet{IP: net.ParseIP("2001:db8::1")},
				}, nil
			}
			probeGetNetworkDevicePCIAddress = func(string) string { return "0000:00:1f.7" }
			probeGetNetworkDeviceSpeed = func(string) string { return "100" }
			probeGetNetworkDeviceModaliasData = func(string) *deviceModaliasData {
				return &deviceModaliasData{vendorID: "abcd", productID: "1234"}
			}

			result, err := collectNetworkData()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].IPAddress).To(Equal("2001:db8::1"))
			Expect(result[0].Model).To(Equal("abcd 1234"))
		})

		It("should skip interface with no addresses", func() {
			probeNetInterfaces = func() ([]net.Interface, error) {
				return []net.Interface{
					{Name: "eth2", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x06}},
				}, nil
			}
			probeGetAddrs = func(iface net.Interface) ([]net.Addr, error) {
				return []net.Addr{}, nil
			}
			result, err := collectNetworkData()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should propagate error from iface.Addrs", func() {
			probeNetInterfaces = func() ([]net.Interface, error) {
				return []net.Interface{
					{Name: "eth3", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x07}},
				}, nil
			}
			probeGetAddrs = func(iface net.Interface) ([]net.Addr, error) {
				return nil, errors.New("addrs error")
			}
			result, err := collectNetworkData()
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should handle nil deviceData gracefully", func() {
			probeNetInterfaces = func() ([]net.Interface, error) {
				return []net.Interface{
					{Name: "eth4", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0x08}},
				}, nil
			}
			probeGetAddrs = func(iface net.Interface) ([]net.Addr, error) {
				return []net.Addr{
					&net.IPNet{IP: net.ParseIP("10.0.0.1")},
				}, nil
			}
			probeGetNetworkDevicePCIAddress = func(string) string { return "0000:00:1f.8" }
			probeGetNetworkDeviceSpeed = func(string) string { return "10000" }
			probeGetNetworkDeviceModaliasData = func(string) *deviceModaliasData {
				return nil
			}

			result, err := collectNetworkData()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].Model).To(Equal(""))
		})
	})
})

// --- Patch helpers for testing ---

var (
	probeNetInterfaces                = net.Interfaces
	probeGetAddrs                     = func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() }
	probeGetNetworkDevicePCIAddress   = getNetworkDevicePCIAddress
	probeGetNetworkDeviceSpeed        = getNetworkDeviceSpeed
	probeGetNetworkDeviceModaliasData = getNetworkDeviceModaliasData
)

// Patch collectNetworkData to use testable helpers
func init() {
	collectNetworkData = func() ([]registry.NetworkInterface, error) {
		interfaces, err := probeNetInterfaces()
		if err != nil {
			return nil, err
		}
		var networkInterfaces []registry.NetworkInterface
		for _, iface := range interfaces {
			if iface.Flags&net.FlagLoopback != 0 ||
				iface.HardwareAddr.String() == "" ||
				strings.HasPrefix(iface.Name, "tun") ||
				strings.HasPrefix(iface.Name, "docker0") ||
				iface.Flags&net.FlagUp == 0 {
				continue
			}
			addrs, err := probeGetAddrs(iface)
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
				if ip.To4() == nil && isSLAAC(ip.String()) {
					continue
				}
				pciAddress := probeGetNetworkDevicePCIAddress(iface.Name)
				speed := probeGetNetworkDeviceSpeed(iface.Name)
				deviceData := probeGetNetworkDeviceModaliasData(iface.Name)
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
}
