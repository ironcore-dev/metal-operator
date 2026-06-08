// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NICVersionSetSpec defines the desired state of NICVersionSet.
type NICVersionSetSpec struct {
    // ServerSelector specifies a label selector to identify the servers to update.
    // +required
    ServerSelector metav1.LabelSelector `json:"serverSelector"`

    // NICVersionTemplate defines the NIC firmware upgrade to apply to selected servers.
    NICVersionTemplate NICVersionTemplate `json:"nicVersionTemplate,omitempty"`
}

// NICVersionSetStatus defines the observed state of NICVersionSet.
type NICVersionSetStatus struct {
    // FullyLabeledServers is the number of servers matching the selector.
    FullyLabeledServers int32 `json:"fullyLabeledServers,omitempty"`
    // AvailableNICVersion is the number of NICVersion resources created by this set.
    AvailableNICVersion int32 `json:"availableNICVersion,omitempty"`
    // PendingNICVersion is the number of NICVersion resources in Pending state.
    PendingNICVersion int32 `json:"pendingNICVersion,omitempty"`
    // InProgressNICVersion is the number of NICVersion resources currently in progress.
    InProgressNICVersion int32 `json:"inProgressNICVersion,omitempty"`
    // CompletedNICVersion is the number of completed NICVersion resources.
    CompletedNICVersion int32 `json:"completedNICVersion,omitempty"`
    // FailedNICVersion is the number of failed NICVersion resources.
    FailedNICVersion int32 `json:"failedNICVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=nicvs
// +kubebuilder:printcolumn:name="NICVersion",type=string,JSONPath=`.spec.nicVersionTemplate.version`
// +kubebuilder:printcolumn:name="SelectedServers",type=integer,JSONPath=`.status.fullyLabeledServers`
// +kubebuilder:printcolumn:name="Available",type=integer,JSONPath=`.status.availableNICVersion`
// +kubebuilder:printcolumn:name="Pending",type=integer,JSONPath=`.status.pendingNICVersion`
// +kubebuilder:printcolumn:name="InProgress",type=integer,JSONPath=`.status.inProgressNICVersion`
// +kubebuilder:printcolumn:name="Completed",type=integer,JSONPath=`.status.completedNICVersion`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.failedNICVersion`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NICVersionSet is the Schema for the nicversionsets API.
type NICVersionSet struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   NICVersionSetSpec   `json:"spec,omitempty"`
    Status NICVersionSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NICVersionSetList contains a list of NICVersionSet.
type NICVersionSetList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []NICVersionSet `json:"items"`
}

func init() {
    SchemeBuilder.Register(&NICVersionSet{}, &NICVersionSetList{})
}