// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// BMCType is the type of the BMC resource.
	BMCType = "bmc"

	// ProtocolRedfish is the Redfish protocol.
	ProtocolRedfish = "Redfish"
	// ProtocolRedfishLocal is the RedfishLocal protocol.
	ProtocolRedfishLocal = "RedfishLocal"
	// ProtocolRedfishKube is the RedfishKube protocol.
	ProtocolRedfishKube = "RedfishKube"
)

type PasswordPolicy string

const (
	// PasswordPolicyExternal indicates that the password policy is managed externally, such as by an external identity provider.
	PasswordPolicyExternal PasswordPolicy = "External"
	// PasswordPolicyInternal indicates that the password policy is managed internally, such as by the BMC itself.
	PasswordPolicyInternal PasswordPolicy = "Internal"
)

// BMCSpec defines the desired state of BMC
// +kubebuilder:validation:XValidation:rule="has(self.access) != has(self.endpointRef)",message="exactly one of access or endpointRef needs to be set"
type BMCSpec struct {
	// BMCUUID is the unique identifier for the BMC as defined in Redfish API.
	// +kubebuilder:validation:Optional
	// +optional
	BMCUUID string `json:"bmcUUID,omitempty"`

	// EndpointRef is a reference to the Kubernetes object that contains the endpoint information for the BMC.
	// This reference is typically used to locate the BMC endpoint within the cluster.
	// +optional
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="endpointRef is immutable"
	EndpointRef *v1.LocalObjectReference `json:"endpointRef"`

	// Endpoint allows inline configuration of network access details for the BMC.
	// Use this field if access settings like address are to be configured directly within the BMC resource.
	// +optional
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="access is immutable"
	Endpoint *InlineEndpoint `json:"access,omitempty"`

	// BMCSecretRef is a reference to the Kubernetes Secret object that contains the credentials
	// required to access the BMC. This secret includes sensitive information such as usernames and passwords.
	// +required
	BMCSecretRef v1.LocalObjectReference `json:"bmcSecretRef"`

	// Protocol specifies the protocol to be used for communicating with the BMC.
	// It could be a standard protocol such as IPMI or Redfish.
	// +required
	Protocol Protocol `json:"protocol"`

	// ConsoleProtocol specifies the protocol to be used for console access to the BMC.
	// This field is optional and can be omitted if console access is not required.
	// +optional
	ConsoleProtocol *ConsoleProtocol `json:"consoleProtocol,omitempty"`

	// UserAccounts is a list of user accounts that can be used to access the BMC.
	// Each account includes a name, role ID, description, and other relevant details.
	// +optional
	UserRefs []UserSpec `json:"userRefs,omitempty"`

	// BMCSettingRef is a reference to a BMCSettings object that specifies
	// the BMC configuration for this BMC.
	// +optional
	BMCSettingRef *v1.LocalObjectReference `json:"bmcSettingsRef,omitempty"`
}

// InlineEndpoint defines inline network access configuration for the BMC.
type InlineEndpoint struct {
	// MACAddress is the MAC address of the endpoint.
	// +optional
	MACAddress string `json:"macAddress,omitempty"`

	// IP is the IP address of the BMC.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Schemaless
	// +optional
	IP IP `json:"ip"`
}

// ConsoleProtocol defines the protocol and port used for console access to the BMC.
type ConsoleProtocol struct {
	// Name specifies the name of the console protocol.
	// This could be a protocol such as "SSH", "Telnet", etc.
	// +kubebuilder:validation:Enum=IPMI;SSH;SSHLenovo
	// +required
	Name ConsoleProtocolName `json:"name"`

	// Port specifies the port number used for console access.
	// This port is used by the specified console protocol to establish connections.
	// +required
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

// ProtocolScheme is a string that contains the protocol scheme
type ProtocolScheme string

const (
	// HTTPProtocolScheme is the http protocol scheme
	HTTPProtocolScheme ProtocolScheme = "http"
	// HTTPSProtocolScheme is the https protocol scheme
	HTTPSProtocolScheme ProtocolScheme = "https"
)

// Protocol defines the protocol and port used for communicating with the BMC.
type Protocol struct {
	// Name specifies the name of the protocol.
	// This could be a protocol such as "IPMI", "Redfish", etc.
	Name ProtocolName `json:"name"`

	// Port specifies the port number used for communication.
	// This port is used by the specified protocol to establish connections.
	Port int32 `json:"port"`

	// Scheme specifies the scheme used for communication.
	Scheme ProtocolScheme `json:"scheme,omitempty"`
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
	// +optional
	MACAddress string `json:"macAddress,omitempty"`

	// IP is the IP address of the BMC.
	// The type is specified as string and is schemaless.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Schemaless
	// +optional
	IP IP `json:"ip,omitempty"`

	// Manufacturer is the name of the BMC manufacturer.
	// +optional
	Manufacturer string `json:"manufacturer,omitempty"`

	// Model is the model number or name of the BMC.
	// +optional
	Model string `json:"model,omitempty"`

	// SKU is the stock keeping unit identifier for the BMC.
	// +optional
	SKU string `json:"sku,omitempty"`

	// SerialNumber is the serial number of the BMC.
	// +optional
	SerialNumber string `json:"serialNumber,omitempty"`

	// FirmwareVersion is the version of the firmware currently running on the BMC.
	// +optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`

	// State represents the current state of the BMC.
	// +optional
	State BMCState `json:"state,omitempty"`

	// PowerState represents the current power state of the BMC.
	// +optional
	PowerState BMCPowerState `json:"powerState,omitempty"`

	// LastResetTime is the timestamp of the last reset operation performed on the BMC.
	// +optional
	LastResetTime *metav1.Time `json:"lastResetTime,omitempty"`

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
// +kubebuilder:printcolumn:name="IP",type=string,JSONPath=`.status.ip`
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
