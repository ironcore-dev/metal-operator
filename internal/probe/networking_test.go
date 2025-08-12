// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"errors"
	"net"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type mockNIC struct {
	interfaces []net.Interface
	addrs      map[string][]net.Addr
	errIface   error
	errAddrs   error
}

func (m *mockNIC) Interfaces() ([]net.Interface, error) {
	return m.interfaces, m.errIface
}

func (m *mockNIC) Addrs(iface *net.Interface) ([]net.Addr, error) {
	if m.errAddrs != nil {
		return nil, m.errAddrs
	}
	return m.addrs[iface.Name], nil
}

type mockNetDeviceData struct {
	model map[string]string
	speed map[string]string
	rev   map[string]string
}

func (m *mockNetDeviceData) GetModel(name string) string {
	return m.model[name]
}
func (m *mockNetDeviceData) GetSpeed(name string) string {
	return m.speed[name]
}
func (m *mockNetDeviceData) GetRevision(name string) string {
	return m.rev[name]
}

var _ = Describe("networking.go", func() {
	Describe("isSLAAC", func() {
		It("should detect SLAAC IPv6 address", func() {
			Expect(isSLAAC("fe80::a00:27ff:fe4e:66a1")).To(BeTrue())
		})
		It("should not detect non-SLAAC IPv6 address", func() {
			Expect(isSLAAC("2001:db8::1")).To(BeFalse())
		})
	})

	Describe("CollectNetworkData", func() {
		var (
			mockNICInst *mockNIC
			mockNDDInst *mockNetDeviceData
			collector   NetworkDataCollector
		)

		BeforeEach(func() {
			mockNICInst = &mockNIC{
				interfaces: []net.Interface{
					{
						Index:        1,
						Name:         "eth0",
						HardwareAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
						Flags:        net.FlagUp,
					},
					{
						Index:        2,
						Name:         "lo",
						HardwareAddr: net.HardwareAddr{},
						Flags:        net.FlagLoopback | net.FlagUp,
					},
					{
						Index:        3,
						Name:         "tun0",
						HardwareAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x56},
						Flags:        net.FlagUp,
					},
					{
						Index:        4,
						Name:         "docker0",
						HardwareAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x57},
						Flags:        net.FlagUp,
					},
					{
						Index:        5,
						Name:         "down0",
						HardwareAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x58},
						Flags:        0,
					},
				},
				addrs: map[string][]net.Addr{
					"eth0": {
						&net.IPNet{IP: net.ParseIP("192.168.1.10")},
						&net.IPNet{IP: net.ParseIP("fe80::a00:27ff:fe4e:66a1")}, // SLAAC
						&net.IPNet{IP: net.ParseIP("2001:db8::1")},              // non-SLAAC
					},
					"lo": {
						&net.IPNet{IP: net.ParseIP("127.0.0.1")},
					},
					"tun0": {
						&net.IPNet{IP: net.ParseIP("10.0.0.1")},
					},
					"docker0": {
						&net.IPNet{IP: net.ParseIP("172.17.0.1")},
					},
					"down0": {
						&net.IPNet{IP: net.ParseIP("192.168.2.10")},
					},
				},
			}
			mockNDDInst = &mockNetDeviceData{
				model: map[string]string{
					"eth0": "8086 15b8",
					"tun0": "8086 15b9",
				},
				speed: map[string]string{
					"eth0": "1000",
					"tun0": "100",
				},
				rev: map[string]string{
					"eth0": "1.0",
					"tun0": "1.1",
				},
			}
			collector = NewNetworkDataCollector(mockNICInst, mockNDDInst)
		})

		FIt("should collect only valid network interfaces and addresses", func() {
			result, err := collector.CollectNetworkData()
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result).To(ConsistOf(
				registry.NetworkInterface{
					Name:       "eth0",
					IPAddress:  "192.168.1.10",
					MACAddress: "00:11:22:33:44:55",
					Model:      "8086 15b8",
					Speed:      "1000",
					Revision:   "1.0",
				},
				registry.NetworkInterface{
					Name:       "eth0",
					IPAddress:  "2001:db8::1",
					MACAddress: "00:11:22:33:44:55",
					Model:      "8086 15b8",
					Speed:      "1000",
					Revision:   "1.0",
				},
			))
		})

		It("should return error if Interfaces fails", func() {
			mockNICInst.errIface = errors.New("iface error")
			_, err := collector.CollectNetworkData()
			Expect(err).To(MatchError("iface error"))
		})

		It("should return error if Addrs fails", func() {
			mockNICInst.errAddrs = errors.New("addrs error")
			_, err := collector.CollectNetworkData()
			Expect(err).To(MatchError("addrs error"))
		})

		It("should skip interfaces without MAC address", func() {
			mockNICInst.interfaces = append(mockNICInst.interfaces, net.Interface{
				Index:        6,
				Name:         "nomac0",
				HardwareAddr: net.HardwareAddr{},
				Flags:        net.FlagUp,
			})
			mockNICInst.addrs["nomac0"] = []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.3.10")}}
			result, err := collector.CollectNetworkData()
			Expect(err).ToNot(HaveOccurred())
			for _, ni := range result {
				Expect(ni.Name).NotTo(Equal("nomac0"))
			}
		})
	})
})
