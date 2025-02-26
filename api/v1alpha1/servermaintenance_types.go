// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ServerMaintenanceNeededLabelKey is a label key that is used to indicate that a server requires maintenance.
	ServerMaintenanceNeededLabelKey = "metal.ironcore.dev/maintenance-needed"
	// ServerMaintenanceReasonAnnotationKey is an annotation key that is used to store the reason for a server maintenance.
	ServerMaintenanceReasonAnnotationKey = "metal.ironcore.dev/reason"
)

// ServerBootConfigurationTemplate defines the parameters to be used for rendering a boot configuration.
type ServerBootConfigurationTemplate struct {
	// Name specifies the name of the boot configuration.
	Name string `json:"name"`
	// Parameters specifies the parameters to be used for rendering the boot configuration.
	Spec ServerBootConfigurationSpec `json:"spec"`
}

// ServerMaintenanceSpec defines the desired state of a ServerMaintenance
type ServerMaintenanceSpec struct {
	// Policy specifies the maintenance policy to be enforced on the server.
	Policy ServerMaintenancePolicy `json:"policy,omitempty"`
	// ServerRef is a reference to the server that is to be maintained.
	ServerRef corev1.LocalObjectReference `json:"serverRef"`
	// ServerPower specifies the power state of the server during maintenance.
	ServerPower Power `json:"serverPower,omitempty"`
	// ServerBootConfigurationTemplate specifies the boot configuration to be applied to the server during maintenance.
	ServerBootConfigurationTemplate *ServerBootConfigurationTemplate `json:"serverBootConfigurationTemplate,omitempty"`
}

// ServerMaintenancePolicy specifies the maintenance policy to be enforced on the server.
type ServerMaintenancePolicy string

const (
	// ServerMaintenancePolicyOwnerApproval specifies that the maintenance policy requires owner approval.
	ServerMaintenancePolicyOwnerApproval ServerMaintenancePolicy = "OwnerApproval"
	// ServerMaintenancePolicyEnforced specifies that the maintenance policy is enforced.
	ServerMaintenancePolicyEnforced ServerMaintenancePolicy = "Enforced"
)

// ServerMaintenanceStatus defines the observed state of a ServerMaintenance
type ServerMaintenanceStatus struct {
	// State specifies the current state of the server maintenance.
	State ServerMaintenanceState `json:"state,omitempty"`
}

// ServerMaintenanceState specifies the current state of the server maintenance.
type ServerMaintenanceState string

const (
	// ServerMaintenanceStatePending specifies that the server maintenance is pending.
	ServerMaintenanceStatePending ServerMaintenanceState = "Pending"
	// ServerMaintenanceStateInMaintenance specifies that the server is in maintenance.
	ServerMaintenanceStateInMaintenance ServerMaintenanceState = "InMaintenance"
	// ServerMaintenanceStateCompleted specifies that the server maintenance has been completed.
	ServerMaintenanceStateCompleted ServerMaintenanceState = "Completed"
	// ServerMaintenanceStateFailed specifies that the server maintenance has failed.
	ServerMaintenanceStateFailed ServerMaintenanceState = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Server",type="string",JSONPath=".spec.serverRef.name"
// +kubebuilder:printcolumn:name="Policy",type="string",JSONPath=`.spec.policy`
// +kubebuilder:printcolumn:name="BootConfiguration",type="string",JSONPath=`.spec.serverBootConfigurationTemplate.name`
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=`.metadata.annotations.metal\.ironcore\.dev\/reason`
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServerMaintenance is the Schema for the ServerMaintenance API
type ServerMaintenance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerMaintenanceSpec   `json:"spec,omitempty"`
	Status ServerMaintenanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServerMaintenanceList contains a list of ServerMaintenances
type ServerMaintenanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerMaintenance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerMaintenance{}, &ServerMaintenanceList{})
}
