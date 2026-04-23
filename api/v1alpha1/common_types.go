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
