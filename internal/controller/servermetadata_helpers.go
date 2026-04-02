// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

// registryToServerMetadata converts a registry.Server into root-level fields on a ServerMetadata object.
func registryToServerMetadata(regServer *registry.Server, meta *metalv1alpha1.ServerMetadata) {
	// SystemInfo
	meta.SystemInfo = metalv1alpha1.MetaDataSystemInfo{
		BIOSInformation: metalv1alpha1.MetaDataBIOSInformation{
			Vendor:  regServer.SystemInfo.BIOSInformation.Vendor,
			Version: regServer.SystemInfo.BIOSInformation.Version,
			Date:    regServer.SystemInfo.BIOSInformation.Date,
		},
		SystemInformation: metalv1alpha1.MetaDataServerInformation{
			Manufacturer: regServer.SystemInfo.SystemInformation.Manufacturer,
			ProductName:  regServer.SystemInfo.SystemInformation.ProductName,
			Version:      regServer.SystemInfo.SystemInformation.Version,
			SerialNumber: regServer.SystemInfo.SystemInformation.SerialNumber,
			UUID:         regServer.SystemInfo.SystemInformation.UUID,
			SKUNumber:    regServer.SystemInfo.SystemInformation.SKUNumber,
			Family:       regServer.SystemInfo.SystemInformation.Family,
		},
		BoardInformation: metalv1alpha1.MetaDataBoardInformation{
			Manufacturer: regServer.SystemInfo.BoardInformation.Manufacturer,
			Product:      regServer.SystemInfo.BoardInformation.Product,
			Version:      regServer.SystemInfo.BoardInformation.Version,
			SerialNumber: regServer.SystemInfo.BoardInformation.SerialNumber,
			AssetTag:     regServer.SystemInfo.BoardInformation.AssetTag,
		},
	}

	// CPU
	meta.CPU = make([]metalv1alpha1.MetaDataCPU, 0, len(regServer.CPU))
	for _, c := range regServer.CPU {
		meta.CPU = append(meta.CPU, metalv1alpha1.MetaDataCPU{
			ID:                   c.ID,
			TotalCores:           c.TotalCores,
			TotalHardwareThreads: c.TotalHardwareThreads,
			Vendor:               c.Vendor,
			Model:                c.Model,
			Capabilities:         c.Capabilities,
		})
	}

	// NetworkInterfaces
	meta.NetworkInterfaces = make([]metalv1alpha1.MetaDataNetworkInterface, 0, len(regServer.NetworkInterfaces))
	for _, ni := range regServer.NetworkInterfaces {
		meta.NetworkInterfaces = append(meta.NetworkInterfaces, metalv1alpha1.MetaDataNetworkInterface{
			Name:          ni.Name,
			IPAddresses:   ni.IPAddresses,
			MACAddress:    ni.MACAddress,
			CarrierStatus: ni.CarrierStatus,
		})
	}

	// LLDP
	meta.LLDP = make([]metalv1alpha1.MetaDataLLDPInterface, 0, len(regServer.LLDP))
	for _, li := range regServer.LLDP {
		iface := metalv1alpha1.MetaDataLLDPInterface{
			Name: li.Name,
		}
		for _, n := range li.Neighbors {
			iface.Neighbors = append(iface.Neighbors, metalv1alpha1.MetaDataLLDPNeighbor{
				ChassisID:         n.ChassisID,
				PortID:            n.PortID,
				PortDescription:   n.PortDescription,
				SystemName:        n.SystemName,
				SystemDescription: n.SystemDescription,
				MgmtIP:            n.MgmtIP,
				Capabilities:      n.Capabilities,
				VlanID:            n.VlanID,
			})
		}
		meta.LLDP = append(meta.LLDP, iface)
	}

	// Storage
	meta.Storage = make([]metalv1alpha1.MetaDataBlockDevice, 0, len(regServer.Storage))
	for _, bd := range regServer.Storage {
		meta.Storage = append(meta.Storage, metalv1alpha1.MetaDataBlockDevice{
			Path:              bd.Path,
			Name:              bd.Name,
			Rotational:        bd.Rotational,
			Removable:         bd.Removable,
			ReadOnly:          bd.ReadOnly,
			Vendor:            bd.Vendor,
			Model:             bd.Model,
			Serial:            bd.Serial,
			WWID:              bd.WWID,
			PhysicalBlockSize: bd.PhysicalBlockSize,
			LogicalBlockSize:  bd.LogicalBlockSize,
			HWSectorSize:      bd.HWSectorSize,
			SizeBytes:         bd.SizeBytes,
			NUMANodeID:        bd.NUMANodeID,
		})
	}

	// Memory
	meta.Memory = make([]metalv1alpha1.MetaDataMemoryDevice, 0, len(regServer.Memory))
	for _, m := range regServer.Memory {
		meta.Memory = append(meta.Memory, metalv1alpha1.MetaDataMemoryDevice{
			SizeBytes:             m.SizeBytes,
			DeviceSet:             m.DeviceSet,
			DeviceLocator:         m.DeviceLocator,
			BankLocator:           m.BankLocator,
			MemoryType:            m.MemoryType,
			Speed:                 m.Speed,
			Vendor:                m.Vendor,
			SerialNumber:          m.SerialNumber,
			AssetTag:              m.AssetTag,
			PartNumber:            m.PartNumber,
			ConfiguredMemorySpeed: m.ConfiguredMemorySpeed,
			MinimumVoltage:        m.MinimumVoltage,
			MaximumVoltage:        m.MaximumVoltage,
			ConfiguredVoltage:     m.ConfiguredVoltage,
		})
	}

	// NICs
	meta.NICs = make([]metalv1alpha1.MetaDataNIC, 0, len(regServer.NICs))
	for _, n := range regServer.NICs {
		meta.NICs = append(meta.NICs, metalv1alpha1.MetaDataNIC{
			Name:            n.Name,
			MAC:             n.MAC,
			PCIAddress:      n.PCIAddress,
			Speed:           n.Speed,
			LinkModes:       n.LinkModes,
			SupportedPorts:  n.SupportedPorts,
			FirmwareVersion: n.FirmwareVersion,
		})
	}

	// PCIDevices
	meta.PCIDevices = make([]metalv1alpha1.MetaDataPCIDevice, 0, len(regServer.PCIDevices))
	for _, pd := range regServer.PCIDevices {
		meta.PCIDevices = append(meta.PCIDevices, metalv1alpha1.MetaDataPCIDevice{
			Address:    pd.Address,
			Vendor:     pd.Vendor,
			VendorID:   pd.VendorID,
			Product:    pd.Product,
			ProductID:  pd.ProductID,
			NumaNodeID: pd.NumaNodeID,
		})
	}
}

