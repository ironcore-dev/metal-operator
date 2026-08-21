// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

// Shared condition types used across multiple controllers.
const (
	// ConditionPoweringOn indicates a server is powering on.
	ConditionPoweringOn = "PoweringOn"
	// ConditionReset indicates a reset condition.
	ConditionReset = "Reset"
	// ConditionReady indicates readiness.
	ConditionReady = "Ready"
	// ConditionWaitingForPowerOff is set while the server is waiting for the BMC
	// to confirm it is powered off before the claim can be released.
	ConditionWaitingForPowerOff = "WaitingForPowerOff"
)

// Shared reason strings used across multiple controllers.
const (
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
	// ReasonWaitingForPowerOff is used while waiting for the BMC to confirm the host is powered off.
	ReasonWaitingForPowerOff = "WaitingForPowerOff"
	// ReasonPowerOffConfirmed marks WaitingForPowerOff as resolved.
	ReasonPowerOffConfirmed = "PowerOffConfirmed"
	// ReasonResetComplete indicates a BMC reset has completed successfully.
	ReasonResetComplete = "ResetComplete"
)
