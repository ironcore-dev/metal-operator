// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	// OperationAnnotation indicates which operation should be performed outside the current spec definition flow.
	OperationAnnotation = "metal.ironcore.dev/operation"
	// PropogatedOperationAnnotation indicates which operation should be performed outside the current spec definition flow.
	PropogatedOperationAnnotation = "metal.ironcore.dev/operationPropogated"
	// OperationAnnotationIgnore skips the reconciliation of a resource if set to true.
	OperationAnnotationIgnore = "ignore"
	// PropogatedOperationAnnotationIgnored skips the reconciliation of a resource if set to true.
	PropogatedOperationAnnotationIgnored = "propogatedIgnored"
	// OperationAnnotationRetry restarts the reconciliation of a resource from failed state -> initial state.
	OperationAnnotationRetry = "retry"
	// InstanceTypeAnnotation is used to specify the type of Server.
	InstanceTypeAnnotation = "metal.ironcore.dev/instance-type"
	// ForceUpdateAnnotation is used to indicate that the spec should be forcefully updated.
	ForceUpdateAnnotation = "metal.ironcore.dev/force-update-resource"
	// OperationAnnotationForceUpdateOrDeleteInProgress allows update/Delete of a resource even if it is in progress.
	OperationAnnotationForceUpdateOrDeleteInProgress = "ForceUpdateOrDeleteInProgress"
	// OperationAnnotationForceUpdateInProgress allows update of a resource even if it is in progress.
	OperationAnnotationForceUpdateInProgress = "ForceUpdateInProgress"
)
