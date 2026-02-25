// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/stmcginnis/gofish/schemas"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BIOSVersionState string

const (
	// BIOSVersionStatePending specifies that the BIOS upgrade is waiting.
	BIOSVersionStatePending BIOSVersionState = "Pending"
	// BIOSVersionStateInProgress specifies that upgrading BIOS is in progress.
	BIOSVersionStateInProgress BIOSVersionState = "InProgress"
	// BIOSVersionStateCompleted specifies that the BIOS upgrade has been completed.
	BIOSVersionStateCompleted BIOSVersionState = "Completed"
	// BIOSVersionStateFailed specifies that the BIOS upgrade has failed.
	BIOSVersionStateFailed BIOSVersionState = "Failed"
)

type UpdatePolicy string

const (
	UpdatePolicyForce UpdatePolicy = "Force"
)

type BIOSVersionTemplate struct {
	// Version specifies the BIOS version to upgrade to.
	// +required
	Version string `json:"version"`

	// UpdatePolicy indicates whether the server's upgrade service should bypass vendor update policies.
	// +optional
	UpdatePolicy *UpdatePolicy `json:"updatePolicy,omitempty"`

	// Image specifies the image to use to upgrade to the given BIOS version.
	// +required
	Image ImageSpec `json:"image"`

	// ServerMaintenancePolicy is a maintenance policy to be enforced on the server.
	// +optional
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// FailedAutoRetryCount is the number of times the controller should automatically retry the BIOSVersion upgrade in case of failure before giving up.
	// +kubebuilder:validation:Minimum=0
	// +optional
	FailedAutoRetryCount *int32 `json:"failedAutoRetryCount,omitempty"`
}

// BIOSVersionSpec defines the desired state of BIOSVersion.
type BIOSVersionSpec struct {
	// BIOSVersionTemplate defines the template for Version to be applied on the servers.
	BIOSVersionTemplate `json:",inline"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that the controller has requested for the referred server.
	// +optional
	ServerMaintenanceRef *ObjectReference `json:"serverMaintenanceRef,omitempty"`

	// ServerRef is a reference to a specific server to apply the BIOS upgrade on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	// +optional
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`
}

type ImageSpec struct {
	// SecretRef is a reference to the Secret containing the credentials to access the image URI.
	// +optional
	SecretRef *corev1.SecretReference `json:"secretRef,omitempty"`

	// TransferProtocol is the network protocol used to retrieve the image URI.
	// +optional
	TransferProtocol string `json:"transferProtocol,omitempty"`

	// URI is the URI of the software image to install.
	// +required
	URI string `json:"URI"`
}

// BIOSVersionStatus defines the observed state of BIOSVersion.
type BIOSVersionStatus struct {
	// State represents the current state of the BIOS upgrade task.
	// +optional
	State BIOSVersionState `json:"state,omitempty"`

	// UpgradeTask contains the state of the Upgrade Task created by the BMC
	// +optional
	UpgradeTask *Task `json:"upgradeTask,omitempty"`

	// AutoRetryCountRemaining is the number of remaining times the controller will automatically retry the BIOSVersion upgrade in case of failure before giving up.
	// +optional
	AutoRetryCountRemaining *int32 `json:"autoRetryCountRemaining,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the BIOS version upgrade state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// Task contains the status of the task created by the BMC for the BIOS upgrade.
type Task struct {
	// URI is the URI of the task created by the BMC for the BIOS upgrade.
	// +optional
	URI string `json:"URI,omitempty"`

	// State is the current state of the task.
	// +optional
	State schemas.TaskState `json:"state,omitempty"`

	// Status is the current status of the task.
	// +optional
	Status schemas.Health `json:"status,omitempty"`

	// PercentComplete is the percentage of completion of the task.
	// +optional
	PercentComplete int32 `json:"percentageComplete,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=biosv
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="UpdatePolicy",type=string,JSONPath=`.spec.updatePolicy`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="ServerMaintenanceRef",type=string,JSONPath=`.spec.serverMaintenanceRef.name`
// +kubebuilder:printcolumn:name="TaskState",type=string,JSONPath=`.status.upgradeTask.state`
// +kubebuilder:printcolumn:name="TaskStatus",type=string,JSONPath=`.status.upgradeTask.status`
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
