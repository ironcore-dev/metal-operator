// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Power defines the possible power states for a device.
type Power string

const (
	// PowerOn indicates that the device is powered on.
	PowerOn Power = "On"

	// PowerOff indicates that the device is powered off.
	PowerOff Power = "Off"

	// TopologyHeightUnit is the annotation key for the height unit of a server in a rack.
	TopologyHeightUnit = "topology.metal.ironcore.dev/heightunit"

	// TopologyRack is the annotation key for the rack of a server.
	TopologyRack = "topology.metal.ironcore.dev/rack"
)

// ServerPowerState defines the possible power states for a server.
type ServerPowerState string

const (
	// ServerOnPowerState indicates that the system is powered on.
	ServerOnPowerState ServerPowerState = "On"

	// ServerOffPowerState indicates that the system is powered off, although some components may
	// continue to have auxiliary power such as the management controller.
	ServerOffPowerState ServerPowerState = "Off"

	// ServerPausedPowerState indicates that the system is paused.
	ServerPausedPowerState ServerPowerState = "Paused"

	// ServerPoweringOnPowerState indicates a temporary state between Off and On.
	// This temporary state can be very short.
	ServerPoweringOnPowerState ServerPowerState = "PoweringOn"

	// ServerPoweringOffPowerState indicates a temporary state between On and Off.
	// The power off action can take time while the OS is in the shutdown process.
	ServerPoweringOffPowerState ServerPowerState = "PoweringOff"
)

// BMCAccess defines the access details for the BMC.
type BMCAccess struct {
	// Protocol specifies the protocol to be used for communicating with the BMC.
	// +required
	Protocol Protocol `json:"protocol"`

	// Address is the address of the BMC.
	// +required
	Address string `json:"address"`

	// BMCSecretRef is a reference to the BMCSecret object that contains the credentials
	// required to access the BMC.
	// +required
	BMCSecretRef v1.LocalObjectReference `json:"bmcSecretRef"`
}

// BootOrder represents the boot order of the server.
type BootOrder struct {
	// Name is the name of the boot device.
	Name string `json:"name"`
	// Priority is the priority of the boot device.
	Priority int `json:"priority"`
	// Device is the device to boot from.
	Device string `json:"device"`
}

