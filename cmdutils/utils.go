// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package cmdutils

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// transform returns a list of transformed list elements with function f.
func transform[L ~[]E, E any, T any](list L, f func(E) T) []T {
	ret := make([]T, len(list))
	for i, elem := range list {
		ret[i] = f(elem)
	}
	return ret
}

func crName(cr *unstructured.Unstructured) string {
	return cr.GetObjectKind().GroupVersionKind().Kind + ":" + cr.GetNamespace() + "/" + cr.GetName()
}
