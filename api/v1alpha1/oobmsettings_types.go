// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OoBM refer's to Out-Of-Band_management like IDrac for Dell, iLo for HPE etc

// OOBMSettingsSpec defines the desired state of OOBMSettings.
type OOBMSettingsSpec struct {

	// OOBMSettings specifies the OoBM settings for the selected serverRef's Out-of-Band-Management
	OOBMSettings OOBMSettingsMap `json:"OoBM,omitempty"`

	// ServerRef is a reference to a specific server's Manager to apply setting to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`

	// BMCRef is a reference to a specific BMC to apply setting to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
	BMCRef *corev1.LocalObjectReference `json:"BMCRef,omitempty"`

	// ServerMaintenancePolicy is maintenance policy to be enforced on the server when applying setting.
	// ServerMaintenancePolicyOwnerApproval is asking for human approval
	// ServerMaintenancePolicyEnforced will not create a maintenance request.
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"ServerMaintenancePolicyTemplate,omitempty"`

	// ServerMaintenanceRef is a reference to a ServerMaintenance object that that OoBM has requested for the referred server.
	ServerMaintenanceRef *corev1.ObjectReference `json:"serverMaintenanceRef,omitempty"`
}

type OOBMSettingsMap struct {
	// Version contains OoBM version
	// +required
	Version string `json:"version"`

	// Settings contains OoBM settings as map
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// ServerMaintenanceState specifies the current state of the server maintenance.
type OoBMMaintenanceState string

const (
	// OoBMMaintenanceStateInVersionUpgrade specifies that the server OoBM is in version upgrade path.
	OoBMMaintenanceStateInVersionUpgrade OoBMMaintenanceState = "InVersionUpgrade"
	// OoBMMaintenanceStateInSettingUpdate specifies that the server OoBM is in setting update path.
	OoBMMaintenanceStateInSettingUpdate OoBMMaintenanceState = "InSettingUpdate"
	// OoBMMaintenanceStateSynced specifies that the server OoBM maintenance has been completed.
	OoBMMaintenanceStateSynced OoBMMaintenanceState = "SyncSettingsCompleted"
	// OoBMMaintenanceStateFailed specifies that the server maintenance has failed.
	OoBMMaintenanceStateFailed OoBMMaintenanceState = "Failed"
)

type OoBMSettingUpdateState string

const (
	// SettingUpdateStateWaitOnServerReboot specifies that the OoBM setting state is waiting on server to turn off during Reboot.
	OoBMSettingUpdateWaitOnServerRebootPowerOff OoBMSettingUpdateState = "WaitOnServerRebootPowerOff"
	// OoBMSettingUpdateWaitOnServerRebootPowerOn specifies that the OoBM setting state is waiting on server to turn on during Reboot.
	OoBMSettingUpdateWaitOnServerRebootPowerOn OoBMSettingUpdateState = "WaitOnServerRebootPowerOn"
	// SettingUpdateStateIssued specifies that the OoBM new setting was posted to RedFish
	OoBMSettingUpdateStateIssue OoBMSettingUpdateState = "IssueSettingUpdate"
	// SettingUpdateStateCompleted specifies that the OoBM setting has been completed.
	OoBMSettingUpdateStateVerification OoBMSettingUpdateState = "VerifySettingUpdate"
)

// OOBMSettingsStatus defines the observed state of OOBMSettings.
type OOBMSettingsStatus struct {
	// State represents the current state of the OoBM configuration task.
	State OoBMMaintenanceState `json:"state,omitempty"`
	// UpdateSettingState represents the current state of the OoBM setting update task.
	UpdateSettingState OoBMSettingUpdateState `json:"updateSettingState,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// OOBMSettings is the Schema for the oobmsettings API.
type OOBMSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OOBMSettingsSpec   `json:"spec,omitempty"`
	Status OOBMSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OOBMSettingsList contains a list of OOBMSettings.
type OOBMSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OOBMSettings `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OOBMSettings{}, &OOBMSettingsList{})
}
