// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BMCVersionState string

const (
	// BMCVersionStatePending specifies that the BMC upgrade is waiting.
	BMCVersionStatePending BMCVersionState = "Pending"
	// BMCVersionStateInProgress specifies that upgrading BMC is in progress.
	BMCVersionStateInProgress BMCVersionState = "InProgress"
	// BMCVersionStateCompleted specifies that the BMC upgrade maintenance has been completed.
	BMCVersionStateCompleted BMCVersionState = "Completed"
	// BMCVersionStateFailed specifies that the BMC upgrade maintenance has failed.
	BMCVersionStateFailed BMCVersionState = "Failed"
)

// +kubebuilder:validation:XValidation:rule="self.version != \"\"",message="version is required"
// +kubebuilder:validation:XValidation:rule="self.image.URI != \"\"",message="image.URI is required"
type BMCVersionTemplate struct {
	BaseTemplate    `json:",inline"`
	VersionTemplate `json:",inline"`
}

// BMCVersionSpec defines the desired state of BMCVersion.
type BMCVersionSpec struct {
	// BMCVersionTemplate defines the template for BMC version to be applied on the server's BMC.
	BMCVersionTemplate `json:",inline"`

	// DriftPolicy controls the controller's reconcile mode when owned by a MaintenancePipelineRun.
	// Empty (default): active. Observe: drift detection only. Suspend: completely frozen.
	// +optional
	DriftPolicy DriftPolicy `json:"driftPolicy,omitempty"`

	// ServerMaintenanceRefs are references to ServerMaintenance objects that the controller has requested for the related servers.
	// +optional
	ServerMaintenanceRefs []ObjectReference `json:"serverMaintenanceRefs,omitempty"`

	// BMCRef is a reference to a specific BMC to apply BMC upgrade on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="bmcRef is immutable"
	BMCRef *corev1.LocalObjectReference `json:"bmcRef,omitempty"`
}

// BMCVersionStatus defines the observed state of BMCVersion.
type BMCVersionStatus struct {
	// State represents the current state of the BMC configuration task.
	State BMCVersionState `json:"state,omitempty"`

	// UpgradeTask contains the state of the upgrade task created by the BMC.
	UpgradeTask *Task `json:"upgradeTask,omitempty"`

	// FailedAttempts is the number of automatic retry attempts made after failure.
	// +optional
	FailedAttempts int32 `json:"failedAttempts,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the BMC version upgrade state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=bmcv
// +kubebuilder:printcolumn:name="BMCVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="UpdatePolicy",type=string,JSONPath=`.spec.updatePolicy`
// +kubebuilder:printcolumn:name="BMCRef",type=string,JSONPath=`.spec.bmcRef.name`
// +kubebuilder:printcolumn:name="TaskProgress",type=integer,JSONPath=`.status.upgradeTask.percentageComplete`
// +kubebuilder:printcolumn:name="TaskState",type=string,JSONPath=`.status.upgradeTask.state`
// +kubebuilder:printcolumn:name="TaskStatus",type=string,JSONPath=`.status.upgradeTask.status`
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
