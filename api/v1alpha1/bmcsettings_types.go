// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// BMCSettingsTemplate defines the template for BMC settings to be applied.
// +kubebuilder:validation:XValidation:rule="!has(self.variables) || self.variables.all(v, self.variables.filter(w, w.key == v.key).size() == 1)",message="variable keys must be unique"
// +kubebuilder:validation:XValidation:rule="!(has(self.settings) && has(self.settingsFlow))",message="settings and settingsFlow are mutually exclusive; migrate to settingsFlow"
type BMCSettingsTemplate struct {
	// Version specifies the BMC firmware version for which the settings should be applied.
	// +optional
	Version string `json:"version,omitempty"`

	// SettingsMap contains BMC settings as a flat key/value map.
	// Deprecated: use settingsFlow instead. This field is mutually exclusive with settingsFlow.
	// This field will be removed in next release.
	// +optional
	SettingsMap map[string]string `json:"settings,omitempty"`

	// SettingsFlow contains BMC settings as a named, priority-ordered list of groups.
	// Replaces the flat settings map. Preferred over settings;
	// +optional
	SettingsFlow []SettingsFlowItem `json:"settingsFlow,omitempty"`

	// Variables is a list of variables that can be used in the settings for templating.
	// +kubebuilder:validation:MaxItems=64
	// +optional
	Variables []Variable `json:"variables,omitempty"`

	// RetryPolicy defines the retry behavior for automatic retries on transient failures.
	// +optional
	RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`

	// ServerMaintenancePolicy is a maintenance policy to be applied on the server.
	// +optional
	ServerMaintenancePolicy ServerMaintenancePolicy `json:"serverMaintenancePolicy,omitempty"`
}

type Variable struct {
	// Key is the name of the variable to be used in the BMCSettingsTemplate format.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +required
	Key string `json:"key"`

	// ValueFrom defines a simple single source for the variable value.
	// +required
	ValueFrom *VariableSourceValueFrom `json:"valueFrom"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.fieldRef) ? 1 : 0) + (has(self.objectFieldRef) ? 1 : 0) + (has(self.configMapKeyRef) ? 1 : 0) + (has(self.secretKeyRef) ? 1 : 0) == 1",message="exactly one of fieldRef, objectFieldRef, configMapKeyRef, or secretKeyRef must be provided"
// +kubebuilder:validation:XValidation:rule="!has(self.objectFieldRef) || self.objectFieldRef.kind == 'BMC'",message="objectFieldRef.kind must be 'BMC'"
type VariableSourceValueFrom struct {
	// FieldRef sources the value from a field of the BMCSettings object itself (e.g. spec.BMCRef.name).
	// Only string-typed fields are supported; integer, bool, or map fields will cause a resolution error.
	// +optional
	FieldRef *FieldRefSelector `json:"fieldRef,omitempty"`

	// ObjectFieldRef sources the value from a field of a named related object.
	// The kind must be "BMC". Supports dot-path navigation and bracket notation for map keys
	// containing dots or slashes (e.g. metadata.labels[kubernetes.metal.cloud.sap/nodename]).
	// +optional
	ObjectFieldRef *ObjectFieldRefSelector `json:"objectFieldRef,omitempty"`

	// ConfigMapKeyRef points to a namespaced ConfigMap key.
	// +optional
	ConfigMapKeyRef *NamespacedKeySelector `json:"configMapKeyRef,omitempty"`

	// SecretKeyRef points to a namespaced Secret key.
	// +optional
	SecretKeyRef *NamespacedKeySelector `json:"secretKeyRef,omitempty"`
}

type FieldRefSelector struct {
	// FieldPath is the path of the field on the BMCSettings object to select (e.g. spec.BMCRef.name).
	// Only string-typed fields are supported; integer, bool, or map fields will cause a resolution error.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +required
	FieldPath string `json:"fieldPath"`
}

// ObjectFieldRefSelector selects a field from a named cluster-scoped object.
// It is intentionally generic; the allowed kinds are constrained at the usage site
// via kubebuilder CEL rules on the parent type.
type ObjectFieldRefSelector struct {
	// Kind is the API kind of the object to read the field from (e.g. "BMC").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +required
	Kind string `json:"kind"`

	// Name is the name of the object to read the field from.
	// Supports $(VAR) substitution using previously resolved variables in declaration order.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// FieldPath is the path of the field to select on the target object.
	// Supports dot-path navigation (e.g. metadata.name) and bracket notation for map
	// keys containing dots or slashes (e.g. metadata.labels[topology.kubernetes.io/region]).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +required
	FieldPath string `json:"fieldPath"`
}

type NamespacedKeySelector struct {
	// Name is the referenced object name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// Namespace is the referenced object namespace.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +required
	Namespace string `json:"namespace"`

	// Key is the key within the referenced object.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Key string `json:"key"`
}

// BMCSettingsSpec defines the desired state of BMCSettings.
// +kubebuilder:validation:XValidation:rule="size(self.version) > 0",message="version is required on BMCSettings"
// +kubebuilder:validation:XValidation:rule="has(self.BMCRef)",message="BMCRef is required"
type BMCSettingsSpec struct {
	BMCSettingsTemplate `json:",inline"`

	// DriftPolicy controls how the controller reacts when hardware deviates from the desired state
	// after the resource has been applied. Empty string (default) means the controller is fully active.
	// Set by the parent CRD; must not be set manually.
	// +optional
	DriftPolicy DriftPolicy `json:"driftPolicy,omitempty"`

	// ServerMaintenanceRefs are references to ServerMaintenance objects which are created by the controller for each
	// server that needs to be updated with the BMC settings.
	// +optional
	ServerMaintenanceRefs []ServerMaintenanceRefItem `json:"serverMaintenanceRefs,omitempty"`

	// BMCRef is a reference to a specific BMC to apply settings to.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="BMCRef is immutable"
	// +required
	BMCRef *corev1.LocalObjectReference `json:"BMCRef"`
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

	// FailedAttempts is the number of automatic retry attempts made after failure.
	// +optional
	FailedAttempts int32 `json:"failedAttempts,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

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
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &BMCSettings{}, &BMCSettingsList{})
		return nil
	})
}
