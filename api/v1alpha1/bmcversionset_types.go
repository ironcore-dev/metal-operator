// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCVersionSetSpec defines the desired state of BMCVersionSet.
type BMCVersionSetSpec struct {
	// BMCSelector specifies a label selector to identify the BMC that are to be selected.
	// +required
	BMCSelector metav1.LabelSelector `json:"bmcSelector"`

	// BMCVersionTemplate defines the template for the BMCversion Resource to be applied to the servers.
	BMCVersionTemplate BMCVersionTemplate `json:"bmcVersionTemplate,omitempty"`
}

// BMCVersionSetStatus defines the observed state of BMCVersionSet.
type BMCVersionSetStatus struct {
	// FullyLabeledBMCS is the number of server in the set.
	FullyLabeledBMCS int32 `json:"fullyLabeledBMCS,omitempty"`
	// AvailableBMCVersion is the number of BMCVersion current created by the set.
	AvailableBMCVersion int32 `json:"availableBMCVersion,omitempty"`
	// PendingBMCVersion is the total number of pending BMCVersion in the set.
	PendingBMCVersion int32 `json:"pendingBMCVersion,omitempty"`
	// InProgressBMCVersion is the total number of BMCVersion in the set that are currently in InProgress.
	InProgressBMCVersion int32 `json:"inProgressBMCVersion,omitempty"`
	// CompletedBMCVersion is the total number of completed BMCVersion in the set.
	CompletedBMCVersion int32 `json:"completedBMCVersion,omitempty"`
	// FailedBMCVersion is the total number of failed BMCVersion in the set.
	FailedBMCVersion int32 `json:"failedBMCVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BMCVersion",type=string,JSONPath=`.spec.bmcVersionTemplate.version`
// +kubebuilder:printcolumn:name="selectedServers",type="integer",JSONPath=`.status.fullyLabeledBMCS`
// +kubebuilder:printcolumn:name="AvailableBMCVersion",type="integer",JSONPath=`.status.availableBMCVersion`
// +kubebuilder:printcolumn:name="Pending",type="integer",JSONPath=`.status.pendingBMCVersion`
// +kubebuilder:printcolumn:name="InProgress",type="integer",JSONPath=`.status.inProgressBMCVersion`
// +kubebuilder:printcolumn:name="Completed",type="integer",JSONPath=`.status.completedBMCVersion`
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=`.status.failedBMCVersion`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BMCVersionSet is the Schema for the bmcversionsets API.
type BMCVersionSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCVersionSetSpec   `json:"spec,omitempty"`
	Status BMCVersionSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BMCVersionSetList contains a list of BMCVersionSet.
type BMCVersionSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCVersionSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCVersionSet{}, &BMCVersionSetList{})
}
