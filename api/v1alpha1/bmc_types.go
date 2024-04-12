// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCSpec defines the desired state of BMC
type BMCSpec struct {
	EndpointRef  v1.LocalObjectReference `json:"endpointRef"`
	BMCSecretRef v1.LocalObjectReference `json:"bmcSecretRef"`
	Protocol     Protocol                `json:"protocol"`
	//+optional
	ConsoleProtocol *ConsoleProtocol `json:"consoleProtocol,omitempty"`
}

type ConsoleProtocol struct {
	Name ConsoleProtocolName `json:"name"`
	Port int32               `json:"port"`
}

type ConsoleProtocolName string

const (
	ConsoleProtocolNameIPMI      ConsoleProtocolName = "IPMI"
	ConsoleProtocolNameSSH       ConsoleProtocolName = "SSH"
	ConsoleProtocolNameSSHLenovo ConsoleProtocolName = "SSHLenovo"
)

type Protocol struct {
	Name ProtocolName `json:"name"`
	Port int32        `json:"port"`
}

type ProtocolName string

const (
	ProtocolNameRedfish ProtocolName = "Redfish"
	ProtocolNameIPMI    ProtocolName = "IPMI"
	ProtocolNameSSH     ProtocolName = "SSH"
)

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

// BMCStatus defines the observed state of BMC
type BMCStatus struct {
	//+kubebuilder:validation:Pattern=`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`
	MACAddress string `json:"macAddress,omitempty"`
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Schemaless
	IP              IP            `json:"ip,omitempty"`
	Manufacturer    string        `json:"manufacturer,omitempty"`
	Model           string        `json:"model,omitempty"`
	SKU             string        `json:"sku,omitempty"`
	SerialNumber    string        `json:"serialNumber,omitempty"`
	FirmwareVersion string        `json:"firmwareVersion,omitempty"`
	State           BMCState      `json:"state,omitempty"`
	PowerState      BMCPowerState `json:"powerState,omitempty"`
	//+patchStrategy=merge
	//+patchMergeKey=type
	//+optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

type BMCState string

const (
	BMCStateEnabled BMCState = "Enabled"
	BMCStateError   BMCState = "Error"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="MACAddress",type=string,JSONPath=`.status.macAddress`
// +kubebuilder:printcolumn:name="MACAddress",type=string,JSONPath=`.status.ip`
// +kubebuilder:printcolumn:name="Manufacturer",type=string,JSONPath=`.status.manufacturer`
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
