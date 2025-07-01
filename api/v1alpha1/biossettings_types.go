// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BIOSSettingsSpec defines the desired state of BIOSSettings.
type BIOSSettingsSpec struct {
	// Version contains software (eg: BIOS, BMC) version this settings applies to
	// +required
	Version string `json:"version"`

	// SettingsMap contains software (eg: BIOS, BMC) settings as map
	// +optional
	SettingsMap map[string]string `json:"settings,omitempty"`

	// ServerRef is a reference to a specific server to apply bios setting on.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that BiosSetting has requested for the referred server.
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

// BIOSSettingsState specifies the current state of the BIOS maintenance.
type BIOSSettingsState string

const (
	// BIOSSettingsStatePending specifies that the bios setting maintenance is waiting
	BIOSSettingsStatePending BIOSSettingsState = "Pending"
	// BIOSSettingsStateInProgress specifies that the BIOSSetting Controller is updating the settings
	BIOSSettingsStateInProgress BIOSSettingsState = "InProgress"
	// BIOSSettingsStateApplied specifies that the bios setting maintenance has been completed.
	BIOSSettingsStateApplied BIOSSettingsState = "Applied"
	// BIOSSettingsStateFailed specifies that the bios setting maintenance has failed.
	BIOSSettingsStateFailed BIOSSettingsState = "Failed"
)

type BIOSSettingUpdateState string

const (
	// BIOSSettingUpdateWaitOnServerRebootPowerOff specifies that the bios setting state is waiting on server to turn off during Reboot.
	BIOSSettingUpdateWaitOnServerRebootPowerOff BIOSSettingUpdateState = "WaitOnServerRebootPowerOff"
	// BIOSSettingUpdateWaitOnServerRebootPowerOn specifies that the bios setting state is waiting on server to turn on during Reboot.
	BIOSSettingUpdateWaitOnServerRebootPowerOn BIOSSettingUpdateState = "WaitOnServerRebootPowerOn"
	// BIOSSettingUpdateStateIssue specifies that the bios new setting was posted to server's RedFish API
	BIOSSettingUpdateStateIssue BIOSSettingUpdateState = "IssueSettingUpdate"
	// BIOSSettingUpdateStateVerification specifies that the bios setting is beening verified.
	BIOSSettingUpdateStateVerification BIOSSettingUpdateState = "VerifySettingUpdate"
)

// BIOSSettingsStatus defines the observed state of BIOSSettings.
type BIOSSettingsStatus struct {

	// State represents the current state of the bios configuration task.
	State BIOSSettingsState `json:"state,omitempty"`

	AppliedStateTimeStamp *metav1.Time `json:"appliedTimeStamp,omitempty"`

	// Conditions represents the latest available observations of the server's current state.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="BIOSVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="ServerMaintenanceRef",type=string,JSONPath=`.spec.serverMaintenanceRef.name`
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="AppliedOn",type=date,JSONPath=`.status.AppliedStateTimeStamp`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// BIOSSettings is the Schema for the biossettings API.
type BIOSSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BIOSSettingsSpec   `json:"spec,omitempty"`
	Status BIOSSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BIOSSettingsList contains a list of BIOSSettings.
type BIOSSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BIOSSettings `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BIOSSettings{}, &BIOSSettingsList{})
}
