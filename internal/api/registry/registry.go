// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

// RegistrationPayload represents the payload to send to the `/register` endpoint,
// including the systemUUID and the server details.
type RegistrationPayload struct {
	SystemUUID string `json:"systemUUID"`
	Data       Server `json:"data"`
}
