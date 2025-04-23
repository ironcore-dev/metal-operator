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

	// BMCSettings specifies the BMC settings for the selected serverRef's Out-of-Band-Management
	BMCSettings BMCSettingsMap `json:"bmcSettings,omitempty"`

	// ServerRef is a reference to a specific server's Manager to apply setting to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRefList []*corev1.LocalObjectReference `json:"serverRefList,omitempty"`

	// BMCRef is a reference to a specific BMC to apply setting to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	BMCRef *corev1.LocalObjectReference `json:"BMCRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server when applying setting.
	// ServerMaintenancePolicyOwnerApproval is asking for human approval if bmc reboot is needed
	// ServerMaintenancePolicyEnforced will not create a maintenance request even if bmc reboot is needed.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that that BMC has requested for the referred server.
	ServerMaintenanceRefMap map[string]*corev1.ObjectReference `json:"serverMaintenanceRefList,omitempty"`
}

type BMCSettingsMap struct {
	// Version contains BMC version
	// +required
	Version string `json:"version"`

	// Settings contains BMC settings as map
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// ServerMaintenanceState specifies the current state of the server maintenance.
type BMCSettingsState string

const (
	// BMCSettingsStateInVersionUpgrade specifies that the server BMC is in version upgrade path.
	BMCSettingsStatePending BMCSettingsState = "Pending"
	// BMCSettingsStateInProgress specifies that the server BMC is in setting update path.
	BMCSettingsStateInProgress BMCSettingsState = "InProgress"
	// BMCSettingsStateApplied specifies that the server BMC maintenance has been completed.
	BMCSettingsStateApplied BMCSettingsState = "Applied"
	// BMCSettingsStateFailed specifies that the server maintenance has failed.
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
