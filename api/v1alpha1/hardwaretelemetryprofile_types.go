// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeliveryMode specifies how telemetry data is delivered from the BMC.
// +kubebuilder:validation:Enum=Polling;EventBased
type DeliveryMode string

const (
	// DeliveryModePolling indicates the operator periodically polls the BMC for metrics.
	DeliveryModePolling DeliveryMode = "Polling"
	// DeliveryModeEventBased indicates the BMC pushes metric reports via Redfish event subscriptions.
	DeliveryModeEventBased DeliveryMode = "EventBased"
)

// TelemetryDelivery configures the telemetry delivery mode.
type TelemetryDelivery struct {
	// Mode is the delivery mode for telemetry data.
	// +kubebuilder:default=Polling
	Mode DeliveryMode `json:"mode"`
}

// TelemetryMetric describes a single metric to collect, identified by its Redfish MetricId
// and further scoped by one or more MetricProperty URIs.
type TelemetryMetric struct {
	// MetricId is the exact Redfish MetricId value to match against MetricValue.MetricId.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	MetricId string `json:"metricId"`
	// Properties is a list of exact Redfish MetricProperty URIs to match.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Properties []string `json:"properties"`
}

// HardwareTelemetryProfileSpec defines the desired state of HardwareTelemetryProfile.
// +kubebuilder:validation:XValidation:rule="!has(self.nodeSelector.matchExpressions) || size(self.nodeSelector.matchExpressions) == 0",message="Only matchLabels is supported in HardwareTelemetryProfile NodeSelector"
// +kubebuilder:validation:XValidation:rule="duration(self.collectionInterval).getSeconds() >= 5",message="collectionInterval must be at least 5s"
// +kubebuilder:validation:XValidation:rule="duration(self.collectionInterval).getSeconds() <= 86400",message="collectionInterval must be at most 24h"
type HardwareTelemetryProfileSpec struct {
	// NodeSelector selects BMC objects by label. Only matchLabels is supported;
	// matchExpressions is rejected via CEL validation.
	NodeSelector metav1.LabelSelector `json:"nodeSelector"`

	// CollectionInterval is the desired polling cadence (e.g. "30s", "5m").
	// Converted to ISO-8601 at the Redfish call site.
	// Must be between 5s and 24h.
	// +kubebuilder:default="30s"
	CollectionInterval metav1.Duration `json:"collectionInterval"`

	// Delivery configures the telemetry delivery mode.
	// +kubebuilder:default={mode: Polling}
	Delivery TelemetryDelivery `json:"delivery,omitempty"`

	// CollectEventLog controls whether the operator scrapes the BMC's event log (SEL) under Mode=Polling.
	// Default true. Ignored under Mode=EventBased (alerts arrive via the event subscription).
	// +optional
	// +kubebuilder:default=true
	CollectEventLog *bool `json:"collectEventLog,omitempty"`

	// Metrics is the list of Redfish metrics to collect.
	// +kubebuilder:validation:MinItems=1
	Metrics []TelemetryMetric `json:"metrics"`

	// Categories is an optional URI-derived bucket filter. Recognised values: "Temperature", "Fan",
	// "Power", "Voltage", "Current", "Utilization", "Memory", "Network", "Storage".
	// Empty means no category filter applies.
	// +optional
	Categories []string `json:"categories,omitempty"`
}

// AppliedNodeStatus reports the per-BMC state resulting from HTP reconciliation.
type AppliedNodeStatus struct {
	// Name is the name of the BMC object.
	Name string `json:"name"`
	// Mode is the effective delivery mode for this BMC ("Polling" or "EventBased").
	Mode string `json:"mode"`
	// MRDActive is true when the controller has confirmed that the BMC-side
	// MetricReportDefinition is active and pushing data.
	// +optional
	MRDActive bool `json:"mrdActive,omitempty"`
	// MRDSupported is false when the BMC rejected the MRD write or the
	// post-create probe found no data.
	// +optional
	MRDSupported bool `json:"mrdSupported,omitempty"`
	// LastAppliedAt is the time the profile was last successfully applied to this BMC.
	// +optional
	LastAppliedAt metav1.Time `json:"lastAppliedAt,omitempty"`
}

// HardwareTelemetryProfileStatus defines the observed state of HardwareTelemetryProfile.
type HardwareTelemetryProfileStatus struct {
	// Conditions holds the conditions for the HardwareTelemetryProfile.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// AppliedNodes lists the per-BMC status for every node currently matched by this profile.
	// +optional
	AppliedNodes []AppliedNodeStatus `json:"appliedNodes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=htp
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.delivery.mode`
// +kubebuilder:printcolumn:name="Interval",type=string,JSONPath=`.spec.collectionInterval`
// +kubebuilder:printcolumn:name="Nodes",type=integer,JSONPath=`.status.appliedNodes[*].name`

// HardwareTelemetryProfile is the Schema for the HardwareTelemetryProfile API.
type HardwareTelemetryProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HardwareTelemetryProfileSpec   `json:"spec,omitempty"`
	Status HardwareTelemetryProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HardwareTelemetryProfileList contains a list of HardwareTelemetryProfile.
type HardwareTelemetryProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwareTelemetryProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &HardwareTelemetryProfile{}, &HardwareTelemetryProfileList{})
		return nil
	})
}
