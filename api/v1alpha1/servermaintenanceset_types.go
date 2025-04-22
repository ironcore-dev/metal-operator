// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerMaintenanceSetSpec defines the desired state of ServerMaintenanceSet.
type ServerMaintenanceSetSpec struct {
	// ServerLabelSelector specifies a label selector to identify the servers that are to be maintained.
	ServerSelector metav1.LabelSelector `json:"serverLabelSelector,omitempty"`
	// Template specifies the template for the server maintenance.
	Template ServerMaintenanceSpec `json:"template,omitempty"`
}

// ServerMaintenanceSetStatus defines the observed state of ServerMaintenanceSet.
type ServerMaintenanceSetStatus struct {
	// Maintenances is the number of server maintenances in the set.
	Maintenances int32 `json:"maintenances,omitempty"`
	// Pending is the total number of pending server maintenances in the set.
	Pending int32 `json:"pending,omitempty"`
	// InMaintenance is the total number of server maintenances in the set that are currently in maintenance.
	InMaintenance int32 `json:"inMaintenance,omitempty"`
	// Completed is the total number of completed server maintenances in the set.
	Completed int32 `json:"completed,omitempty"`
	// Failed is the total number of failed server maintenances in the set.
	Failed int32 `json:"failed,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ServerMaintenanceSet is the Schema for the ServerMaintenanceSet API.
type ServerMaintenanceSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerMaintenanceSetSpec   `json:"spec,omitempty"`
	Status ServerMaintenanceSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerMaintenanceSetList contains a list of ServerMaintenanceSet.
type ServerMaintenanceSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerMaintenanceSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerMaintenanceSet{}, &ServerMaintenanceSetList{})
}
