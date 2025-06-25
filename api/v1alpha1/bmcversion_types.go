// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BMCVersionState string

const (
	// BMCVersionStatePending specifies that the BMC upgrade maintenance is waiting
	BMCVersionStatePending BMCVersionState = "Pending"
	// BMCVersionStateInProgress specifies that upgrading BMC is in progress.
	BMCVersionStateInProgress BMCVersionState = "Processing"
	// BMCVersionStateCompleted specifies that the BMC upgrade maintenance has been completed.
	BMCVersionStateCompleted BMCVersionState = "Completed"
	// BMCVersionStateFailed specifies that the BMC upgrade maintenance has failed.
	BMCVersionStateFailed BMCVersionState = "Failed"
)

// BMCVersionSpec defines the desired state of BMCVersion.
type BMCVersionSpec struct {
	// Version contains BMC version to upgrade to
	// +required
	Version string `json:"version"`
	// An indication of whether the server's upgrade service should bypass vendor update policies
	UpdatePolicy *UpdatePolicy `json:"updatePolicy,omitempty"`
	// details regarding the image to use to upgrade to given BMC version
	// +required
	Image ImageSpec `json:"image"`

	// BMCRef is a reference to a specific BMC to apply BMC upgrade on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	BMCRef *corev1.LocalObjectReference `json:"BMCRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server managed by referred BMC.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// ServerMaintenanceRefs are references to a ServerMaintenance objects that Controller has requested for the each of the related server.
	ServerMaintenanceRefs []ServerMaintenanceRefItem `json:"serverMaintenanceRefs,omitempty"`
}

// BMCVersionStatus defines the observed state of BMCVersion.
type BMCVersionStatus struct {
	// State represents the current state of the BMC configuration task.
	State BMCVersionState `json:"state,omitempty"`

	// UpgradeTask contains the state of the Upgrade Task created by the BMC
	UpgradeTask *TaskStatus `json:"upgradeTask,omitempty"`
	// Conditions represents the latest available observations of the BMC version upgrade state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BMCVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="updateType",type=string,JSONPath=`.spec.updateType`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="ServerMaintenanceRef",type=string,JSONPath=`.spec.serverMaintenanceRef.name`
// +kubebuilder:printcolumn:name="TaskProgress",type=integer,JSONPath=`.status.upgradeTask.percentageComplete`
// +kubebuilder:printcolumn:name="TaskState",type=string,JSONPath=`.status.upgradeTask.taskState`
// +kubebuilder:printcolumn:name="TaskStatus",type=string,JSONPath=`.status.upgradeTask.taskStatus`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BMCVersion is the Schema for the bmcversions API.
type BMCVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCVersionSpec   `json:"spec,omitempty"`
	Status BMCVersionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BMCVersionList contains a list of BMCVersion.
type BMCVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCVersion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCVersion{}, &BMCVersionList{})
}
