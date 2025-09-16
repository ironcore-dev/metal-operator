// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCSettings refer's to Out-Of-Band_management like IDrac for Dell, iLo for HPE etc Settings

type BMCSettingsTemplate struct {

	// Version defines the BMC firmware for which the settings should be applied.
	// +required
	Version string `json:"version"`

	// SettingsMap contains bmc settings as map
	// +optional
	SettingsMap map[string]string `json:"settings,omitempty"`

	// ServerMaintenancePolicy is a maintenance policy to be applied on the server.
	// +optional
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// ServerMaintenanceRefs are references to ServerMaintenance objects which are created by the controller for each
	// server that needs to be updated with the BMC settings.
	// +optional
	ServerMaintenanceRefs []ServerMaintenanceRefItem `json:"serverMaintenanceRefs,omitempty"`
}

// BMCSettingsSpec defines the desired state of BMCSettings.

type BMCSettingsSpec struct {
	BMCSettingsTemplate `json:",inline"`

	// BMCRef is a reference to a specific BMC to apply setting to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	// +optional
	BMCRef *corev1.LocalObjectReference `json:"BMCRef,omitempty"`
}

// ServerMaintenanceRefItem is a reference to a ServerMaintenance object.
type ServerMaintenanceRefItem struct {
	// ServerMaintenanceRef is a reference to a ServerMaintenance object that the BMCSettings has requested for the referred server.
	// +optional
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

// BMCSettingsState specifies the current state of the server maintenance.
type BMCSettingsState string

const (
	// BMCSettingsStatePending specifies that the BMC maintenance is waiting
	BMCSettingsStatePending BMCSettingsState = "Pending"
	// BMCSettingsStateInProgress specifies that the BMC setting changes are in progress
	BMCSettingsStateInProgress BMCSettingsState = "InProgress"
	// BMCSettingsStateApplied specifies that the BMC maintenance has been completed.
	BMCSettingsStateApplied BMCSettingsState = "Applied"
	// BMCSettingsStateFailed specifies that the BMC maintenance has failed.
	BMCSettingsStateFailed BMCSettingsState = "Failed"
)

// BMCSettingsStatus defines the observed state of BMCSettings.
type BMCSettingsStatus struct {
	// State represents the current state of the BMC configuration task.
	// +optional
	State BMCSettingsState `json:"state,omitempty"`
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
