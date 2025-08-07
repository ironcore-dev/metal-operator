// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

// OSBootedCondition is the condition type for indicating a server has booted.
const OSBootedCondition = "OSBooted"

// RegistrationPayload represents the payload to send to the `/register` endpoint,
// including the systemUUID and the server details.
type RegistrationPayload struct {
	SystemUUID string `json:"systemUUID"`
	Data       Server `json:"data"`
}

// BootstatePayload represents the payload to send to the `/bootstate` endpoint,
// including the systemUUID and the booted state.
type BootstatePayload struct {
	SystemUUID string `json:"systemUUID"`
	Booted     bool   `json:"booted"`
}
