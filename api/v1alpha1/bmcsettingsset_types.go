// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCSettingsSetSpec defines the desired state of BMCSettingsSet.
type BMCSettingsSetSpec struct {
	// BMCSettingsTemplate defines the template for the BMCSettings Resource to be applied to the BMCs.
	// +required
	BMCSettingsTemplate BMCSettingsTemplate `json:"bmcSettingsTemplate,omitempty"`

	//  BMCSelector specifies a label selector to identify the BMCs that are to be selected.
	// +required
	BMCSelector metav1.LabelSelector `json:"bmcSelector"`
}

// BMCSettingsSetStatus defines the observed state of BMCSettingsSet.
type BMCSettingsSetStatus struct {
	// FullyLabeledBMCs is the number of BMC in the set.
	FullyLabeledBMCs int32 `json:"fullyLabeledBMCs,omitempty"`
	// AvailableBMCSettings is the number of BMCSettings currently created by the set.
	AvailableBMCSettings int32 `json:"availableBMCSettings,omitempty"`
	// PendingBMCSettings is the total number of pending BMC in the set.
	PendingBMCSettings int32 `json:"pendingBMCSettings,omitempty"`
	// InProgressBMCSettings is the total number of BMC in the set that are currently in progress.
	InProgressBMCSettings int32 `json:"inProgressBMCSettings,omitempty"`
	// CompletedBMCSettings is the total number of completed BMC in the set.
	CompletedBMCSettings int32 `json:"completedBMCSettings,omitempty"`
	// FailedBMCSettings is the total number of failed BMC in the set.
	FailedBMCSettings int32 `json:"failedBMCSettings,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BMCVersion",type=string,JSONPath=`.spec.bmcSettingsTemplate.version`
// +kubebuilder:printcolumn:name="TotalBMCs",type="integer",JSONPath=`.status.fullyLabeledBMCs`
// +kubebuilder:printcolumn:name="AvailableBMCSettings",type="integer",JSONPath=`.status.availableBMCSettings`
// +kubebuilder:printcolumn:name="Pending",type="integer",JSONPath=`.status.pendingBMCSettings`
// +kubebuilder:printcolumn:name="InProgress",type="integer",JSONPath=`.status.inProgressBMCSettings`
// +kubebuilder:printcolumn:name="Completed",type="integer",JSONPath=`.status.completedBMCSettings`
// +kubebuilder:printcolumn:name="Failed",type="integer",JSONPath=`.status.failedBMCSettings`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// BMCSettingsSet is the Schema for the bmcsettingssets API.
type BMCSettingsSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCSettingsSetSpec   `json:"spec,omitempty"`
	Status BMCSettingsSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BMCSettingsSetList contains a list of BMCSettingsSet.
type BMCSettingsSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCSettingsSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCSettingsSet{}, &BMCSettingsSetList{})
}
