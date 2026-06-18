// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ShouldAllowForceUpdateInProgress checks if the object should force allow update.
func ShouldAllowForceUpdateInProgress(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationForceUpdateInProgress || val == metalv1alpha1.OperationAnnotationForceUpdateOrDeleteInProgress
}

// ShouldAllowForceDeleteInProgress checks if the object be allowed to be force deleted.
func ShouldAllowForceDeleteInProgress(obj client.Object) bool {
	val, found := obj.GetAnnotations()[metalv1alpha1.OperationAnnotation]
	if !found {
		return false
	}
	return val == metalv1alpha1.OperationAnnotationForceUpdateOrDeleteInProgress
}

// validateDriftPolicy returns a validation error if driftPolicy is set on an object
// that has no owner references. driftPolicy is managed exclusively by the parent CRD
// and must not be set on standalone resources.
func validateDriftPolicy(obj client.Object, driftPolicy metalv1alpha1.DriftPolicy) field.ErrorList {
	if driftPolicy == "" || len(obj.GetOwnerReferences()) > 0 {
		return nil
	}
	return field.ErrorList{
		field.Forbidden(
			field.NewPath("spec").Child("driftPolicy"),
			"driftPolicy may only be set by the parent CRD via an owner reference; must not be set manually",
		),
	}
}

// checkDuplicateBMCSettings returns an error if another BMCSettings in the list targets
// the same BMC and the same version as obj — a true conflict. Two objects for the same BMC
// but different versions (e.g. hop stages) are permitted.
func checkDuplicateBMCSettings(list *metalv1alpha1.BMCSettingsList, obj *metalv1alpha1.BMCSettings) error {
	for _, existing := range list.Items {
		if existing.Name == obj.Name {
			continue
		}
		if existing.Spec.BMCRef.Name == obj.Spec.BMCRef.Name && existing.Spec.Version == obj.Spec.Version {
			return apierrors.NewInvalid(
				schema.GroupKind{Group: obj.GroupVersionKind().Group, Kind: obj.Kind},
				obj.Name,
				field.ErrorList{field.Duplicate(
					field.NewPath("spec").Child("BMCRef"),
					"a BMCSettings for the same BMC and version already exists: "+existing.Name,
				)},
			)
		}
	}
	return nil
}

// checkDuplicateBIOSSettings returns an error if another BIOSSettings in the list targets
// the same server and the same version as obj.
func checkDuplicateBIOSSettings(list *metalv1alpha1.BIOSSettingsList, obj *metalv1alpha1.BIOSSettings) error {
	for _, existing := range list.Items {
		if existing.Name == obj.Name {
			continue
		}
		if existing.Spec.ServerRef.Name == obj.Spec.ServerRef.Name && existing.Spec.Version == obj.Spec.Version {
			return apierrors.NewInvalid(
				schema.GroupKind{Group: obj.GroupVersionKind().Group, Kind: obj.Kind},
				obj.Name,
				field.ErrorList{field.Duplicate(
					field.NewPath("spec").Child("serverRef"),
					"a BIOSSettings for the same server and version already exists: "+existing.Name,
				)},
			)
		}
	}
	return nil
}

// checkDuplicateBMCVersions returns an error if another BMCVersion in the list targets
// the same BMC and the same version as obj.
func checkDuplicateBMCVersions(list *metalv1alpha1.BMCVersionList, obj *metalv1alpha1.BMCVersion) error {
	for _, existing := range list.Items {
		if existing.Name == obj.Name {
			continue
		}
		if existing.Spec.BMCRef.Name == obj.Spec.BMCRef.Name && existing.Spec.Version == obj.Spec.Version {
			return apierrors.NewInvalid(
				schema.GroupKind{Group: obj.GroupVersionKind().Group, Kind: obj.Kind},
				obj.Name,
				field.ErrorList{field.Duplicate(
					field.NewPath("spec").Child("bmcRef"),
					"a BMCVersion for the same BMC and version already exists: "+existing.Name,
				)},
			)
		}
	}
	return nil
}

// checkDuplicateBIOSVersions returns an error if another BIOSVersion in the list targets
// the same server and the same version as obj.
func checkDuplicateBIOSVersions(list *metalv1alpha1.BIOSVersionList, obj *metalv1alpha1.BIOSVersion) error {
	for _, existing := range list.Items {
		if existing.Name == obj.Name {
			continue
		}
		if existing.Spec.ServerRef.Name == obj.Spec.ServerRef.Name && existing.Spec.Version == obj.Spec.Version {
			return apierrors.NewInvalid(
				schema.GroupKind{Group: obj.GroupVersionKind().Group, Kind: obj.Kind},
				obj.Name,
				field.ErrorList{field.Duplicate(
					field.NewPath("spec").Child("serverRef"),
					"a BIOSVersion for the same server and version already exists: "+existing.Name,
				)},
			)
		}
	}
	return nil
}
