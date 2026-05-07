// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PipelineStageKind identifies the type of child resource a pipeline stage manages.
// +kubebuilder:validation:Enum=BMCSettings;BMCVersion;BIOSSettings;BIOSVersion
type PipelineStageKind string

const (
	// PipelineStageKindBMCSettings creates a BMCSettings child (BMC-scoped, once per Run).
	PipelineStageKindBMCSettings PipelineStageKind = "BMCSettings"
	// PipelineStageKindBMCVersion creates BMCVersion children, one per hop (BMC-scoped, once per Run).
	PipelineStageKindBMCVersion PipelineStageKind = "BMCVersion"
	// PipelineStageKindBIOSSettings creates BIOSSettings children (Server-scoped, one per server in serverRefs).
	PipelineStageKindBIOSSettings PipelineStageKind = "BIOSSettings"
	// PipelineStageKindBIOSVersion creates BIOSVersion children, one per hop per server (Server-scoped).
	PipelineStageKindBIOSVersion PipelineStageKind = "BIOSVersion"
)

// PipelineStage defines a single stage in the maintenance pipeline.
// Stages execute strictly in list order; each stage must reach Completed before the next begins.
// kind acts as the discriminator: it determines which template fields are valid.
// CEL rules enforce that settings-only fields (settingsFlow, variables) are present only for
// BMCSettings/BIOSSettings kinds, and version-only fields (image) only for BMCVersion/BIOSVersion kinds.
// +kubebuilder:validation:XValidation:rule="(self.kind == 'BMCSettings' || self.kind == 'BIOSSettings') ? !has(self.template.image) : (has(self.template.image) && self.template.image.URI != ”)",message="image.URI is required for BMCVersion/BIOSVersion stages and must not be set on settings stages"
// +kubebuilder:validation:XValidation:rule="(self.kind == 'BMCVersion' || self.kind == 'BIOSVersion') ? (!has(self.template.settingsFlow) || self.template.settingsFlow.size() == 0) : true",message="settingsFlow is only valid on BMCSettings/BIOSSettings stages"
// +kubebuilder:validation:XValidation:rule="(self.kind == 'BMCVersion' || self.kind == 'BIOSVersion') ? (!has(self.template.variables) || self.template.variables.size() == 0) : true",message="variables is only valid on BMCSettings/BIOSSettings stages"
type PipelineStage struct {
	// Name is the unique identifier for this stage within the pipeline.
	// Used as a label value and for status tracking.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// Kind determines the type of child resource this stage creates and its target scope.
	// BMCSettings / BMCVersion are BMC-scoped (one child per Run).
	// BIOSSettings / BIOSVersion are Server-scoped (one child per server in serverRefs).
	// +required
	Kind PipelineStageKind `json:"kind"`

	// Template defines the desired state for the child resource this stage creates.
	// kind acts as the discriminator — see PipelineStageTemplate for field validity per kind.
	// +required
	Template PipelineStageTemplate `json:"template"`
}

// MaintenancePipelineSpec defines the desired state of MaintenancePipeline.
type MaintenancePipelineSpec struct {
	// ServerSelector selects the Server objects this pipeline targets.
	// The controller resolves each server's spec.bmcRef to group servers by BMC and create
	// one MaintenancePipelineRun per unique BMC.
	// +required
	ServerSelector metav1.LabelSelector `json:"serverSelector"`

	// MaxConcurrent caps the number of MaintenancePipelineRun objects (unique BMCs)
	// that may be in InProgress phase simultaneously. Controls fleet-level blast radius.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxConcurrent int32 `json:"maxConcurrent,omitempty"`

	// DriftPolicy defines what the pipeline does when hardware deviates from the desired state
	// after a stage has completed. Only Reconcile and Observe are valid at the pipeline level;
	// Suspend is reserved for child resources managed by MaintenancePipelineRun.
	// +kubebuilder:default=Reconcile
	// +kubebuilder:validation:Enum=Reconcile;Observe
	// +optional
	DriftPolicy DriftPolicy `json:"driftPolicy,omitempty"`

	// Stages is the ordered list of maintenance stages for this pipeline.
	// Stages execute strictly in list order; each stage must reach Completed before the next begins.
	// +kubebuilder:validation:MinItems=1
	// +required
	Stages []PipelineStage `json:"stages"`
}

// MaintenancePipelineRunSummary aggregates run phase counts across all owned MaintenancePipelineRun objects.
type MaintenancePipelineRunSummary struct {
	// Total is the total number of MaintenancePipelineRun objects owned by this pipeline.
	Total int32 `json:"total"`

	// Pending is the number of runs in Pending phase.
	// +optional
	Pending int32 `json:"pending,omitempty"`

	// InProgress is the number of runs currently executing.
	// +optional
	InProgress int32 `json:"inProgress,omitempty"`

	// Completed is the number of runs that have reached Completed phase.
	// +optional
	Completed int32 `json:"completed,omitempty"`

	// Failed is the number of runs in Failed phase.
	// +optional
	Failed int32 `json:"failed,omitempty"`
}

// MaintenancePipelineStatus defines the observed state of MaintenancePipeline.
type MaintenancePipelineStatus struct {
	// Runs summarises the phase distribution of all owned MaintenancePipelineRun objects.
	// +optional
	Runs *MaintenancePipelineRunSummary `json:"runs,omitempty"`

	// Conditions represents the latest observations of the pipeline.
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=mp
// +kubebuilder:printcolumn:name="MaxConcurrent",type=integer,JSONPath=`.spec.maxConcurrent`
// +kubebuilder:printcolumn:name="DriftPolicy",type=string,JSONPath=`.spec.driftPolicy`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.runs.total`
// +kubebuilder:printcolumn:name="InProgress",type=integer,JSONPath=`.status.runs.inProgress`
// +kubebuilder:printcolumn:name="Completed",type=integer,JSONPath=`.status.runs.completed`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.runs.failed`
// +kubebuilder:printcolumn:name="Pending",type=integer,JSONPath=`.status.runs.pending`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MaintenancePipeline is the Schema for the maintenancepipelines API.
type MaintenancePipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MaintenancePipelineSpec   `json:"spec,omitempty"`
	Status MaintenancePipelineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MaintenancePipelineList contains a list of MaintenancePipeline.
type MaintenancePipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaintenancePipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MaintenancePipeline{}, &MaintenancePipelineList{})
}
