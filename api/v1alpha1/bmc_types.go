// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	BMCType              = "bmc"
	ProtocolRedfish      = "Redfish"
	ProtocolRedfishLocal = "RedfishLocal"
)

// BMCSpec defines the desired state of BMC
type BMCSpec struct {
	// EndpointRef is a reference to the Kubernetes object that contains the endpoint information for the BMC.
	// This reference is typically used to locate the BMC endpoint within the cluster.
	EndpointRef v1.LocalObjectReference `json:"endpointRef"`

	// BMCSecretRef is a reference to the Kubernetes Secret object that contains the credentials
	// required to access the BMC. This secret includes sensitive information such as usernames and passwords.
	BMCSecretRef v1.LocalObjectReference `json:"bmcSecretRef"`

	// Protocol specifies the protocol to be used for communicating with the BMC.
	// It could be a standard protocol such as IPMI or Redfish.
	Protocol Protocol `json:"protocol"`

	// ConsoleProtocol specifies the protocol to be used for console access to the BMC.
	// This field is optional and can be omitted if console access is not required.
	// +optional
	ConsoleProtocol *ConsoleProtocol `json:"consoleProtocol,omitempty"`
}

// ConsoleProtocol defines the protocol and port used for console access to the BMC.
type ConsoleProtocol struct {
	// Name specifies the name of the console protocol.
	// This could be a protocol such as "SSH", "Telnet", etc.
	Name ConsoleProtocolName `json:"name"`

	// Port specifies the port number used for console access.
	// This port is used by the specified console protocol to establish connections.
	Port int32 `json:"port"`
}

// ConsoleProtocolName defines the possible names for console protocols.
type ConsoleProtocolName string

const (
	// ConsoleProtocolNameIPMI represents the IPMI console protocol.
	ConsoleProtocolNameIPMI ConsoleProtocolName = "IPMI"

	// ConsoleProtocolNameSSH represents the SSH console protocol.
	ConsoleProtocolNameSSH ConsoleProtocolName = "SSH"

	// ConsoleProtocolNameSSHLenovo represents the SSH console protocol specific to Lenovo hardware.
	ConsoleProtocolNameSSHLenovo ConsoleProtocolName = "SSHLenovo"
)

// Protocol defines the protocol and port used for communicating with the BMC.
type Protocol struct {
	// Name specifies the name of the protocol.
	// This could be a protocol such as "IPMI", "Redfish", etc.
	Name ProtocolName `json:"name"`

	// Port specifies the port number used for communication.
	// This port is used by the specified protocol to establish connections.
	Port int32 `json:"port"`
}

// ProtocolName defines the possible names for protocols used for communicating with the BMC.
type ProtocolName string

const (
	// ProtocolNameRedfish represents the Redfish protocol.
	ProtocolNameRedfish ProtocolName = "Redfish"

	// ProtocolNameIPMI represents the IPMI protocol.
	ProtocolNameIPMI ProtocolName = "IPMI"

	// ProtocolNameSSH represents the SSH protocol.
	ProtocolNameSSH ProtocolName = "SSH"
)

// BMCPowerState defines the possible power states for a BMC.
type BMCPowerState string

const (
	// OnPowerState the system is powered on.
	OnPowerState BMCPowerState = "On"
	// OffPowerState the system is powered off, although some components may
	// continue to have AUX power such as management controller.
	OffPowerState BMCPowerState = "Off"
	// PausedPowerState the system is paused.
	PausedPowerState BMCPowerState = "Paused"
	// PoweringOnPowerState A temporary state between Off and On. This
	// temporary state can be very short.
	PoweringOnPowerState BMCPowerState = "PoweringOn"
	// PoweringOffPowerState A temporary state between On and Off. The power
	// off action can take time while the OS is in the shutdown process.
	PoweringOffPowerState BMCPowerState = "PoweringOff"
)

// BMCStatus defines the observed state of BMC.
type BMCStatus struct {
	// MACAddress is the MAC address of the BMC.
	// The format is validated using a regular expression pattern.
	// +kubebuilder:validation:Pattern=`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`
	MACAddress string `json:"macAddress,omitempty"`

	// IP is the IP address of the BMC.
	// The type is specified as string and is schemaless.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Schemaless
	IP IP `json:"ip,omitempty"`

	// Manufacturer is the name of the BMC manufacturer.
	Manufacturer string `json:"manufacturer,omitempty"`

	// Model is the model number or name of the BMC.
	Model string `json:"model,omitempty"`

	// SKU is the stock keeping unit identifier for the BMC.
	SKU string `json:"sku,omitempty"`

	// SerialNumber is the serial number of the BMC.
	SerialNumber string `json:"serialNumber,omitempty"`

	// FirmwareVersion is the version of the firmware currently running on the BMC.
	FirmwareVersion string `json:"firmwareVersion,omitempty"`

	// State represents the current state of the BMC.
	State BMCState `json:"state,omitempty"`

	// PowerState represents the current power state of the BMC.
	PowerState BMCPowerState `json:"powerState,omitempty"`

	// Conditions represents the latest available observations of the BMC's current state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// BMCState defines the possible states of a BMC.
type BMCState string

const (
	// BMCStateEnabled indicates that the BMC is enabled and functioning correctly.
	BMCStateEnabled BMCState = "Enabled"

	// BMCStateError indicates that there is an error with the BMC.
	BMCStateError BMCState = "Error"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="MACAddress",type=string,JSONPath=`.status.macAddress`
// +kubebuilder:printcolumn:name="MACAddress",type=string,JSONPath=`.status.ip`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.status.model`
// +kubebuilder:printcolumn:name="SKU",type=string,JSONPath=`.status.sku`,priority=100
// +kubebuilder:printcolumn:name="SerialNumber",type=string,JSONPath=`.status.serialNumber`,priority=100
// +kubebuilder:printcolumn:name="FirmwareVersion",type=string,JSONPath=`.status.firmwareVersion`,priority=100
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="PowerState",type=string,JSONPath=`.status.powerState`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BMC is the Schema for the bmcs API
type BMC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCSpec   `json:"spec,omitempty"`
	Status BMCStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BMCList contains a list of BMC
type BMCList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMC `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMC{}, &BMCList{})
}
