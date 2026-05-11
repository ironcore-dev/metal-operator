// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// TaintEffect defines the effect of a taint on a ServerClaim.
// +kubebuilder:validation:Enum=NoBind;Evict
type TaintEffect string

const (
	// TaintEffectNoBind prevents new ServerClaims from binding to the server
	// unless they have a matching toleration.
	TaintEffectNoBind TaintEffect = "NoBind"

	// TaintEffectEvict causes existing ServerClaims bound to the server to be
	// evicted if they do not have a matching toleration.
	TaintEffectEvict TaintEffect = "Evict"
)

// Taint represents a taint applied to a Server that restricts which
// ServerClaims can be bound to it.
type Taint struct {
	// Key is the taint key to be applied to a server.
	// +kubebuilder:validation:MinLength=1
	// +required
	Key string `json:"key"`

	// Value is the taint value corresponding to the taint key.
	// +optional
	Value string `json:"value,omitempty"`

	// Effect indicates the effect of the taint on ServerClaims that do not
	// tolerate the taint.
	// +kubebuilder:default=NoBind
	// +optional
	Effect TaintEffect `json:"effect,omitempty"`
}

// TolerationOperator represents a key's relationship to a value in a Toleration.
// +kubebuilder:validation:Enum=Equal;Exists
type TolerationOperator string

const (
	// TolerationOperatorEqual requires that the key and value of the toleration
	// match those of the taint exactly.
	TolerationOperatorEqual TolerationOperator = "Equal"

	// TolerationOperatorExists matches any taint with the specified key,
	// regardless of value.
	TolerationOperatorExists TolerationOperator = "Exists"
)

// Toleration allows a ServerClaim to tolerate taints on a Server so that
// the claim can be bound to a server that would otherwise be restricted.
type Toleration struct {
	// Key is the taint key that the toleration applies to.
	// +kubebuilder:validation:MinLength=1
	// +required
	Key string `json:"key"`

	// Operator represents the key's relationship to the value.
	// +optional
	Operator TolerationOperator `json:"operator,omitempty"`

	// Value is the taint value the toleration matches to.
	// +optional
	Value string `json:"value,omitempty"`

	// Effect indicates the taint effect to tolerate.
	// +optional
	Effect TaintEffect `json:"effect,omitempty"`
}
