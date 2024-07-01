// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package macdb

type MacPrefixes struct {
	MacPrefixes []MacPrefix `json:"macPrefixes"`
}

type Console struct {
	Type string `json:"type"`
	Port int32  `json:"port"`
}

type MacPrefix struct {
	MacPrefix          string       `json:"macPrefix"`
	Manufacturer       string       `json:"manufacturer"`
	Protocol           string       `json:"protocol"`
	Port               int32        `json:"port"`
	Type               string       `json:"type"`
	DefaultCredentials []Credential `json:"defaultCredentials"`
	Console            Console      `json:"console,omitempty"`
}

type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
