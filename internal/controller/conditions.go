// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

// Shared condition types used across multiple controllers.
const (
	// ConditionServerMaintenanceCreated indicates a ServerMaintenance resource has been created.
	ConditionServerMaintenanceCreated = "ServerMaintenanceCreated"
	// ConditionServerMaintenanceDeleted indicates a ServerMaintenance resource has been deleted.
	ConditionServerMaintenanceDeleted = "ServerMaintenanceDeleted"
	// ConditionServerMaintenanceWaiting indicates waiting for ServerMaintenance approval.
	ConditionServerMaintenanceWaiting = "ServerMaintenanceWaiting"
	// ConditionResetIssued indicates a reset has been issued.
	ConditionResetIssued = "ResetIssued"
	// ConditionVersionUpgradeIssued indicates a version upgrade has been issued.
	ConditionVersionUpgradeIssued = "VersionUpgradeIssued"
	// ConditionVersionUpgradeCompleted indicates a version upgrade has completed.
	ConditionVersionUpgradeCompleted = "VersionUpgradeCompleted"
	// ConditionVersionUpgradeVerification indicates version upgrade verification is in progress.
	ConditionVersionUpgradeVerification = "VersionUpgradeVerification"
	// ConditionVersionUpgradeReboot indicates a reboot during version upgrade.
	ConditionVersionUpgradeReboot = "VersionUpgradeReboot"
	// ConditionVersionUpdatePending indicates a version update is pending.
	ConditionVersionUpdatePending = "VersionUpdatePending"
	// ConditionPoweringOn indicates a server is powering on.
	ConditionPoweringOn = "PoweringOn"
	// ConditionReset indicates a reset condition.
	ConditionReset = "Reset"
	// ConditionReady indicates readiness.
	ConditionReady = "Ready"
	// ConditionRetryOfFailedResourceIssued indicates a retry of a failed resource has been issued.
	ConditionRetryOfFailedResourceIssued = "RetryOfFailedResourceIssued"
)

// Shared reason strings used across multiple controllers.
const (
	// ReasonUpgradeIssued indicates an upgrade has been issued.
	ReasonUpgradeIssued = "UpgradeIssued"
	// ReasonUpgradeTaskFailed indicates an upgrade task has failed.
	ReasonUpgradeTaskFailed = "UpgradeTaskFailed"
	// ReasonUpgradeIssueFailed indicates an upgrade failed to be issued.
	ReasonUpgradeIssueFailed = "UpgradeIssueFailed"
	// ReasonUpgradeTaskCompleted indicates an upgrade task has completed.
	ReasonUpgradeTaskCompleted = "UpgradeTaskCompleted"
	// ReasonVersionUpdateVerified indicates a version update has been verified.
	ReasonVersionUpdateVerified = "VersionUpdateVerified"
	// ReasonVersionVerificationFailed indicates version verification has failed.
	ReasonVersionVerificationFailed = "VersionVerificationFailed"
	// ReasonVersionUpgradePending indicates a version upgrade is pending.
	ReasonVersionUpgradePending = "VersionUpgradePending"
	// ReasonResetIssued indicates a reset has been issued.
	ReasonResetIssued = "ResetIssued"
	// ReasonAuthenticationFailed indicates authentication has failed.
	ReasonAuthenticationFailed = "AuthenticationFailed"
	// ReasonInternalError indicates an internal server error.
	ReasonInternalError = "InternalServerError"
	// ReasonUnknownError indicates an unknown error.
	ReasonUnknownError = "UnknownError"
	// ReasonConnectionFailed indicates a connection failure.
	ReasonConnectionFailed = "ConnectionFailed"
	// ReasonUserReset indicates a user-requested reset.
	ReasonUserReset = "UserRequested"
	// ReasonAutoReset indicates an automatic reset.
	ReasonAutoReset = "AutoResetting"
	// ReasonConnected indicates a successful connection.
	ReasonConnected = "Connected"
	// ReasonMaintenanceCreated indicates a ServerMaintenance resource has been created.
	ReasonMaintenanceCreated = "ServerMaintenanceHasBeenCreated"
	// ReasonMaintenanceDeleted indicates a ServerMaintenance resource has been deleted.
	ReasonMaintenanceDeleted = "ServerMaintenanceHasBeenDeleted"
	// ReasonMaintenanceWaiting indicates waiting for ServerMaintenance approval.
	ReasonMaintenanceWaiting = "ServerMaintenanceWaitingOnApproval"
	// ReasonMaintenanceApproved indicates ServerMaintenance has been approved.
	ReasonMaintenanceApproved = "ServerMaintenanceApproval"
	// ReasonRetryOfFailedResourceIssued indicates a retry of a failed resource has been issued.
	ReasonRetryOfFailedResourceIssued = "RetryOfFailedResourceIssued"
)
