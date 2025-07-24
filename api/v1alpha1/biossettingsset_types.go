// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BIOSSettingsSetSpec defines the desired state of BIOSSettingsSet.
type BIOSSettingsSetSpec struct {
	// todo: merge this into common structure when we #351 merged
	// Version contains software (eg: BIOS, BMC) version this settings applies to
	// +required
	Version string `json:"version"`
	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// SettingsFlow contains BIOS settings sequence to apply on the BIOS in given order
	// if the settingsFlow length is 1, BIOSSettings resource is created.
	// if the length is more than 1, BIOSSettingsFlow resource is created.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="SettingsFlow is immutable"
	SettingsFlow []SettingsFlowItem `json:"settingsFlow,omitempty"`

	// ServerSelector specifies a label selector to identify the servers that are to be selected.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ServerSelector is immutable"
	// +required
	ServerSelector metav1.LabelSelector `json:"serverSelector"`
}

// todo: remove this when we #351 merge
type SettingsFlowItem struct {
	Settings map[string]string `json:"settings,omitempty"`
	// Priority defines the order of applying the settings
	// any int greater than 0. lower number have higher Priority (ie; lower number is applied first)
	Priority int32 `json:"priority"`
}

// BIOSSettingsSetStatus defines the observed state of BIOSSettingsSet.
type BIOSSettingsSetStatus struct {
	// FullyLabeledServers is the number of server in the set.
	FullyLabeledServers int32 `json:"fullyLabeledServers,omitempty"`
	// AvailableBIOSVersion is the number of Settings current created by the set.
	AvailableBIOSSettings int32 `json:"availableBIOSSettings,omitempty"`
	// PendingBIOSSettings is the total number of pending server in the set.
	PendingBIOSSettings int32 `json:"pendingBIOSSettings,omitempty"`
	// InProgressBIOSSettings is the total number of server in the set that are currently in InProgress.
	InProgressBIOSSettings int32 `json:"inProgressBIOSSettings,omitempty"`
	// CompletedBIOSSettings is the total number of completed server in the set.
	CompletedBIOSSettings int32 `json:"completedBIOSSettings,omitempty"`
	// FailedBIOSSettings is the total number of failed server in the set.
	FailedBIOSSettings int32 `json:"failedBIOSSettings,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="TotalServers",type="string",JSONPath=`.status.fullyLabeledServers`
// +kubebuilder:printcolumn:name="AvailableBIOSSettings",type="string",JSONPath=`.status.availableBIOSSettings`
// +kubebuilder:printcolumn:name="Pending",type="string",JSONPath=`.status.pendingBIOSSettings`
// +kubebuilder:printcolumn:name="InProgress",type="string",JSONPath=`.status.inProgressBIOSSettings`
// +kubebuilder:printcolumn:name="Completed",type="string",JSONPath=`.status.completedBIOSSettings`
// +kubebuilder:printcolumn:name="Failed",type="string",JSONPath=`.status.failedBIOSSettings`
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
