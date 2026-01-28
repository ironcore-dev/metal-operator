// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"context"
	"net/http"
	"reflect"

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

// ManagerInterface defines methods for OEM-specific Server Manager's operations in BMC.
type ManagerInterface interface {
	// GetOEMBMCSettingAttribute retrieves OEM-specific BMC setting attributes.
	GetOEMBMCSettingAttribute(ctx context.Context, attributes map[string]string) (redfish.SettingsAttributes, error)

	// GetBMCPendingAttributeValues retrieves pending BMC attribute values.
	GetBMCPendingAttributeValues(ctx context.Context) (redfish.SettingsAttributes, error)

	// CheckBMCAttributes checks if the BMC attributes are valid and returns whether a reset is required.
	CheckBMCAttributes(ctx context.Context, attributes redfish.SettingsAttributes) (bool, error)

	// GetObjFromUri retrieves an object from a given URI and populates the response object.
	GetObjFromUri(ctx context.Context, uri string, respObj any) ([]string, error)

	// UpdateBMCAttributesApplyAt updates BMC attributes and applies them at the specified time.
	UpdateBMCAttributesApplyAt(ctx context.Context, attrs redfish.SettingsAttributes, applyTime common.ApplyTime) error
}

// OEMInterface defines methods for OEM-specific Server operations in BMC.
type OEMInterface interface {
	GetUpdateRequestBody(parameters *redfish.SimpleUpdateParameters) *SimpleUpdateRequestBody
	GetUpdateTaskMonitorURI(response *http.Response) (string, error)
	GetTaskMonitorDetails(ctx context.Context, taskMonitorResponse *http.Response) (*redfish.Task, error)
	MountVirtualMedia(ctx context.Context, systemURI string, mediaURL string, slotID string) error
	EjectVirtualMedia(ctx context.Context, systemURI string, slotID string) error
	GetVirtualMediaStatus(ctx context.Context, systemURI string) ([]*redfish.VirtualMedia, error)
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
