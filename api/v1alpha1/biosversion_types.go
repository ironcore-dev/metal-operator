// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BIOSVersionState string

const (
	// BIOSVersionStatePending specifies that the bios upgrade maintenance is waiting
	BIOSVersionStatePending BIOSVersionState = "Pending"
	// BIOSVersionStateInProgress specifies that upgrading bios is in progress.
	BIOSVersionStateInProgress BIOSVersionState = "Processing"
	// BIOSVersionStateCompleted specifies that the bios upgrade maintenance has been completed.
	BIOSVersionStateCompleted BIOSVersionState = "Completed"
	// BIOSVersionStateFailed specifies that the bios upgrade maintenance has failed.
	BIOSVersionStateFailed BIOSVersionState = "Failed"
)

type UpdatePolicy string

const (
	UpdatePolicyForce UpdatePolicy = "Force"
)

// BIOSVersionSpec defines the desired state of BIOSVersion.
type BIOSVersionSpec struct {
	// Version contains BIOS version to upgrade to
	// +required
	Version string `json:"version"`
	// An indication of whether the server's upgrade service should bypass vendor update policies
	UpdatePolicy *UpdatePolicy `json:"updatePolicy,omitempty"`
	// details regarding the image to use to upgrade to given BIOS version
	// +required
	Image ImageSpec `json:"image"`

	// ServerRef is a reference to a specific server to apply bios upgrade on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that that Controller has requested for the referred server.
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

type ImageSpec struct {
	// ImageSecretRef is a reference to the Kubernetes Secret (of type SecretTypeBasicAuth) object that contains the credentials
	// to access the ImageURI. This secret includes sensitive information such as usernames and passwords.
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
	// The network protocol that the server's update service uses to retrieve 'ImageURI'
	TransferProtocol string `json:"transferProtocol,omitempty"`
	// The URI of the software image to update/install."
	// +required
	URI string `json:"URI"`
}

// BIOSVersionStatus defines the observed state of BIOSVersion.
type BIOSVersionStatus struct {
	// State represents the current state of the bios configuration task.
	State BIOSVersionState `json:"state,omitempty"`

	// UpgradeTask contains the state of the Upgrade Task created by the BMC
	UpgradeTask *TaskStatus `json:"upgradeTask,omitempty"`
	// Conditions represents the latest available observations of the Bios version upgrade state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

type TaskStatus struct {
	TaskURI         string            `json:"taskURI,omitempty"`
	State           redfish.TaskState `json:"taskState,omitempty"`
	Status          common.Health     `json:"taskStatus,omitempty"`
	PercentComplete int               `json:"percentageComplete,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="ForceUpdate",type=string,JSONPath=`.spec.updateType`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="ServerMaintenanceRef",type=string,JSONPath=`.spec.serverMaintenanceRef.name`
// +kubebuilder:printcolumn:name="TaskState",type=string,JSONPath=`.status.upgradeTask.taskState`
// +kubebuilder:printcolumn:name="TaskStatus",type=string,JSONPath=`.status.upgradeTask.taskStatus`
// +kubebuilder:printcolumn:name="TaskProgress",type=integer,JSONPath=`.status.upgradeTask.percentageComplete`
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BIOSVersion is the Schema for the biosversions API.
type BIOSVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BIOSVersionSpec   `json:"spec,omitempty"`
	Status BIOSVersionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BIOSVersionList contains a list of BIOSVersion.
type BIOSVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BIOSVersion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BIOSVersion{}, &BIOSVersionList{})
}
