// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BIOSVersionSetSpec defines the desired state of BIOSVersionSet.
type BIOSVersionSetSpec struct {
	// ServerSelector specifies a label selector to identify the servers that are to be selected.
	// +required
	ServerSelector metav1.LabelSelector `json:"serverSelector"`

	// BiosVersionTemplate defines the template for the BIOSversion Resource to be applied to the servers.
	BiosVersionTemplate BIOSVersionTemplate `json:"biosVersionTemplate,omitempty"`
}

// BIOSVersionSetStatus defines the observed state of BIOSVersionSet.
type BIOSVersionSetStatus struct {
	// FullyLabeledServers is the number of servers in the set.
	FullyLabeledServers int32 `json:"fullyLabeledServers,omitempty"`
	// AvailableBIOSVersion is the number of BIOSVersion created by the set.
	AvailableBIOSVersion int32 `json:"availableBIOSVersion,omitempty"`
	// PendingBIOSVersion is the total number of pending BIOSVersion in the set.
	PendingBIOSVersion int32 `json:"pendingBIOSVersion,omitempty"`
	// InProgressBIOSVersion is the total number of BIOSVersion in the set that are currently in InProgress.
	InProgressBIOSVersion int32 `json:"inProgressBIOSVersion,omitempty"`
	// CompletedBIOSVersion is the total number of completed BIOSVersion in the set.
	CompletedBIOSVersion int32 `json:"completedBIOSVersion,omitempty"`
	// FailedBIOSVersion is the total number of failed BIOSVersion in the set.
	FailedBIOSVersion int32 `json:"failedBIOSVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.biosVersionTemplate.version`
// +kubebuilder:printcolumn:name="selectedServers",type="integer",JSONPath=`.status.fullyLabeledServers`
// +kubebuilder:printcolumn:name="AvailableBIOSVersion",type="integer",JSONPath=`.status.availableBIOSVersion`
// +kubebuilder:printcolumn:name="Pending",type="integer",JSONPath=`.status.pendingBIOSVersion`
// +kubebuilder:printcolumn:name="InProgress",type="integer",JSONPath=`.status.inProgressBIOSVersion`
// +kubebuilder:printcolumn:name="Completed",type="integer",JSONPath=`.status.completedBIOSVersion`
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=`.status.failedBIOSVersion`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BIOSVersionSet is the Schema for the biosversionsets API.
type BIOSVersionSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BIOSVersionSetSpec   `json:"spec,omitempty"`
	Status BIOSVersionSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BIOSVersionSetList contains a list of BIOSVersionSet.
type BIOSVersionSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BIOSVersionSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BIOSVersionSet{}, &BIOSVersionSetList{})
}
