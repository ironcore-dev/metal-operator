// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "github.com/stmcginnis/gofish/redfish"

// establishing general rule for constants naming for Annotations
// "Key" for Annotation constants should be named OperationAnnotation.
// we do not want to handle multiple annotation keys for outside the spec flow operations.
// "Value" for Annotation constants should be named as OperationAnnotation<ActionType>
// e.g.
// OperationAnnotationIgnore
// OperationAnnotationIgnoreChild
// OperationAnnotationIgnoreChildAndSelf
// OperationAnnotationRetry

const (
	// OperationAnnotation indicates operation should be performed outside the current spec definition flow.
	// This annotation performs Operation on the Server.
	OperationAnnotation = "metal.ironcore.dev/operation"
	// OperationAnnotationIgnore skips the reconciliation of a resource if OperationAnnotation is set to this.
	OperationAnnotationIgnore = "ignore-reconciliation"
	// OperationAnnotationIgnorePropagated skips the reconciliation of a resource's Child if OperationAnnotation is set to this.
	OperationAnnotationIgnorePropagated = "ignore-reconciliation-propagated"
	// OperationAnnotationIgnoreChild skips the reconciliation of a resource's Child if OperationAnnotation is set to this.
	OperationAnnotationIgnoreChild = "ignore-child-reconciliation"
	// OperationAnnotationIgnoreChildAndSelf skips the reconciliation of a resource's Child ans self if OperationAnnotation is set to this.
	OperationAnnotationIgnoreChildAndSelf = "ignore-child-and-self-reconciliation"
	// OperationAnnotationRetryChild restarts the reconciliation of a resource's Child if OperationAnnotation is set to this, from failed state -> initial state.
	OperationAnnotationRetryChild = "retry-child-reconciliation"
	// OperationAnnotationRetryChildAndSelf restarts the reconciliation of a resource's Child ans self if OperationAnnotation is set to this, from failed state -> initial state..
	OperationAnnotationRetryChildAndSelf = "retry-child-and-self-reconciliation"
	// AnnotationInstanceType is used to specify the type of Server.
	AnnotationInstanceType = "metal.ironcore.dev/instance-type"
	// OperationAnnotationForceUpdateOrDeleteInProgress allows update/Delete of a resource even if it is in progress.
	OperationAnnotationForceUpdateOrDeleteInProgress = "allow-in-progress-delete"
	// OperationAnnotationForceUpdateInProgress allows update of a resource even if it is in progress.
	OperationAnnotationForceUpdateInProgress = "allow-in-progress-update"
	// OperationAnnotationRetryFailed restarts the reconciliation of a resource from failed state -> initial state.
	OperationAnnotationRetryFailed = "retry-failed-state-resource"
	// OperationAnnotationRetryFailedPropagated restarts the reconciliation of a resource's child from failed state -> initial state.
	OperationAnnotationRetryFailedPropagated = "retry-failed-state-resource-propagated"
)

const (
	// GracefulRestartServerPower indicates to gracefully restart the baremetal server power.
	GracefulRestartServerPower = "graceful-restart-server"
	// HardRestartServerPower indicates to hard restart the baremetal server power.
	HardRestartServerPower = "hard-restart-server"
	// PowerCycleServerPower indicates to power cycle the baremetal server.
	PowerCycleServerPower = "power-cycle-server"
	// ForceOffServerPower indicates to force powerOff the baremetal server power.
	ForceOffServerPower = "force-off-server"
	// ForceOnServerPower indicates to force powerOn the baremetal server power.
	ForceOnServerPower = "force-on-server"

	// GracefulRestartBMC indicates to gracefully restart the baremetal server's BMC's power.
	GracefulRestartBMC = "graceful-restart-bmc"
)

var AnnotationToRedfishMapping = map[string]redfish.ResetType{
	GracefulRestartServerPower: redfish.GracefulRestartResetType,
	HardRestartServerPower:     redfish.ForceRestartResetType,
	PowerCycleServerPower:      redfish.PowerCycleResetType,
	ForceOffServerPower:        redfish.ForceOffResetType,
	ForceOnServerPower:         redfish.ForceOnResetType,
	GracefulRestartBMC:         redfish.GracefulRestartResetType,
}
