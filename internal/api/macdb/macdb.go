// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package macdb

import metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

// MacPrefixes is a list of MacPrefix
type MacPrefixes struct {
	// MacPrefixes is a list of MacPrefix
	MacPrefixes []MacPrefix `json:"macPrefixes"`
}

// Console is a struct that contains the type and port of the console
type Console struct {
	// Type is the type of the console
	Type string `json:"type"`
	// Port is the port of the console
	Port int32 `json:"port"`
}

// MacPrefix is a struct that contains the mac prefix, manufacturer, protocol, protocol scheme, port, type,
// default credentials and console
type MacPrefix struct {
	// MacPrefix is the mac prefix
	MacPrefix string `json:"macPrefix"`
	// Manufacturer is the manufacturer
	Manufacturer string `json:"manufacturer"`
	// Protocol is the protocol
	Protocol string `json:"protocol"`
	// ProtocolScheme is the protocol scheme (http, https)
	ProtocolScheme metalv1alpha1.ProtocolScheme `json:"protocolScheme,omitempty"`
	// Port is the port
	Port int32 `json:"port"`
	// Type is the type
	Type string `json:"type"`
	// DefaultCredentials is the default credentials
	DefaultCredentials []Credential `json:"defaultCredentials"`
	// Console is the console
	Console Console `json:"console,omitempty"`
}

// Credential is a struct that contains the username and password
type Credential struct {
	// Username is the username
	Username string `json:"username"`
	// Password is the password
	Password string `json:"password"`
}
