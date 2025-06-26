// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	// OperationAnnotation indicates which operation should be performed outside the current spec definition flow.
	OperationAnnotation = "metal.ironcore.dev/operation"
	// OperationAnnotationIgnore skips the reconciliation of a resource if set to true.
	OperationAnnotationIgnore = "ignore"
	// InstanceTypeAnnotation is used to specify the type of Server.
	InstanceTypeAnnotation = "metal.ironcore.dev/instance-type"
	// OperationAnnotationRotateCredentials is used to indicate that credentials should be rotated.
	OperationAnnotationRotateCredentials = "rotate-credentials"
)
