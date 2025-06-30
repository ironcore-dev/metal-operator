// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BIOSVersionSetSpec defines the desired state of BIOSVersionSet.
type BIOSVersionSetSpec struct {
	// Version contains software (eg: BIOS, BMC) version this settings applies to
	// +required
	Version string `json:"version"`

	// An indication of whether the server's upgrade service should bypass vendor update policies
	UpdatePolicy *UpdatePolicy `json:"updatePolicy,omitempty"`
	// details regarding the image to use to upgrade to given BIOS version
	// +required
	Image ImageSpec `json:"image"`

	// ServerSelector specifies a label selector to identify the servers that are to be selected.
	// +required
	ServerSelector metav1.LabelSelector `json:"serverSelector"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`
}

// BIOSVersionSetStatus defines the observed state of BIOSVersionSet.
type BIOSVersionSetStatus struct {
	// TotalServers is the number of server in the set.
	TotalServers int32 `json:"totalServers,omitempty"`
	// TotalVersionResource is the number of Settings current created by the set.
	TotalVersionResource int32 `json:"totalVersionResource,omitempty"`
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
