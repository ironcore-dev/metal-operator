// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package move

import "sigs.k8s.io/controller-runtime/pkg/client"

type Clients struct {
	Source client.Client
	Target client.Client
}
