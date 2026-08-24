// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"

	"github.com/stmcginnis/gofish/schemas"
)

// Establishing the general rule for constants naming for Annotations:
//   - "Key" for Annotation constants should be named OperationAnnotation.
//     We do not want to handle multiple annotation keys for operations performed
//     outside the spec-defined flow.
//   - "Value" for Annotation constants should be named as OperationAnnotation<ActionType>.
//
// Values follow the grammar: <verb>[-child]
// where <verb> is one of: ignore | retry
// e.g.
//
//	OperationAnnotationIgnore  -> "ignore"
//	OperationAnnotationRetry   -> "retry"

const (
	// OperationAnnotation is the annotation key selecting an out-of-band operation to perform on
	// a resource, i.e. an action driven by the annotation value rather than by spec reconciliation.
	// Its value must be one of the OperationAnnotation* constants below.
	OperationAnnotation = "metal.ironcore.dev/operation"

	// OperationAnnotationIgnore makes the reconciler skip a resource entirely when set on it.
	OperationAnnotationIgnore = "ignore"

	// OperationAnnotationForceUpdateOrDeleteInProgress allows a resource to be deleted even while an
	// operation is still in progress.
	OperationAnnotationForceUpdateOrDeleteInProgress = "allow-in-progress-delete"
	// OperationAnnotationForceUpdateInProgress allows a resource to be updated even while an operation
	// is still in progress.
	OperationAnnotationForceUpdateInProgress = "allow-in-progress-update"

	// OperationAnnotationRetry restarts a resource's reconciliation from failed state back to
	// initial state when set on it.
	OperationAnnotationRetry = "retry"

	// OperationAnnotationRetryPropagated is set by the controller on a child resource to restart
	// the child's reconciliation from failed state back to initial state, after the parent requested
	// retrying its children via retry-child or retry-child-and-self. It is controller-set state, not
	// user-facing input.
	OperationAnnotationRetryPropagated = "retry-propagated"

	// OperationAnnotationRotateCredentials indicates that a resource's credentials should be rotated
	// when set on it.
	OperationAnnotationRotateCredentials = "rotate-credentials"

	// OperationAnnotationPark requests that a Server be parked out of the ServerClaim lifecycle so an
	// external component can run out-of-band day-2 operations (firmware/BIOS/BMC updates, diagnostics,
	// hardware rework).
	OperationAnnotationPark = "park"

	// OperationAnnotationUnpark requests that a parked Server be resumed into the normal
	// ServerClaim lifecycle.
	OperationAnnotationUnpark = "unpark"

	// ParkedAnnotation is the durable, controller-set marker recording that a Server is parked.
	ParkedAnnotation = "metal.ironcore.dev/parked"

	// PreParkStateAnnotation records the Server state before it was parked, so unpark can
	// resume the Server into its previous state.
	PreParkStateAnnotation = "metal.ironcore.dev/pre-park-state"

	// MetadataKeyPrefix is the shared prefix for labels and annotations on a Server
	// whose suffix is exposed via the metaldata service to the booted server.
	MetadataKeyPrefix = "metadata.metal.ironcore.dev/"

	// MetadataFlavorHeader is the HTTP header the metaldata service requires on
	// every request, mirroring GCE/GCP metadata server conventions.
	MetadataFlavorHeader = "Metadata-Flavor"

	// MetadataFlavorValue is the expected value for MetadataFlavorHeader.
	MetadataFlavorValue = "IronCore Metal"
)

// SecretTypeUserData is the Secret type required for Secrets referenced by
// ServerClaim.Spec.UserDataRef.
const SecretTypeUserData v1.SecretType = "metal.ironcore.dev/user-data"

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

	// ForceSSHResetBMC is set internally by the BMCReconciler on the OperationAnnotation when a
	// Redfish reset fails with a 5xx error. On the next reconcile the controller detects this
	// annotation and performs the SSH reset directly.
	ForceSSHResetBMC = "force-ssh-reset-bmc"
)

var AnnotationToRedfishMapping = map[string]schemas.ResetType{
	GracefulRestartServerPower: schemas.GracefulRestartResetType,
	HardRestartServerPower:     schemas.ForceRestartResetType,
	PowerCycleServerPower:      schemas.PowerCycleResetType,
	ForceOffServerPower:        schemas.ForceOffResetType,
	ForceOnServerPower:         schemas.ForceOnResetType,
	GracefulRestartBMC:         schemas.GracefulRestartResetType,
}