// metaDataToNetworkInterfaces converts ServerMetadata network interface and LLDP data
// into the Server.Status.NetworkInterfaces format.
func metaDataToNetworkInterfaces(log interface {
	Error(err error, msg string, keysAndValues ...any)
}, meta *metalv1alpha1.ServerMetadata) []metalv1alpha1.NetworkInterface {
	nics := make([]metalv1alpha1.NetworkInterface, 0, len(meta.NetworkInterfaces))
	for _, ni := range meta.NetworkInterfaces {
		nic := metalv1alpha1.NetworkInterface{
			Name:          ni.Name,
			MACAddress:    ni.MACAddress,
			CarrierStatus: ni.CarrierStatus,
		}

		var allIPs []metalv1alpha1.IP
		for _, ipAddr := range ni.IPAddresses {
			if ipAddr != "" {
				ip, err := metalv1alpha1.ParseIP(ipAddr)
				if err != nil {
					log.Error(err, "Invalid IP address in ServerMetadata, skipping", "interface", ni.Name, "ip", ipAddr)
					continue
				}
				allIPs = append(allIPs, ip)
			}
		}
		nic.IPs = allIPs
		nics = append(nics, nic)
	}

	// Merge LLDP neighbors into corresponding network interfaces
	for _, lldpIface := range meta.LLDP {
		for i := range nics {
			if nics[i].Name == lldpIface.Name {
				neighbors := make([]metalv1alpha1.LLDPNeighbor, 0, len(lldpIface.Neighbors))
				for _, neighbor := range lldpIface.Neighbors {
					neighbors = append(neighbors, metalv1alpha1.LLDPNeighbor{
						MACAddress:        neighbor.ChassisID,
						PortID:            neighbor.PortID,
						PortDescription:   neighbor.PortDescription,
						SystemName:        neighbor.SystemName,
						SystemDescription: neighbor.SystemDescription,
					})
				}
				nics[i].Neighbors = neighbors
				break
			}
		}
	}

	return nics
}
