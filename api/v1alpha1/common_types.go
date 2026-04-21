// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"encoding/json"
	"net/netip"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
)

// ObjectReference is the namespaced name reference to an object.
type ObjectReference struct {
	// Deprecated: APIVersion is no longer used. Retained for backwards compatibility.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// Deprecated: Kind is no longer used. Retained for backwards compatibility.
	// +optional
	Kind string `json:"kind,omitempty"`
	// Namespace is the namespace of the referenced object.
	// +required
	Namespace string `json:"namespace"`
	// Name is the name of the referenced object.
	// +required
	Name string `json:"name"`
	// Deprecated: UID is no longer used. Retained for backwards compatibility.
	// +optional
	UID types.UID `json:"uid,omitempty"`
}

// ImmutableObjectReference is a namespaced name reference whose name and namespace
// cannot be changed once set (the entire reference can still be set or cleared).
type ImmutableObjectReference struct {
	// Deprecated: APIVersion is no longer used. Retained for backwards compatibility.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// Deprecated: Kind is no longer used. Retained for backwards compatibility.
	// +optional
	Kind string `json:"kind,omitempty"`
	// Namespace is the namespace of the referenced object.
	// +required
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:XValidation:rule="self == oldSelf"
	Namespace string `json:"namespace"`
	// Name is the name of the referenced object.
	// +required
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self == oldSelf"
	Name string `json:"name"`
	// Deprecated: UID is no longer used. Retained for backwards compatibility.
	// +optional
	UID types.UID `json:"uid,omitempty"`
}

// RetryPolicy defines the retry behavior on transient failures.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of automatic retry attempts after failure.
	// 0 means no automatic retries. Must be between 0 and 10 inclusive.
	// If not set, the operator-level default is used.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +optional
	MaxAttempts *int32 `json:"maxAttempts,omitempty"`
}

// IP is an IP address.
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:Format=ip
type IP struct {
	netip.Addr `json:"-"`
}

func (in *IP) DeepCopyInto(out *IP) {
	*out = *in
}

func (in *IP) DeepCopy() *IP {
	return &IP{in.Addr}
}

func (i IP) GomegaString() string {
	return i.String()
}

func (i *IP) UnmarshalJSON(b []byte) error {
	if len(b) == 4 && string(b) == "null" {
		i.Addr = netip.Addr{}
		return nil
	}

	var str string
	err := json.Unmarshal(b, &str)
	if err != nil {
		return err
	}

	p, err := netip.ParseAddr(str)
	if err != nil {
		return err
	}

	i.Addr = p
	return nil
}

func (i IP) MarshalJSON() ([]byte, error) {
	if i.IsZero() {
		// Encode unset/nil objects as JSON's "null".
		return []byte("null"), nil
	}
	return json.Marshal(i.String())
}

func (i IP) ToUnstructured() any {
	if i.IsZero() {
		return nil
	}
	return i.String()
}

func (i *IP) IsValid() bool {
	return i != nil && i.Addr.IsValid()
}

func (i *IP) IsZero() bool {
	return i == nil || !i.Addr.IsValid()
}

func (i IP) Family() v1.IPFamily {
	switch {
	case i.Is4():
		return v1.IPv4Protocol
	case i.Is6():
		return v1.IPv6Protocol
	default:
		return ""
	}
}

func (i IP) OpenAPISchemaType() []string { return []string{"string"} }

func (i IP) OpenAPISchemaFormat() string { return "ip" }

func NewIP(ip netip.Addr) IP {
	return IP{ip}
}

func ParseIP(s string) (IP, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return IP{}, err
	}
	return IP{addr}, nil
}

func ParseNewIP(s string) (*IP, error) {
	ip, err := ParseIP(s)
	if err != nil {
		return nil, err
	}
	return &ip, nil
}

func MustParseIP(s string) IP {
	return IP{netip.MustParseAddr(s)}
}

func MustParseNewIP(s string) *IP {
	ip, err := ParseNewIP(s)
	runtime.Must(err)
	return ip
}

func NewIPPtr(ip netip.Addr) *IP {
	return &IP{ip}
}

func PtrToIP(addr IP) *IP {
	return &addr
}

// IPPrefix represents a network prefix.
// +nullable
type IPPrefix struct {
	netip.Prefix `json:"-"`
}

