// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServerMaintenanceReplicaSpec defines the desired state of ServerMaintenanceReplica.
type ServerMaintenanceReplicaSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ServerLabelSelector specifies a label selector to identify the servers that are to be maintained.
	ServerSelector metav1.LabelSelector `json:"serverLabelSelector,omitempty"`

	Template ServerMaintenanceSpec `json:"template,omitempty"`
}

// ServerMaintenanceReplicaStatus defines the observed state of ServerMaintenanceReplica.
type ServerMaintenanceReplicaStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Replicas                  int32 `json:"servers,omitempty"`
	PendingReplicas           int32 `json:"serversPending,omitempty"`
	ServerMaintenanceReplicas int32 `json:"serversInMaintenance,omitempty"`
	CompletedReplicas         int32 `json:"serversCompleted,omitempty"`
	FailedReplicas            int32 `json:"serversFailed,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ServerMaintenanceReplica is the Schema for the ServerMaintenanceReplicas API.
type ServerMaintenanceReplica struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerMaintenanceReplicaSpec   `json:"spec,omitempty"`
	Status ServerMaintenanceReplicaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerMaintenanceReplicaList contains a list of ServerMaintenanceReplica.
type ServerMaintenanceReplicaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerMaintenanceReplica `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerMaintenanceReplica{}, &ServerMaintenanceReplicaList{})
}
