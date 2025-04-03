// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServerMaintenanceSetSpec defines the desired state of ServerMaintenanceSet.
type ServerMaintenanceSetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ServerLabelSelector specifies a label selector to identify the servers that are to be maintained.
	ServerSelector metav1.LabelSelector `json:"serverLabelSelector,omitempty"`

	Template ServerMaintenanceSpec `json:"template,omitempty"`
}

// ServerMaintenanceSetStatus defines the observed state of ServerMaintenanceSet.
type ServerMaintenanceSetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Maintenances  int32 `json:"maintenances,omitempty"`
	Pending       int32 `json:"pending,omitempty"`
	InMaintenance int32 `json:"inMaintenance,omitempty"`
	Completed     int32 `json:"completed,omitempty"`
	Failed        int32 `json:"failed,omitempty"`
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
