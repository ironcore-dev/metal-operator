package app

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// transform returns a list of transformed list elements with function f.
func transform[L ~[]E, E any, T any](list L, f func(E) T) []T {
	ret := make([]T, len(list))
	for i, elem := range list {
		ret[i] = f(elem)
	}
	return ret
}

func metalObjectToString(obj client.Object) string {
	if crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition); ok {
		return "CRD:" + crd.Spec.Names.Kind
	}

	return obj.GetObjectKind().GroupVersionKind().Kind + ":" + client.ObjectKeyFromObject(obj).String()
}
