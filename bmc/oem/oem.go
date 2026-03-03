// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"context"
	"net/http"
	"reflect"

	"github.com/stmcginnis/gofish/schemas"
)

// ManagerInterface defines methods for OEM-specific Server Manager's operations in BMC.
type ManagerInterface interface {
	// GetOEMBMCSettingAttribute retrieves OEM-specific BMC setting attributes.
	GetOEMBMCSettingAttribute(ctx context.Context, attributes map[string]string) (schemas.SettingsAttributes, error)

	// GetBMCPendingAttributeValues retrieves pending BMC attribute values.
	GetBMCPendingAttributeValues(ctx context.Context) (schemas.SettingsAttributes, error)

	// CheckBMCAttributes checks if the BMC attributes are valid and returns whether a reset is required.
	CheckBMCAttributes(ctx context.Context, attributes schemas.SettingsAttributes) (bool, error)

	// GetObjFromUri retrieves an object from a given URI and populates the response object.
	GetObjFromUri(ctx context.Context, uri string, respObj any) (string, error)

	// UpdateBMCAttributesApplyAt updates BMC attributes and applies them at the specified time.
	UpdateBMCAttributesApplyAt(ctx context.Context, attrs schemas.SettingsAttributes, applyTime schemas.SettingsApplyTime) error
}

// OEMInterface defines methods for OEM-specific Server operations in BMC.
type OEMInterface interface {
	GetUpdateRequestBody(parameters *schemas.UpdateServiceSimpleUpdateParameters) *SimpleUpdateRequestBody
	GetUpdateTaskMonitorURI(response *http.Response) (string, error)
	GetTaskMonitorDetails(ctx context.Context, taskMonitorResponse *http.Response) (*schemas.Task, error)
	MountVirtualMedia(ctx context.Context, systemURI string, mediaURL string, slotID string) error
	EjectVirtualMedia(ctx context.Context, systemURI string, slotID string) error
	GetVirtualMediaStatus(ctx context.Context, systemURI string) ([]*schemas.VirtualMedia, error)
}

func IsSubMap(main, sub map[string]any) bool {
	for k, vSub := range sub {
		vMain, ok := main[k]
		if !ok {
			return false
		}
		switch vSubTyped := vSub.(type) {
		case map[string]any:
			vMainTyped, ok := vMain.(map[string]any)
			if !ok || !IsSubMap(vMainTyped, vSubTyped) {
				return false
			}
		default:
			if !reflect.DeepEqual(vMain, vSub) {
				return false
			}
		}
	}
	return true
}
