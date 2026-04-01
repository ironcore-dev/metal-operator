// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package api

// ServerInfo represents information about a server in the data center.
type ServerInfo struct {
	Name         string `json:"name"`
	Rack         string `json:"rack"`
	HeightUnit   int    `json:"heightUnit"`
	Power        string `json:"power"`
	IndicatorLED string `json:"indicatorLED"`
	State        string `json:"state"`

	// Enrichment data from ServerMetadata (optional, for graceful degradation).
	Enrichment map[string]string `json:"enrichment,omitempty"`

	// Location info derived from well-known enrichment keys.
	Location *LocationInfo `json:"location,omitempty"`

	// Hardware metadata from ServerMetadata.
	Hardware *HardwareInfo `json:"hardware,omitempty"`
}

// LocationInfo represents parsed location hierarchy from enrichment data.
type LocationInfo struct {
	Site          string   `json:"site,omitempty"`
	Building      string   `json:"building,omitempty"`
	Room          string   `json:"room,omitempty"`
	RackName      string   `json:"rackName,omitempty"`
	Position      string   `json:"position,omitempty"`
	HierarchyPath []string `json:"hierarchyPath,omitempty"`
}

// HardwareInfo represents aggregated hardware metadata from ServerMetadata.
type HardwareInfo struct {
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	SerialNumber string `json:"serialNumber,omitempty"`
	UUID         string `json:"uuid,omitempty"`

	BIOSVersion string `json:"biosVersion,omitempty"`
	BIOSVendor  string `json:"biosVendor,omitempty"`

	TotalCPUs    int    `json:"totalCpus,omitempty"`
	CPUModel     string `json:"cpuModel,omitempty"`
	TotalCores   uint32 `json:"totalCores,omitempty"`
	TotalThreads uint32 `json:"totalThreads,omitempty"`

	TotalMemoryGB int `json:"totalMemoryGb,omitempty"`
	MemoryModules int `json:"memoryModules,omitempty"`

	TotalStorageGB int `json:"totalStorageGb,omitempty"`
	StorageDevices int `json:"storageDevices,omitempty"`

	NetworkInterfaces int `json:"networkInterfaces,omitempty"`
	PCIDeviceCount    int `json:"pciDeviceCount,omitempty"`
}
