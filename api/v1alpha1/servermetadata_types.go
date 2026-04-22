// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=smd
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServerMetadata is a flat data object (no spec/status) that persists the full
// probe agent discovery payload. Similar to how Endpoints or ConfigMap store
// data directly at the root level. The relationship to its Server is
// established by using the same name and an owner reference.
type ServerMetadata struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// SystemInfo contains BIOS, system, and board information from DMI/SMBIOS.
	SystemInfo MetaDataSystemInfo `json:"systemInfo,omitempty"`

	// CPU is a list of CPUs discovered on the server.
	CPU []MetaDataCPU `json:"cpu,omitempty"`

	// NetworkInterfaces is a list of network interfaces discovered on the server.
	NetworkInterfaces []MetaDataNetworkInterface `json:"networkInterfaces,omitempty"`

	// LLDP contains LLDP neighbor information per interface.
	LLDP []MetaDataLLDPInterface `json:"lldp,omitempty"`

	// Storage is a list of block devices discovered on the server.
	Storage []MetaDataBlockDevice `json:"storage,omitempty"`

	// Memory is a list of memory devices discovered on the server.
	Memory []MetaDataMemoryDevice `json:"memory,omitempty"`

	// NICs is a list of raw NIC details (PCI address, speed, firmware).
	NICs []MetaDataNIC `json:"nics,omitempty"`

	// PCIDevices is a list of PCI devices discovered on the server.
	PCIDevices []MetaDataPCIDevice `json:"pciDevices,omitempty"`
}

// +kubebuilder:object:root=true

// ServerMetadataList contains a list of ServerMetadata.
type ServerMetadataList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerMetadata `json:"items"`
}

type MetaDataSystemInfo struct {
	BIOSInformation   MetaDataBIOSInformation   `json:"biosInformation,omitempty"`
	SystemInformation MetaDataServerInformation `json:"systemInformation,omitempty"`
	BoardInformation  MetaDataBoardInformation  `json:"boardInformation,omitempty"`
}

type MetaDataBIOSInformation struct {
	Vendor  string `json:"vendor,omitempty"`
	Version string `json:"version,omitempty"`
	Date    string `json:"date,omitempty"`
}

type MetaDataServerInformation struct {
	Manufacturer string `json:"manufacturer,omitempty"`
	ProductName  string `json:"productName,omitempty"`
	Version      string `json:"version,omitempty"`
	SerialNumber string `json:"serialNumber,omitempty"`
	UUID         string `json:"uuid,omitempty"`
	SKUNumber    string `json:"skuNumber,omitempty"`
	Family       string `json:"family,omitempty"`
}

type MetaDataBoardInformation struct {
	Manufacturer string `json:"manufacturer,omitempty"`
	Product      string `json:"product,omitempty"`
	Version      string `json:"version,omitempty"`
	SerialNumber string `json:"serialNumber,omitempty"`
	AssetTag     string `json:"assetTag,omitempty"`
}

type MetaDataCPU struct {
	ID                   int      `json:"id"`
	TotalCores           uint32   `json:"totalCores,omitempty"`
	TotalHardwareThreads uint32   `json:"totalHardwareThreads,omitempty"`
	Vendor               string   `json:"vendor,omitempty"`
	Model                string   `json:"model,omitempty"`
	Capabilities         []string `json:"capabilities,omitempty"`
}

type MetaDataNetworkInterface struct {
	Name          string   `json:"name"`
	IPAddresses   []string `json:"ipAddresses,omitempty"`
	MACAddress    string   `json:"macAddress"`
	CarrierStatus string   `json:"carrierStatus,omitempty"`
}

type MetaDataLLDPInterface struct {
	Name      string                 `json:"name"`
	Neighbors []MetaDataLLDPNeighbor `json:"neighbors,omitempty"`
}

type MetaDataLLDPNeighbor struct {
	ChassisID         string   `json:"chassisId,omitempty"`
	PortID            string   `json:"portId,omitempty"`
	PortDescription   string   `json:"portDescription,omitempty"`
	SystemName        string   `json:"systemName,omitempty"`
	SystemDescription string   `json:"systemDescription,omitempty"`
	MgmtIP            string   `json:"mgmtIp,omitempty"`
	Capabilities      []string `json:"capabilities,omitempty"`
	VlanID            string   `json:"vlanId,omitempty"`
}

type MetaDataBlockDevice struct {
	Path              string `json:"path,omitempty"`
	Name              string `json:"name,omitempty"`
	Rotational        bool   `json:"rotational,omitempty"`
	Removable         bool   `json:"removable,omitempty"`
	ReadOnly          bool   `json:"readOnly,omitempty"`
	Vendor            string `json:"vendor,omitempty"`
	Model             string `json:"model,omitempty"`
	Serial            string `json:"serial,omitempty"`
	WWID              string `json:"wwid,omitempty"`
	PhysicalBlockSize uint64 `json:"physicalBlockSize,omitempty"`
	LogicalBlockSize  uint64 `json:"logicalBlockSize,omitempty"`
	HWSectorSize      uint64 `json:"hWSectorSize,omitempty"`
	SizeBytes         uint64 `json:"sizeBytes,omitempty"`
	NUMANodeID        int    `json:"numaNodeID,omitempty"`
}

type MetaDataMemoryDevice struct {
	SizeBytes             int64  `json:"size,omitempty"`
	DeviceSet             string `json:"deviceSet,omitempty"`
	DeviceLocator         string `json:"deviceLocator,omitempty"`
	BankLocator           string `json:"bankLocator,omitempty"`
	MemoryType            string `json:"memoryType,omitempty"`
	Speed                 string `json:"speed,omitempty"`
	Vendor                string `json:"vendor,omitempty"`
	SerialNumber          string `json:"serialNumber,omitempty"`
	AssetTag              string `json:"assetTag,omitempty"`
	PartNumber            string `json:"partNumber,omitempty"`
	ConfiguredMemorySpeed string `json:"configuredMemorySpeed,omitempty"`
	MinimumVoltage        string `json:"minimumVoltage,omitempty"`
	MaximumVoltage        string `json:"maximumVoltage,omitempty"`
	ConfiguredVoltage     string `json:"configuredVoltage,omitempty"`
}

type MetaDataNIC struct {
	Name            string   `json:"name,omitempty"`
	MAC             string   `json:"mac,omitempty"`
	PCIAddress      string   `json:"pciAddress,omitempty"`
	Speed           string   `json:"speed,omitempty"`
	LinkModes       []string `json:"linkModes,omitempty"`
	SupportedPorts  []string `json:"supportedPorts,omitempty"`
	FirmwareVersion string   `json:"firmwareVersion,omitempty"`
}

type MetaDataPCIDevice struct {
	Address    string `json:"address,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	VendorID   string `json:"vendorID,omitempty"`
	Product    string `json:"product,omitempty"`
	ProductID  string `json:"productID,omitempty"`
	NumaNodeID int    `json:"numaNodeID,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ServerMetadata{}, &ServerMetadataList{})
}