func (i IPPrefix) GomegaString() string {
	return i.String()
}

func (i IPPrefix) IP() IP {
	return IP{i.Addr()}
}

func (i *IPPrefix) UnmarshalJSON(b []byte) error {
	if len(b) == 4 && string(b) == "null" {
		i.Prefix = netip.Prefix{}
		return nil
	}

	var str string
	err := json.Unmarshal(b, &str)
	if err != nil {
		return err
	}

	p, err := netip.ParsePrefix(str)
	if err != nil {
		return err
	}

	i.Prefix = p
	return nil
}

func (i IPPrefix) MarshalJSON() ([]byte, error) {
	if i.IsZero() {
		// Encode unset/nil objects as JSON's "null".
		return []byte("null"), nil
	}
	return json.Marshal(i.String())
}

func (i IPPrefix) ToUnstructured() any {
	if i.IsZero() {
		return nil
	}
	return i.String()
}

func (in *IPPrefix) DeepCopyInto(out *IPPrefix) {
	*out = *in
}

func (in *IPPrefix) DeepCopy() *IPPrefix {
	return &IPPrefix{in.Prefix}
}

func (in *IPPrefix) IsValid() bool {
	return in != nil && in.Prefix.IsValid()
}

func (in *IPPrefix) IsZero() bool {
	return in == nil || !in.Prefix.IsValid()
}

func (in IPPrefix) OpenAPISchemaType() []string { return []string{"string"} }

func (in IPPrefix) OpenAPISchemaFormat() string { return "ip-prefix" }

func NewIPPrefix(prefix netip.Prefix) *IPPrefix {
	return &IPPrefix{Prefix: prefix}
}

func ParseIPPrefix(s string) (IPPrefix, error) {
	prefix, err := netip.ParsePrefix(s)
	if err != nil {
		return IPPrefix{}, err
	}
	return IPPrefix{prefix}, nil
}

func ParseNewIPPrefix(s string) (*IPPrefix, error) {
	prefix, err := ParseIPPrefix(s)
	if err != nil {
		return nil, err
	}
	return &prefix, nil
}

func MustParseIPPrefix(s string) IPPrefix {
	return IPPrefix{netip.MustParsePrefix(s)}
}

func MustParseNewIPPrefix(s string) *IPPrefix {
	prefix, err := ParseNewIPPrefix(s)
	runtime.Must(err)
	return prefix
}

func PtrToIPPrefix(prefix IPPrefix) *IPPrefix {
	return &prefix
}

func EqualIPPrefixes(a, b IPPrefix) bool {
	return a == b
}

// SettingsFlowItem represents a named, prioritized group of settings to be applied as a unit.
// Used by both BMCSettingsTemplate and BIOSSettingsTemplate.
type SettingsFlowItem struct {
	// Name is the name of the flow item.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1000
	Name string `json:"name"`

	// Priority defines the order of applying the settings. Lower numbers are applied first.
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=2147483645
	Priority int32 `json:"priority"`

	// Settings contains the key=value settings for this step.
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
}

// FieldMatch defines a generic field equality check on the referenced object.
type FieldMatch struct {
	// FieldPath is the dot-notation path to the field on the referenced object,
	// e.g. ".status.state".
	// +required
	FieldPath string `json:"fieldPath"`

	// Value is the expected string value of the field.
	// +required
	Value string `json:"value"`
}

// Variable defines a single named variable used in $(VAR_NAME) substitution
// within BMCSettingsTemplate settings values and ReadinessGate Name fields.
// Variables are resolved by the Set controller at stamp time.
type Variable struct {
	// Key is the variable name, referenced as $(KEY) in template strings.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +required
	Key string `json:"key"`

	// ValueFrom defines a single source for the variable value.
	// +required
	ValueFrom *VariableSourceValueFrom `json:"valueFrom"`
}

