// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCSettings refer's to Out-Of-Band_management like IDrac for Dell, iLo for HPE etc Settings

// BMCSettingsSpec defines the desired state of BMCSettings.
type BMCSettingsSpec struct {

	// Version contains BMC version this settings applies to
	// +required
	Version string `json:"version"`

	// SettingsMap contains bmc settings as map
	// +optional
	SettingsMap map[string]string `json:"settings,omitempty"`

	// BMCRef is a reference to a specific BMC to apply setting to.
	// ServerRef is ignored if BMCRef is set
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	BMCRef *corev1.LocalObjectReference `json:"BMCRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server when applying setting.
	// ServerMaintenancePolicyOwnerApproval is asking for User approval for changing BMC settings
	//	note: User approval is only enforced for server's which are reserved state
	// ServerMaintenancePolicyEnforced will not create a maintenance request even if bmc reboot is needed.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// ServerMaintenanceRefList are references to a ServerMaintenance objects that Controller has requested for the each of the related server.
	ServerMaintenanceRefList []ServerMaintenanceRefList `json:"serverMaintenanceRefList,omitempty"`
}

type ServerMaintenanceRefList struct {
	ServerName           string                  `json:"serverName,omitempty"`
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

// ServerMaintenanceState specifies the current state of the server maintenance.
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
	State BMCSettingsState `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BMCVersion",type=string,JSONPath=`.spec.bmcSettings.version`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="BMCRef",type=string,JSONPath=`.spec.BMCRef.name`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`

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
