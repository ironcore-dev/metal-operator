// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"github.com/jaypipes/ghw"
	"github.com/jaypipes/pcidb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("networkDeviceData", func() {
	var (
		netInfo *ghw.NetworkInfo
		pciInfo *ghw.PCIInfo
		data    *networkDeviceData
	)

	BeforeEach(func() {
		netInfo = &ghw.NetworkInfo{
			NICs: []*ghw.NIC{
				{
					Name:       "eth0",
					Speed:      "1000Mb/s",
					PCIAddress: ptrTo("0000:00:1f.6"),
				},
				{
					Name:       "eth1",
					Speed:      "100Mb/s",
					PCIAddress: ptrTo("0000:00:1f.7"),
				},
			},
		}
		pciInfo = &ghw.PCIInfo{
			Devices: []*ghw.PCIDevice{
				{
					Address:  "0000:00:1f.6",
					Vendor:   &pcidb.Vendor{Name: "Intel"},
					Product:  &pcidb.Product{Name: "Ethernet Controller"},
					Revision: "01",
				},
				{
					Address:  "0000:00:1f.7",
					Vendor:   &pcidb.Vendor{Name: "Realtek"},
					Product:  &pcidb.Product{Name: "RTL8111/8168/8411"},
					Revision: "02",
				},
			},
		}
		data = &networkDeviceData{
			netInfo: netInfo,
			pciInfo: pciInfo,
		}
	})

	Describe("GetModel", func() {
		It("returns correct model for eth0", func() {
			Expect(data.GetModel("eth0")).To(Equal("Intel Ethernet Controller"))
		})
		It("returns correct model for eth1", func() {
			Expect(data.GetModel("eth1")).To(Equal("Realtek RTL8111/8168/8411"))
		})
		It("returns empty string for unknown iface", func() {
			Expect(data.GetModel("eth2")).To(Equal(""))
		})
	})

	Describe("GetSpeed", func() {
		It("returns correct speed for eth0", func() {
			Expect(data.GetSpeed("eth0")).To(Equal("1000Mb/s"))
		})
		It("returns correct speed for eth1", func() {
			Expect(data.GetSpeed("eth1")).To(Equal("100Mb/s"))
		})
		It("returns empty string for unknown iface", func() {
			Expect(data.GetSpeed("eth2")).To(Equal(""))
		})
	})

	Describe("GetRevision", func() {
		It("returns correct revision for eth0", func() {
			Expect(data.GetRevision("eth0")).To(Equal("01"))
		})
		It("returns correct revision for eth1", func() {
			Expect(data.GetRevision("eth1")).To(Equal("02"))
		})
		It("returns empty string for unknown iface", func() {
			Expect(data.GetRevision("eth2")).To(Equal(""))
		})
	})

	Describe("findPCIAddressByInterfaceName", func() {
		It("returns correct PCI address for eth0", func() {
			Expect(data.findPCIAddressByInterfaceName("eth0")).To(Equal("0000:00:1f.6"))
		})
		It("returns empty string for unknown iface", func() {
			Expect(data.findPCIAddressByInterfaceName("eth2")).To(Equal(""))
		})
	})

	Describe("findNICByInterfaceName", func() {
		It("returns correct NIC for eth0", func() {
			nic := data.findNICByInterfaceName("eth0")
			Expect(nic).NotTo(BeNil())
			Expect(nic.Name).To(Equal("eth0"))
		})
		It("returns nil for unknown iface", func() {
			Expect(data.findNICByInterfaceName("eth2")).To(BeNil())
		})
	})
})

func ptrTo(s string) *string {
	return &s
}
