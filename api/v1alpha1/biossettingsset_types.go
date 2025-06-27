// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BIOSSettingsSetSpec defines the desired state of BIOSSettingsSet.
type BIOSSettingsSetSpec struct {
	// Version contains software (eg: BIOS, BMC) version this settings applies to
	// +required
	Version string `json:"version"`

	// ServerSelector specifies a label selector to identify the servers that are to be selected.
	// +required
	ServerSelector metav1.LabelSelector `json:"serverSelector"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// SettingsFlow contains BIOS settings sequence to apply on the BIOS in given order
	// if the settingsFlow length is 1, BIOSSettings resource is created.
	// if the length is more than 1, BIOSSettingsFlow resource is created.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverSelector is immutable"
	SettingsFlow []SettingsFlowItem `json:"settingsFlow,omitempty"`
}

type SettingsFlowItem struct {
	Settings map[string]string `json:"settings,omitempty"`
	// Priority defines the order of applying the settings
	// any int greater than 0. lower number have higher Priority (ie; lower number is applied first)
	Priority int32 `json:"priority"`
}

// BIOSSettingsSetStatus defines the observed state of BIOSSettingsSet.
type BIOSSettingsSetStatus struct {
	// TotalServers is the number of server in the set.
	TotalServers int32 `json:"totalServers,omitempty"`
	// TotalSettings is the number of Settings current created by the set.
	TotalSettings int32 `json:"totalSettings,omitempty"`
	// Pending is the total number of pending server in the set.
	Pending int32 `json:"pending,omitempty"`
	// InMaintenance is the total number of server in the set that are currently in InProgress.
	InProgress int32 `json:"inProgress,omitempty"`
	// Completed is the total number of completed server in the set.
	Completed int32 `json:"completed,omitempty"`
	// Failed is the total number of failed server in the set.
	Failed int32 `json:"failed,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="serverLabelSelector",type=string,JSONPath=`.spec.serverSelector`
// +kubebuilder:printcolumn:name="TotalServers",type="string",JSONPath=`.status.totalServers`
// +kubebuilder:printcolumn:name="Pending",type="string",JSONPath=`.status.pending`
// +kubebuilder:printcolumn:name="InProgress",type="string",JSONPath=`.status.inProgress`
// +kubebuilder:printcolumn:name="Completed",type="string",JSONPath=`.status.completed`
// +kubebuilder:printcolumn:name="Failed",type="string",JSONPath=`.status.failed`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BIOSSettingsSet is the Schema for the biossettingssets API.
type BIOSSettingsSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BIOSSettingsSetSpec   `json:"spec,omitempty"`
	Status BIOSSettingsSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BIOSSettingsSetList contains a list of BIOSSettingsSet.
type BIOSSettingsSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BIOSSettingsSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BIOSSettingsSet{}, &BIOSSettingsSetList{})
}
