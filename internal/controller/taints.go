// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

// tolerationMatchesTaint returns true if the given toleration matches the given taint.
func tolerationMatchesTaint(toleration v1alpha1.Toleration, taint v1alpha1.Taint) bool {
	// Keys must always match.
	if toleration.Key != taint.Key {
		return false
	}

	// If the toleration specifies an effect, it must match the taint's effect.
	if toleration.Effect != "" && toleration.Effect != taint.Effect {
		return false
	}

	// Check value matching based on the operator.
	switch toleration.Operator {
	case v1alpha1.TolerationOperatorExists:
		// Exists: key (and optionally effect) must match; value is ignored.
		return true
	case v1alpha1.TolerationOperatorEqual, "":
		// Equal (or empty, treated as Equal): key AND value must match.
		return toleration.Value == taint.Value
	default:
		return false
	}
}

// tolerates returns true if the given tolerations cover all NoBind taints in the
// given taint list. Evict taints are skipped (future work) and are always
// considered tolerated.
func tolerates(taints []v1alpha1.Taint, tolerations []v1alpha1.Toleration) bool {
	for _, taint := range taints {
		// Only NoBind blocks binding; skip Evict taints entirely.
		if taint.Effect != v1alpha1.TaintEffectNoBind {
			continue
		}

		tolerated := false
		for _, toleration := range tolerations {
			if tolerationMatchesTaint(toleration, taint) {
				tolerated = true
				break
			}
		}
		if !tolerated {
			return false
		}
	}
	return true
}
