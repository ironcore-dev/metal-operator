// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "github.com/stmcginnis/gofish/redfish"

// establishing general rule for constants naming for Annotations
// "Key" for Annotation constants should be named as <OperationAnnotation><Action>
// e.g.
// OperationAnnotation
// "Value" for Annotation constants should be named as <Action>OperationAnnotation
// e.g.
// IgnoreOperationAnnotation
// IgnoreChildOperationAnnotation
// IgnoreChildAndSelfOperationAnnotation
// RetryOperationAnnotation

const (
	// OperationAnnotation indicates operation should be performed outside the current spec definition flow.
	// This annotation performs Operation on the Server.
	OperationAnnotation = "metal.ironcore.dev/operation"

	// IgnoreOperationAnnotation skips the reconciliation of a resource if OperationAnnotation is set to this.
	IgnoreOperationAnnotation = "ignore-reconciliation"

	// OperationAnnotationPropagated indicates OperationAnnotation operation is being propagated to Child resources from its Parent.
	// This annotation is set by the operator when parent resource propagates Operation on child resources.
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

	// ForceUpdateOrDeleteInProgressOperationAnnotation allows update/Delete of a resource even if it is in progress.
	ForceUpdateOrDeleteInProgressOperationAnnotation = "force-update-delete-InProgress"
	// ForceUpdateInProgressOperationAnnotation allows update of a resource even if it is in progress.
	ForceUpdateInProgressOperationAnnotation = "force-update-InProgress"

	// RetryFailedOperationAnnotation restarts the reconciliation of a resource from failed state -> initial state.
	RetryFailedOperationAnnotation = "retry-failed-state-resource"
)

const (
	// GracefulShutdownServerPower indicates to gracefully restart the baremetal server power.
	GracefulRestartServerPower = "graceful-restart-server-power"
	// HardRestartServerPower indicates to hard restart the baremetal server power.
	HardRestartServerPower = "hard-restart-server-power"
	// PowerOffServerPower indicates to power cycle the baremetal server.
	PowerCycleServerPower = "power-cycle-server-power"
	// ForceOffServerPower indicates to force powerOff the baremetal server power.
	ForceOffServerPower = "force-off-server-power"
	// ForceOnServerPower indicates to force powerOn the baremetal server power.
	ForceOnServerPower = "force-on-server-power"
)

var AnnotationToRedfishMapping = map[string]redfish.ResetType{
	GracefulRestartServerPower: redfish.OnResetType,
	HardRestartServerPower:     redfish.ForceRestartResetType,
	PowerCycleServerPower:      redfish.PowerCycleResetType,
	ForceOffServerPower:        redfish.ForceOffResetType,
	ForceOnServerPower:         redfish.ForceOnResetType,
}

const (
	// ForceResetBMCOperationAnnotation forces a reset of BMC before next operation
	ForceResetBMCOperationAnnotation = "force-reset-BMC"
)