// VariableSourceValueFrom defines how to resolve a variable value.
// Exactly one field must be set.
//
// +kubebuilder:validation:XValidation:rule="(has(self.fieldRef) ? 1 : 0) + (has(self.configMapKeyRef) ? 1 : 0) + (has(self.secretKeyRef) ? 1 : 0) + (has(self.ownedByRef) ? 1 : 0) == 1",message="exactly one of fieldRef, configMapKeyRef, secretKeyRef, or ownedByRef must be provided"
type VariableSourceValueFrom struct {
	// FieldRef sources the value from a field of the subject object (e.g. metadata.name).
	// +optional
	FieldRef *FieldRefSelector `json:"fieldRef,omitempty"`

	// ConfigMapKeyRef points to a namespaced ConfigMap key.
	// +optional
	ConfigMapKeyRef *NamespacedKeySelector `json:"configMapKeyRef,omitempty"`

	// SecretKeyRef points to a namespaced Secret key.
	// +optional
	SecretKeyRef *NamespacedKeySelector `json:"secretKeyRef,omitempty"`

	// OwnedByRef finds the sibling child of the named Set that matches the
	// given field filter, and resolves to that child's name.
	// +optional
	OwnedByRef *OwnedByRefSelector `json:"ownedByRef,omitempty"`
}

// FieldRefSelector selects a field on the subject object by dot-notation path.
type FieldRefSelector struct {
	// FieldPath is the path of the field on the subject object to select (e.g. metadata.name).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	// +required
	FieldPath string `json:"fieldPath"`
}

// NamespacedKeySelector references a key within a namespaced ConfigMap or Secret.
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

// OwnedByRefSelector identifies a sibling child of a Set by finding the child
// owned by the named Set whose matchField equals the resolved match value.
type OwnedByRefSelector struct {
	// Kind of the owning Set CRD, e.g. "BMCSettingsSet" or "BMCVersionSet".
	// +required
	Kind string `json:"kind"`

	// Name of the owning Set CRD.
	// +required
	Name string `json:"name"`

	// MatchField filters among all children owned by Kind/Name.
	// The Set controller picks the child where MatchField.FieldPath
	// equals MatchField.Value ($(VAR) substitution applied to Value).
	// +required
	MatchField OwnedByMatchField `json:"matchField"`
}

// OwnedByMatchField specifies the field filter used to identify one sibling child.
type OwnedByMatchField struct {
	// FieldPath is the dot-notation path on the sibling child object to filter by,
	// e.g. ".spec.bmcRef.name".
	// +required
	FieldPath string `json:"fieldPath"`

	// Value is the expected value of FieldPath. Supports $(VAR_NAME) substitution
	// using variables defined earlier in the same gate's variables list.
	// +required
	Value string `json:"value"`
}

// ReadinessGate blocks a resource in Pending until the referenced object
// satisfies the specified check.
//
// Object resolution — Name must resolve to a literal object name.
// On concrete child objects (BMCVersion, BMCSettings) Name is always a
// literal string. In Set templates, Name may contain $(VAR_NAME) references
// that are resolved from the gate's own Variables list by the Set controller
// at stamp time; the stamped child always receives a literal name.
//
// Exactly one of ConditionType or FieldMatch must be set (match criterion):
//   - ConditionType: checks that the named condition is set to True.
//   - FieldMatch: checks that a specific field equals the given value.
//
// +kubebuilder:validation:XValidation:rule="has(self.conditionType) != has(self.fieldMatch)",message="exactly one of conditionType or fieldMatch must be set"
type ReadinessGate struct {
	// APIVersion of the object whose condition is checked, e.g. "metal.ironcore.dev/v1alpha1".
	// +required
	APIVersion string `json:"apiVersion"`

	// Kind of the object whose condition is checked, e.g. "BMCSettings".
	// +required
	Kind string `json:"kind"`

	// Name of the object to look up. On concrete child objects this is always a
	// literal name. It may contain $(VAR_NAME) references
	// resolved from Variables at stamp time.
	// +required
	Name string `json:"name"`

	// ConditionType checks that the named condition on the referenced object is set to True.
	// Mutually exclusive with FieldMatch.
	// +optional
	ConditionType string `json:"conditionType,omitempty"`

	// FieldMatch checks that a specific field on the referenced object equals the given value.
	// Mutually exclusive with ConditionType.
	// +optional
	FieldMatch *FieldMatch `json:"fieldMatch,omitempty"`

	// Variables defines $(VAR_NAME) substitutions used in the Name field of this gate.
	// Resolved by the Set controller at stamp time; always empty on concrete child objects.
	// fieldRef variables are resolved first; ownedByRef variables are resolved second
	// and may reference $(VAR_NAME) values from fieldRef variables in their matchField.value.
	// +optional
	Variables []Variable `json:"variables,omitempty"`
}
