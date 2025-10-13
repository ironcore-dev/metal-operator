// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// establishing general rule for constants naming for Annotations
// "Key" for Annotation constants should be named as <OperationAnnotation><Action>
// e.g.
// OperationAnnotationReset
// OperationAnnotationIgnore
// OperationAnnotationPropagated
// OperationAnnotationForceUpdate
// "Value" for Annotation constants should be named as <Action>OperationAnnotation
// e.g.
// IgnoreOperationAnnotation
// IgnoreChildOperationAnnotation
// IgnoreChildAndSelfOperationAnnotation
// RetryOperationAnnotation

const (
	// OperationAnnotationReset indicates operation should be performed outside the current spec definition flow.
	// This annotation performs Operation on the Server.
	OperationAnnotationReset = "metal.ironcore.dev/operation-reset"
	// ForceResetOperationAnnotation forces a reset before next operation
	ForceResetOperationAnnotation = "ForceReset"

	// OperationAnnotationIgnore indicates which operation should be performed outside the current spec definition flow.
	OperationAnnotationIgnore = "metal.ironcore.dev/operation-ignore"
	// IgnoreOperationAnnotation skips the reconciliation of a resource if OperationAnnotation is set to this.
	IgnoreOperationAnnotation = "ignore"

	// OperationAnnotationPropagated indicates which operation should be performed outside the current spec definition flow.
	OperationAnnotationPropagated = "metal.ironcore.dev/operation-propagated"
	// IgnoreChildOperationAnnotation skips the reconciliation of a resource's Child if OperationAnnotation is set to this.
	IgnoreChildOperationAnnotation = "ignore-child"
	// IgnoreChildAndSelfOperationAnnotation skips the reconciliation of a resource's Child ans self if OperationAnnotation is set to this.
	IgnoreChildAndSelfOperationAnnotation = "ignore-child-and-self"
	// RetryChildOperationAnnotation restarts the reconciliation of a resource's Child if OperationAnnotation is set to this, from failed state -> initial state.
	RetryChildOperationAnnotation = "retry-child"
	// RetryChildAndSelfOperationAnnotation restarts the reconciliation of a resource's Child ans self if OperationAnnotation is set to this, from failed state -> initial state..
	RetryChildAndSelfOperationAnnotation = "retry-child-and-self"

	// AnnotationInstanceType is used to specify the type of Server.
	AnnotationInstanceType = "metal.ironcore.dev/instance-type"

	// OperationAnnotationForceUpdate is used to indicate that the spec should be forcefully updated.
	OperationAnnotationForceUpdate = "metal.ironcore.dev/force-update-resource"
	// ForceUpdateOrDeleteInProgressOperationAnnotation allows update/Delete of a resource even if it is in progress.
	ForceUpdateOrDeleteInProgressOperationAnnotation = "ForceUpdateOrDeleteInProgress"
	// ForceUpdateInProgressOperationAnnotation allows update of a resource even if it is in progress.
	ForceUpdateInProgressOperationAnnotation = "ForceUpdateInProgress"

	// OperationAnnotationRetry indicates which operation should be performed outside the current spec definition flow.
	OperationAnnotationRetry = "metal.ironcore.dev/operation-retry"
	// RetryOperationAnnotation restarts the reconciliation of a resource from failed state -> initial state.
	RetryOperationAnnotation = "retry"
)
