// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
    "github.com/stmcginnis/gofish/schemas"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NICVersionState represents the current state of a NIC firmware upgrade.
type NICVersionState string

const (
    // NICVersionStatePending specifies that the NIC upgrade is waiting.
    NICVersionStatePending NICVersionState = "Pending"
    // NICVersionStateInProgress specifies that the NIC upgrade is in progress.
    NICVersionStateInProgress NICVersionState = "InProgress"
    // NICVersionStateCompleted specifies that the NIC upgrade has completed.
    NICVersionStateCompleted NICVersionState = "Completed"
    // NICVersionStateFailed specifies that the NIC upgrade has failed.
    NICVersionStateFailed NICVersionState = "Failed"
)

// NICVersionConditionType represents a condition type for NICVersion.
type NICVersionConditionType string

const (
    // NICVersionConditionBMCReset indicates the BMC has been reset.
    NICVersionConditionBMCReset NICVersionConditionType = "NICUpgradeBMCReset"
    // NICVersionConditionUpgradeIssued indicates the upgrade request was accepted by the BMC.
    NICVersionConditionUpgradeIssued NICVersionConditionType = "NICUpgradeIssued"
    // NICVersionConditionUpgradeCompleted indicates the upgrade task completed.
    NICVersionConditionUpgradeCompleted NICVersionConditionType = "NICUpgradeCompleted"
    // NICVersionConditionPowerOff indicates the server was powered off after upgrade.
    NICVersionConditionPowerOff NICVersionConditionType = "NICUpgradePowerOff"
    // NICVersionConditionPowerOn indicates the server was powered on after upgrade.
    NICVersionConditionPowerOn NICVersionConditionType = "NICUpgradePowerOn"
    // NICVersionConditionVerification indicates the firmware version was verified.
    NICVersionConditionVerification NICVersionConditionType = "NICUpgradeVerification"
)

// NICVersionTemplate defines the template shared between NICVersion and NICVersionSet.
type NICVersionTemplate struct {
    // Version is the target NIC firmware version to upgrade to.
    // +required
    Version string `json:"version"`

    // UpdatePolicy indicates whether to bypass vendor update policies.
    // +optional
    UpdatePolicy *UpdatePolicy `json:"updatePolicy,omitempty"`

    // Image specifies the firmware binary location.
    // +required
    Image ImageSpec `json:"image"`

    // ServerMaintenancePolicy defines the maintenance approval mode.
    // +optional
    ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`

    // Targets specifies Redfish FirmwareInventory URIs of NIC components to update.
    // Example: ["/redfish/v1/UpdateService/FirmwareInventory/15/"]
    // If empty, the operator will auto-discover NIC targets from FirmwareInventory.
    // +optional
    Targets []string `json:"targets,omitempty"`

    // NICSelector identifies which NIC(s) to update when Targets is empty.
    // Matches against FirmwareInventory entry names (e.g., "ConnectX", "Ethernet").
    // +optional
    NICSelector string `json:"nicSelector,omitempty"`
}

// NICVersionSpec defines the desired state of NICVersion.
type NICVersionSpec struct {
    // NICVersionTemplate defines the template for the NIC firmware upgrade.
    NICVersionTemplate `json:",inline"`

    // ServerMaintenanceRef is a reference to the ServerMaintenance created for this upgrade.
    // +optional
    ServerMaintenanceRef *ObjectReference `json:"serverMaintenanceRef,omitempty"`

    // ServerRef is a reference to the server to upgrade the NIC firmware on.
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="serverRef is immutable"
    // +optional
    ServerRef *corev1.LocalObjectReference `json:"serverRef,omitempty"`
}

// NICVersionStatus defines the observed state of NICVersion.
type NICVersionStatus struct {
    // State is the current state of the NIC firmware upgrade.
    // +optional
    State NICVersionState `json:"state,omitempty"`

    // UpgradeTask contains the BMC task status for the firmware upgrade.
    // +optional
    UpgradeTask *NICTask `json:"upgradeTask,omitempty"`

    // DiscoveredTargets lists the NIC FirmwareInventory URIs found during auto-discovery.
    // +optional
    DiscoveredTargets []string `json:"discoveredTargets,omitempty"`

    // CurrentVersion is the NIC firmware version read from the BMC before upgrade.
    // +optional
    CurrentVersion string `json:"currentVersion,omitempty"`

    // Conditions represents the latest available observations of the NIC upgrade state.
    // +patchStrategy=merge
    // +patchMergeKey=type
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// NICTask contains the status of the upgrade task created by the BMC.
type NICTask struct {
    // URI is the URI of the task created by the BMC.
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
// +kubebuilder:resource:scope=Cluster,shortName=nicv
// +kubebuilder:printcolumn:name="NICVersion",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="UpdatePolicy",type=string,JSONPath=`.spec.updatePolicy`
// +kubebuilder:printcolumn:name="ServerRef",type=string,JSONPath=`.spec.serverRef.name`
// +kubebuilder:printcolumn:name="ServerMaintenanceRef",type=string,JSONPath=`.spec.serverMaintenanceRef.name`
// +kubebuilder:printcolumn:name="TaskState",type=string,JSONPath=`.status.upgradeTask.state`
// +kubebuilder:printcolumn:name="TaskProgress",type=integer,JSONPath=`.status.upgradeTask.percentageComplete`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NICVersion is the Schema for the nicversions API.
type NICVersion struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   NICVersionSpec   `json:"spec,omitempty"`
    Status NICVersionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NICVersionList contains a list of NICVersion.
type NICVersionList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []NICVersion `json:"items"`
}

func init() {
    SchemeBuilder.Register(&NICVersion{}, &NICVersionList{})
}