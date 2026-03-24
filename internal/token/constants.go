// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package token

// DiscoveryTokenSigningSecretName is the name of the Kubernetes Secret
// containing the signing key for discovery tokens
const DiscoveryTokenSigningSecretName = "discovery-token-signing-secret"

// DiscoveryTokenSigningSecretKey is the data key within the Secret
const DiscoveryTokenSigningSecretKey = "signing-key"
