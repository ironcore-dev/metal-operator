// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Power defines the possible power states for a device.
type Power string

const (
	// PowerOn indicates that the device is powered on.
	PowerOn Power = "On"

	// PowerOff indicates that the device is powered off.
	PowerOff Power = "Off"
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
	Protocol Protocol `json:"protocol"`

	// Endpoint is the address of the BMC endpoint.
	Endpoint string `json:"endpoint"`

	// BMCSecretRef is a reference to the Kubernetes Secret object that contains the credentials
	// required to access the BMC. This secret includes sensitive information such as usernames and passwords.
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
	UUID string `json:"uuid"`

	// Power specifies the desired power state of the server.
	Power Power `json:"power,omitempty"`

	// IndicatorLED specifies the desired state of the server's indicator LED.
	IndicatorLED IndicatorLED `json:"indicatorLED,omitempty"`

	// ServerClaimRef is a reference to a ServerClaim object that claims this server.
	// This field is optional and can be omitted if no claim is associated with this server.
	ServerClaimRef *v1.ObjectReference `json:"serverClaimRef,omitempty"`

	// BMCRef is a reference to the BMC object associated with this server.
	// This field is optional and can be omitted if no BMC is associated with this server.
	BMCRef *v1.LocalObjectReference `json:"bmcRef,omitempty"`

	// BMC contains the access details for the BMC.
	// This field is optional and can be omitted if no BMC access is specified.
	BMC *BMCAccess `json:"bmc,omitempty"`

	// BootConfigurationRef is a reference to a BootConfiguration object that specifies
	// the boot configuration for this server. This field is optional and can be omitted
	// if no boot configuration is specified.
	BootConfigurationRef *v1.ObjectReference `json:"bootConfigurationRef,omitempty"`

	// BootOrder specifies the boot order of the server.
	BootOrder []BootOrder `json:"bootOrder,omitempty"`

	// BIOSSettingsRef is a reference to a ServerBIOS object.
	BIOSSettingsRef v1.LocalObjectReference `json:"biOSSettingsRef,omitempty"`
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

// ServerStatus defines the observed state of Server.
type ServerStatus struct {
	// Manufacturer is the name of the server manufacturer.
	Manufacturer string `json:"manufacturer,omitempty"`

	// Model is the model of the server.
	Model string `json:"model,omitempty"`

	// SKU is the stock keeping unit identifier for the server.
	SKU string `json:"sku,omitempty"`

	// SerialNumber is the serial number of the server.
	SerialNumber string `json:"serialNumber,omitempty"`

	// PowerState represents the current power state of the server.
	PowerState ServerPowerState `json:"powerState,omitempty"`

	// IndicatorLED specifies the current state of the server's indicator LED.
	IndicatorLED IndicatorLED `json:"indicatorLED,omitempty"`

	// State represents the current state of the server.
	State ServerState `json:"state,omitempty"`

	// NetworkInterfaces is a list of network interfaces associated with the server.
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`

	// Conditions represents the latest available observations of the server's current state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// NetworkInterface defines the details of a network interface.
type NetworkInterface struct {
	// Name is the name of the network interface.
	Name string `json:"name"`

	// IP is the IP address assigned to the network interface.
	// The type is specified as string and is schemaless.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Schemaless
	IP IP `json:"ip"`

	// MACAddress is the MAC address of the network interface.
	MACAddress string `json:"macAddress"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.spec.uuid`
// +kubebuilder:printcolumn:name="Manufacturer",type=string,JSONPath=`.status.manufacturer`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.status.model`
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
