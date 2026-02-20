// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import "github.com/stmcginnis/gofish/schemas"

type SimpleUpdateRequestBody struct {
	schemas.UpdateServiceSimpleUpdateParameters
	RedfishOperationApplyTime schemas.OperationApplyTime `json:"@Redfish.OperationApplyTime,omitempty"`
}
