// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BIOSSettingsSetSpec defines the desired state of BIOSSettingsSet.
type BIOSSettingsSetSpec struct {
	// BIOSSettingsTemplate defines the template for the BIOSSettings resource to be applied to the servers.
	BIOSSettingsTemplate BIOSSettingsTemplate `json:"biosSettingsTemplate,omitempty"`

	// ServerSelector specifies a label selector to identify the servers that are to be selected.
	// +required
	ServerSelector metav1.LabelSelector `json:"serverSelector"`
}

// BIOSSettingsSetStatus defines the observed state of BIOSSettingsSet.
type BIOSSettingsSetStatus struct {
	// FullyLabeledServers is the number of servers in the set.
	FullyLabeledServers int32 `json:"fullyLabeledServers,omitempty"`
	// AvailableBIOSSettings is the number of BIOSSettings currently created by the set.
	AvailableBIOSSettings int32 `json:"availableBIOSSettings,omitempty"`
	// PendingBIOSSettings is the total number of pending BIOSSettings in the set.
	PendingBIOSSettings int32 `json:"pendingBIOSSettings,omitempty"`
	// InProgressBIOSSettings is the total number of BIOSSettings in the set that are currently in progress.
	InProgressBIOSSettings int32 `json:"inProgressBIOSSettings,omitempty"`
	// CompletedBIOSSettings is the total number of completed BIOSSettings in the set.
	CompletedBIOSSettings int32 `json:"completedBIOSSettings,omitempty"`
	// FailedBIOSSettings is the total number of failed BIOSSettings in the set.
	FailedBIOSSettings int32 `json:"failedBIOSSettings,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bss
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.biosSettingsTemplate.version`
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
