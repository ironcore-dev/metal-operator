// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServerBIOSSpec defines the desired state of ServerBIOS.
type ServerBIOSSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// BIOS specifies the BIOS settings for the selected serverRef or serverSelector.
	BIOS BIOSSettings `json:"bios,omitempty"`

	// ServerRef is a reference to a specific server to be claimed.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"maintenancePolicy,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that that BIOS has requested for the referred server.
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

type BIOSSettings struct {
	// Version contains BIOS version
	// +required
	Version string `json:"version"`

	// Settings contains BIOS settings as map
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// ServerMaintenanceState specifies the current state of the server maintenance.
type BIOSMaintenanceState string

const (
	// BIOSMaintenanceStatePending specifies that the server bios maintenance is pending.
	BIOSMaintenanceStatePending BIOSMaintenanceState = "Pending"
	// BIOSMaintenanceStateInVersionUpgrade specifies that the server bios is in version upgrade path.
	BIOSMaintenanceStateInVersionUpgrade BIOSMaintenanceState = "InVersionUpgrade"
	// BIOSMaintenanceStateInSettingUpdate specifies that the server bios is in setting update path.
	BIOSMaintenanceStateInSettingUpdate BIOSMaintenanceState = "InSettingUpdate"
	// BIOSMaintenanceStateCompleted specifies that the server bios maintenance has been completed.
	BIOSMaintenanceStateCompleted BIOSMaintenanceState = "Completed"
	// BIOSMaintenanceStateFailed specifies that the server maintenance has failed.
	BIOSMaintenanceStateFailed BIOSMaintenanceState = "Failed"
)

// ServerBIOSStatus defines the observed state of ServerBIOS.
type ServerBIOSStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// State represents the current state of the bios configuration task.
	State BIOSMaintenanceState `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// ServerBIOS is the Schema for the serverbios API.
type ServerBIOS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerBIOSSpec   `json:"spec,omitempty"`
	Status ServerBIOSStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerBIOSList contains a list of ServerBIOS.
type ServerBIOSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerBIOS `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerBIOS{}, &ServerBIOSList{})
}
