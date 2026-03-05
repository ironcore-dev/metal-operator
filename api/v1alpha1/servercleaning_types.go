// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServerCleaningSpec defines the desired cleaning operations
// +kubebuilder:validation:XValidation:rule="has(self.serverRef) || has(self.serverSelector)", message="either serverRef or serverSelector must be specified"
type ServerCleaningSpec struct {
	// ServerRef references a specific Server to be cleaned.
	// Mutually exclusive with ServerSelector.
	// +optional
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// ServerSelector specifies a label selector to identify servers to be cleaned.
	// Mutually exclusive with ServerRef.
	// +optional
	ServerSelector *metav1.LabelSelector `json:"serverSelector,omitempty"`

	// DiskWipe specifies disk erasing configuration
	// +optional
	DiskWipe *DiskWipeConfig `json:"diskWipe,omitempty"`

	// BMCReset specifies if BMC should be reset to defaults
	// +optional
	BMCReset bool `json:"bmcReset,omitempty"`

	// BIOSReset specifies if BIOS should be reset to defaults
	// +optional
	BIOSReset bool `json:"biosReset,omitempty"`

	// NetworkCleanup specifies if network configurations should be cleared
	// +optional
	NetworkCleanup bool `json:"networkCleanup,omitempty"`

	// ServerBootConfigurationTemplate defines the boot configuration for cleaning agent
	// If not specified, cleaning operations are performed via BMC APIs
	// +optional
	ServerBootConfigurationTemplate *ServerBootConfigurationTemplate `json:"serverBootConfigurationTemplate,omitempty"`
}

// DiskWipeConfig defines disk erasing behavior
type DiskWipeConfig struct {
	// Method specifies the disk erasing method
	// +kubebuilder:validation:Enum=quick;secure;dod
	// +kubebuilder:default=quick
	Method DiskWipeMethod `json:"method"`

	// IncludeBootDrives specifies whether to erase boot drives
	// +optional
	IncludeBootDrives bool `json:"includeBootDrives,omitempty"`
}

// DiskWipeMethod defines the available disk erasing methods
type DiskWipeMethod string

const (
	// DiskWipeMethodQuick performs a quick erase (single pass)
	DiskWipeMethodQuick DiskWipeMethod = "quick"

	// DiskWipeMethodSecure performs a secure erase (3 passes)
	DiskWipeMethodSecure DiskWipeMethod = "secure"

	// DiskWipeMethodDoD performs DoD 5220.22-M standard erase (7 passes)
	DiskWipeMethodDoD DiskWipeMethod = "dod"
)

// ServerCleaningState defines the state of the cleaning process
type ServerCleaningState string

const (
	// ServerCleaningStatePending indicates cleaning is waiting to start
	ServerCleaningStatePending ServerCleaningState = "Pending"

	// ServerCleaningStateInProgress indicates cleaning is in progress
	ServerCleaningStateInProgress ServerCleaningState = "InProgress"

	// ServerCleaningStateCompleted indicates cleaning completed successfully
	ServerCleaningStateCompleted ServerCleaningState = "Completed"

	// ServerCleaningStateFailed indicates cleaning failed
	ServerCleaningStateFailed ServerCleaningState = "Failed"
)

// ServerCleaningStatus defines the observed state of ServerCleaning
type ServerCleaningStatus struct {
	// State represents the current state of the cleaning process
	// +optional
	State ServerCleaningState `json:"state,omitempty"`

	// SelectedServers is the total number of servers selected for cleaning
	// +optional
	SelectedServers int32 `json:"selectedServers,omitempty"`

	// PendingCleanings is the number of servers with pending cleaning
	// +optional
	PendingCleanings int32 `json:"pendingCleanings,omitempty"`

	// InProgressCleanings is the number of servers currently being cleaned
	// +optional
	InProgressCleanings int32 `json:"inProgressCleanings,omitempty"`

	// CompletedCleanings is the number of servers successfully cleaned
	// +optional
	CompletedCleanings int32 `json:"completedCleanings,omitempty"`

	// FailedCleanings is the number of servers where cleaning failed
	// +optional
	FailedCleanings int32 `json:"failedCleanings,omitempty"`

	// ServerCleaningStatuses contains per-server cleaning status
	// +optional
	ServerCleaningStatuses []ServerCleaningStatusEntry `json:"serverCleaningStatuses,omitempty"`

	// Conditions represents the latest available observations
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// ServerCleaningStatusEntry represents the cleaning status for a single server
type ServerCleaningStatusEntry struct {
	// ServerName is the name of the server
	// +required
	ServerName string `json:"serverName"`

	// State is the cleaning state for this server
	// +required
	State ServerCleaningState `json:"state"`

	// Message provides additional information about the cleaning state
	// +optional
	Message string `json:"message,omitempty"`

	// LastUpdateTime is the last time this status was updated
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// CleaningTasks contains information about the cleaning tasks for this server
	// +optional
	CleaningTasks []CleaningTaskStatus `json:"cleaningTasks,omitempty"`
}

// CleaningTaskStatus represents the status of a cleaning task
type CleaningTaskStatus struct {
	// TaskURI is the URI to monitor the task
	// +optional
	TaskURI string `json:"taskURI,omitempty"`

	// TaskType indicates what type of cleaning task this is
	// +required
	TaskType string `json:"taskType"`

	// TargetID identifies the target resource (e.g., drive ID for disk erase)
	// +optional
	TargetID string `json:"targetID,omitempty"`

	// State is the current state of the task
	// +optional
	State string `json:"state,omitempty"`

	// PercentComplete indicates the completion percentage (0-100)
	// +optional
	PercentComplete int `json:"percentComplete,omitempty"`

	// Message provides additional information about the task
	// +optional
	Message string `json:"message,omitempty"`

	// LastUpdateTime is the last time this task status was updated
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=scl
// +kubebuilder:printcolumn:name="Selected",type=integer,JSONPath=`.status.selectedServers`
// +kubebuilder:printcolumn:name="Completed",type=integer,JSONPath=`.status.completedCleanings`
// +kubebuilder:printcolumn:name="InProgress",type=integer,JSONPath=`.status.inProgressCleanings`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.failedCleanings`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServerCleaning is the Schema for the servercleaning API
type ServerCleaning struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerCleaningSpec   `json:"spec,omitempty"`
	Status ServerCleaningStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerCleaningList contains a list of ServerCleaning
type ServerCleaningList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerCleaning `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerCleaning{}, &ServerCleaningList{})
}