// ServerSpec defines the desired state of a Server.
type ServerSpec struct {
	// UUID is the unique identifier for the server.
	// Deprecated in favor of systemUUID.
	// +optional
	UUID string `json:"uuid,omitempty"`

	// SystemUUID is the unique identifier for the server.
	// If not provided, it will be derived from the serial
	// +optional
	SystemUUID string `json:"systemUUID"`

	// SystemURI is the unique URI for the server resource in REDFISH API.
	SystemURI string `json:"systemURI,omitempty"`

	// Power specifies the desired power state of the server.
	// +optional
	Power Power `json:"power,omitempty"`

	// IndicatorLED specifies the desired state of the server's indicator LED.
	// +optional
	IndicatorLED IndicatorLED `json:"indicatorLED,omitempty"`

	// ServerClaimRef is a reference to a ServerClaim object that claims this server.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == null || oldSelf == null || self == oldSelf",message="serverClaimRef cannot be switched directly"
	// +optional
	ServerClaimRef *ObjectReference `json:"serverClaimRef,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that maintains this server.
	// +optional
	ServerMaintenanceRef *ObjectReference `json:"serverMaintenanceRef,omitempty"`

	// BMCRef is a reference to the BMC object associated with this server.
	// +optional
	BMCRef *v1.LocalObjectReference `json:"bmcRef,omitempty"`

	// BMC contains the access details for the BMC.
	// +optional
	BMC *BMCAccess `json:"bmc,omitempty"`

	// BootConfigurationRef is a reference to a BootConfiguration object that specifies
	// the boot configuration for this server.
	// +optional
	BootConfigurationRef *ObjectReference `json:"bootConfigurationRef,omitempty"`

	// MaintenanceBootConfigurationRef is a reference to a BootConfiguration object that specifies
	// the boot configuration for this server during maintenance.
	// +optional
	MaintenanceBootConfigurationRef *ObjectReference `json:"maintenanceBootConfigurationRef,omitempty"`

	// BootOrder specifies the boot order of the server.
	// +optional
	BootOrder []BootOrder `json:"bootOrder,omitempty"`

	// BIOSSettingsRef is a reference to a biossettings object that specifies
	// the BIOS configuration for this server.
	// +optional
	BIOSSettingsRef *v1.LocalObjectReference `json:"biosSettingsRef,omitempty"`
}

// ServerState defines the possible states of a server.
type ServerState string

const (
	// ServerStateInitial indicates that the server is in its initial state.
	ServerStateInitial ServerState = "Initial"

	// ServerStateDiscovery indicates that the server is in its discovery state.
	ServerStateDiscovery ServerState = "Discovery"

	// ServerStateAvailable indicates that the server is available for use.
	ServerStateAvailable ServerState = "Available"

	// ServerStateReserved indicates that the server is reserved for a specific use or user.
	ServerStateReserved ServerState = "Reserved"

	// ServerStateError indicates that there is an error with the server.
	ServerStateError ServerState = "Error"

	// ServerStateMaintenance indicates that the server is in maintenance.
	ServerStateMaintenance ServerState = "Maintenance"
)

// IndicatorLED represents LED indicator states
type IndicatorLED string

const (
	// UnknownIndicatorLED indicates the state of the Indicator LED cannot be
	// determined.
	UnknownIndicatorLED IndicatorLED = "Unknown"
	// LitIndicatorLED indicates the Indicator LED is lit.
	LitIndicatorLED IndicatorLED = "Lit"
	// BlinkingIndicatorLED indicates the Indicator LED is blinking.
	BlinkingIndicatorLED IndicatorLED = "Blinking"
	// OffIndicatorLED indicates the Indicator LED is off.
	OffIndicatorLED IndicatorLED = "Off"
)

// StorageState represents Storage states
type StorageState string

const (
	// StorageStateEnabled indicates that the storage device is enabled.
	StorageStateEnabled StorageState = "Enabled"
	// StorageStateDisabled indicates that the storage device is disabled.
	StorageStateDisabled StorageState = "Disabled"
	// StorageStateAbsent indicates that the storage device is absent.
	StorageStateAbsent StorageState = "Absent"
)

// ServerStatus defines the observed state of Server.
type ServerStatus struct {
	// Manufacturer is the name of the server manufacturer.
	// +optional
	Manufacturer string `json:"manufacturer,omitempty"`

	// BIOSVersion is the version of the server's BIOS.
	BIOSVersion string `json:"biosVersion,omitempty"`

	// Model is the model of the server.
	// +optional
	Model string `json:"model,omitempty"`

	// SKU is the stock keeping unit identifier for the server.
	// +optional
	SKU string `json:"sku,omitempty"`

	// SerialNumber is the serial number of the server.
	// +optional
	SerialNumber string `json:"serialNumber,omitempty"`

	// PowerState represents the current power state of the server.
	// +optional
	PowerState ServerPowerState `json:"powerState,omitempty"`

	// IndicatorLED specifies the current state of the server's indicator LED.
	// +optional
	IndicatorLED IndicatorLED `json:"indicatorLED,omitempty"`

	// State represents the current state of the server.
	// +optional
	State ServerState `json:"state,omitempty"`

	// NetworkInterfaces is a list of network interfaces associated with the server.
	// +optional
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`

	// TotalSystemMemory is the total amount of memory in bytes available on the server.
	// +optional
	TotalSystemMemory *resource.Quantity `json:"totalSystemMemory,omitempty"`

	// Processors is a list of Processors associated with the server.
	// +optional
	Processors []Processor `json:"processors,omitempty"`

	// Storages is a list of storages associated with the server.
	// +optional
	Storages []Storage `json:"storages,omitempty"`

	// Conditions represents the latest available observations of the server's current state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// Processor defines the details of a Processor.
type Processor struct {
	// ID is the name of the Processor.
	// +required
	ID string `json:"id"`

	// Type is the type of the Processor.
	// +optional
	Type string `json:"type,omitempty"`

	// Architecture is the architecture of the Processor.
	// +optional
	Architecture string `json:"architecture,omitempty"`

	// InstructionSet is the instruction set of the Processor.
	// +optional
	InstructionSet string `json:"instructionSet,omitempty"`

	// Manufacturer is the manufacturer of the Processor.
	// +optional
	Manufacturer string `json:"manufacturer,omitempty"`

	// Model is the model of the Processor.
	// +optional
	Model string `json:"model,omitempty"`

	// MaxSpeedMHz is the maximum speed of the Processor in MHz.
	// +optional
	MaxSpeedMHz int32 `json:"maxSpeedMHz,omitempty"`

	// TotalCores is the total number of cores in the Processor.
	// +optional
	TotalCores int32 `json:"totalCores,omitempty"`

	// TotalThreads is the total number of threads in the Processor.
	// +optional
	TotalThreads int32 `json:"totalThreads,omitempty"`
}

// NetworkInterface defines the details of a network interface.
type NetworkInterface struct {
	// Name is the name of the network interface.
	// +required
	Name string `json:"name"`

	// IP is the IP address assigned to the network interface.
	// Deprecated: Use IPs instead. Kept for backward compatibility, always nil.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Schemaless
	// +optional
	IP *IP `json:"ip,omitempty"`

	// IPs is a list of IP addresses (both IPv4 and IPv6) assigned to the network interface.
	// +optional
	IPs []IP `json:"ips,omitempty"`

	// MACAddress is the MAC address of the network interface.
	// +required
	MACAddress string `json:"macAddress"`

	// CarrierStatus is the operational carrier status of the network interface.
	// +optional
	CarrierStatus string `json:"carrierStatus,omitempty"`

	// Neighbors contains the LLDP neighbors discovered on this interface.
	// +optional
	Neighbors []LLDPNeighbor `json:"neighbors,omitempty"`
}

// LLDPNeighbor defines the details of an LLDP neighbor.
type LLDPNeighbor struct {
	// MACAddress is the MAC address of the LLDP neighbor.
	// +optional
	MACAddress string `json:"macAddress,omitempty"`

	// PortID is the port identifier of the LLDP neighbor.
	// +optional
	PortID string `json:"portID,omitempty"`

	// PortDescription is the port description of the LLDP neighbor.
	// +optional
	PortDescription string `json:"portDescription,omitempty"`

	// SystemName is the system name of the LLDP neighbor.
	// +optional
	SystemName string `json:"systemName,omitempty"`

	// SystemDescription is the system description of the LLDP neighbor.
	// +optional
	SystemDescription string `json:"systemDescription,omitempty"`
}

// StorageDrive defines the details of one storage drive.
type StorageDrive struct {
	// Name is the name of the storage drive.
	// +optional
	Name string `json:"name,omitempty"`

	// MediaType specifies the media type of the storage device.
	// +optional
	MediaType string `json:"mediaType,omitempty"`

	// Type specifies the type of the storage device.
	// +optional
	Type string `json:"type,omitempty"`

	// Capacity specifies the size of the storage device in bytes.
	// +optional
	Capacity *resource.Quantity `json:"capacity,omitempty"`

	// Vendor specifies the vendor of the storage device.
	// +optional
	Vendor string `json:"vendor,omitempty"`

	// Model specifies the model of the storage device.
	// +optional
	Model string `json:"model,omitempty"`

	// State specifies the state of the storage device.
	// +optional
	State StorageState `json:"state,omitempty"`
}

// StorageVolume defines the details of one storage volume.
type StorageVolume struct {
	// Name is the name of the storage volume.
	// +optional
	Name string `json:"name,omitempty"`

	// Capacity specifies the size of the storage device in bytes.
	// +optional
	Capacity *resource.Quantity `json:"capacity,omitempty"`

	// Status specifies the status of the volume.
	// +optional
	State StorageState `json:"state,omitempty"`

	// RAIDType specifies the RAID type of the associated Volume.
	// +optional
	RAIDType string `json:"raidType,omitempty"`

	// VolumeUsage specifies the volume usage type for the Volume.
	// +optional
	VolumeUsage string `json:"volumeUsage,omitempty"`
}

// Storage defines the details of one storage device.
type Storage struct {
	// Name is the name of the storage device.
	// +optional
	Name string `json:"name,omitempty"`

	// State specifies the state of the storage device.
	// +optional
	State StorageState `json:"state,omitempty"`

	// Volumes is a collection of volumes associated with this storage.
	// +optional
	Volumes []StorageVolume `json:"volumes,omitempty"`

	// Drives is a collection of drives associated with this storage.
	// +optional
	Drives []StorageDrive `json:"drives,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=srv
// +kubebuilder:printcolumn:name="SystemUUID",type=string,JSONPath=`.spec.systemUUID`
// +kubebuilder:printcolumn:name="Manufacturer",type=string,JSONPath=`.status.manufacturer`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.status.model`
// +kubebuilder:printcolumn:name="Memory",type=string,JSONPath=`.status.totalSystemMemory`
// +kubebuilder:printcolumn:name="SKU",type=string,JSONPath=`.status.sku`,priority=100
// +kubebuilder:printcolumn:name="SerialNumber",type=string,JSONPath=`.status.serialNumber`,priority=100
// +kubebuilder:printcolumn:name="PowerState",type=string,JSONPath=`.status.powerState`
// +kubebuilder:printcolumn:name="IndicatorLED",type=string,JSONPath=`.status.indicatorLED`,priority=100
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Server is the Schema for the servers API
type Server struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerSpec   `json:"spec,omitempty"`
	Status ServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerList contains a list of Server
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Server `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}
