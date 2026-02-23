// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCSettingsTemplate defines the template for BMC settings to be applied.

type BMCSettingsTemplate struct {
	// Version specifies the BMC firmware version for which the settings should be applied.
	// +required
	Version string `json:"version"`

	// SettingsMap contains BMC settings as a map.
	// +optional
	SettingsMap map[string]string `json:"settings,omitempty"`

	// FailedAutoRetryCount is the number of times the controller should automatically retry the BMCSettings upgrade in case of failure before giving up.
	// kubebuilder:validation:Minimum=0
	// +optional
	FailedAutoRetryCount *int32 `json:"failedAutoRetryCount,omitempty"`

	// ServerMaintenancePolicy is a maintenance policy to be applied on the server.
	// +optional
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`
}

// BMCSettingsSpec defines the desired state of BMCSettings.
type BMCSettingsSpec struct {
	BMCSettingsTemplate `json:",inline"`

	// ServerMaintenanceRefs are references to ServerMaintenance objects which are created by the controller for each
	// server that needs to be updated with the BMC settings.
	// +optional
	ServerMaintenanceRefs []ServerMaintenanceRefItem `json:"serverMaintenanceRefs,omitempty"`

	// BMCRef is a reference to a specific BMC to apply settings to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="BMCRef is immutable"
	// +required
	BMCRef *corev1.LocalObjectReference `json:"BMCRef,omitempty"`
}

// ServerMaintenanceRefItem is a reference to a ServerMaintenance object.
type ServerMaintenanceRefItem struct {
	// ServerMaintenanceRef is a reference to a ServerMaintenance object that the BMCSettings has requested for the referred server.
	// +optional
	ServerMaintenanceRef *ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

// BMCSettingsState specifies the current state of the server maintenance.
type BMCSettingsState string

const (
	// BMCSettingsStatePending specifies that the BMC settings update is waiting.
	BMCSettingsStatePending BMCSettingsState = "Pending"
	// BMCSettingsStateInProgress specifies that the BMC settings changes are in progress.
	BMCSettingsStateInProgress BMCSettingsState = "InProgress"
	// BMCSettingsStateApplied specifies that the BMC settings have been applied.
	BMCSettingsStateApplied BMCSettingsState = "Applied"
	// BMCSettingsStateFailed specifies that the BMC settings update has failed.
	BMCSettingsStateFailed BMCSettingsState = "Failed"
)

// BMCSettingsStatus defines the observed state of BMCSettings.
type BMCSettingsStatus struct {
	// State represents the current state of the BMC configuration task.
	// +optional
	State BMCSettingsState `json:"state,omitempty"`

	// AutoRetryCountRemaining is the number of remaining times the controller will automatically retry the BMCSettings upgrade in case of failure before giving up.
	// +optional
	AutoRetryCountRemaining *int32 `json:"autoRetryCountRemaining,omitempty"`

	// Conditions represents the latest available observations of the BMC Settings Resource state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BMCVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="BMCRef",type=string,JSONPath=`.spec.BMCRef.name`

// BMCSettings is the Schema for the BMCSettings API.
type BMCSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCSettingsSpec   `json:"spec,omitempty"`
	Status BMCSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BMCSettingsList contains a list of BMCSettings.
type BMCSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCSettings `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCSettings{}, &BMCSettingsList{})
}
