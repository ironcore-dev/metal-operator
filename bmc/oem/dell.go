// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

type Dell struct {
	Service *gofish.Service
}

func (r *Dell) GetUpdateRequestBody(
	parameters *redfish.SimpleUpdateParameters,
) *SimpleUpdateRequestBody {
	RequestBody := &SimpleUpdateRequestBody{}
	RequestBody.RedfishOperationApplyTime = redfish.ImmediateOperationApplyTime
	RequestBody.ForceUpdate = parameters.ForceUpdate
	RequestBody.ImageURI = parameters.ImageURI
	RequestBody.Passord = parameters.Passord
	RequestBody.Username = parameters.Username
	RequestBody.Targets = parameters.Targets
	RequestBody.TransferProtocol = parameters.TransferProtocol

	return RequestBody
}

func (r *Dell) GetUpdateTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body %v %v", err, rawBody)
	}

	if taskMonitor, ok := response.Header["Location"]; ok && len(rawBody) == 0 {
		return taskMonitor[0], nil
	}

	return "", fmt.Errorf("unexpected response body %v %v", err, rawBody)
}

func (r *Dell) GetTaskMonitorDetails(ctx context.Context, taskMonitorResponse *http.Response) (*redfish.Task, error) {
	task := &redfish.Task{}
	respTaskRawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respTaskRawBody, &task)
	if err != nil {
		return nil, err
	}

	return task, nil
}

type DellIdracManager struct {
	BMC *redfish.Manager
}

type DellAttributes struct {
	Id         string
	Attributes redfish.SettingsAttributes
	Settings   common.Settings `json:"@Redfish.Settings"`
	Etag       string
}

type DellManagerLinksOEM struct {
	DellLinkAttributes  common.Links `json:"DellAttributes"`
	DellAttributesCount int          `json:"DellAttributes@odata.count"`
}

func (d *DellIdracManager) GetObjFromUri(
	uri string,
	respObj any,
) ([]string, error) {
	resp, err := d.BMC.GetClient().Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(rawBody, &respObj)
	if err != nil {
		return nil, err
	}
	return resp.Header["Etag"], nil
}

func (d *DellIdracManager) GetOEMBMCSettingAttribute() ([]DellAttributes, error) {

	type temp struct {
		DellOEMData DellManagerLinksOEM `json:"Dell"`
	}

	tempData := &temp{}
	err := json.Unmarshal(d.BMC.OemLinks, tempData)
	if err != nil {
		return nil, err
	}

	BMCDellAttributes := []DellAttributes{}
	err = nil
	for _, data := range tempData.DellOEMData.DellLinkAttributes {
		BMCDellAttribute := &DellAttributes{}
		eTag, errAttr := d.GetObjFromUri(data.String(), BMCDellAttribute)
		if errAttr != nil {
			err = errors.Join(err, errAttr)
		}
		if eTag != nil {
			BMCDellAttribute.Etag = eTag[0]
		}
		BMCDellAttributes = append(BMCDellAttributes, *BMCDellAttribute)
	}
	if err != nil {
		return BMCDellAttributes, err
	}

	return BMCDellAttributes, nil
}

func (d *DellIdracManager) UpdateBMCAttributesApplyAt(
	attrs redfish.SettingsAttributes,
	applyTime common.ApplyTime,
) error {

	BMCattributeValues, err := d.GetOEMBMCSettingAttribute()
	if err != nil {
		return err
	}

	payloads := make(map[string]redfish.SettingsAttributes, len(BMCattributeValues))
	for key, value := range attrs {
		for _, eachAttr := range BMCattributeValues {
			if _, ok := eachAttr.Attributes[key]; ok {
				if data, ok := payloads[eachAttr.Settings.SettingsObject.String()]; ok {
					data[key] = value
				} else {
					payloads[eachAttr.Settings.SettingsObject.String()] = make(redfish.SettingsAttributes)
					payloads[eachAttr.Settings.SettingsObject.String()][key] = value
				}
				// keys cant be duplicate. Hence, break once its already found in one of idrac settings sub types
				break
			}
		}
	}

	// If there are any allowed updates, try to send updates to the system and
	// return the result.
	if len(payloads) > 0 {
		var errs []error
		// for wach sub type, apply the settings
		for settingPath, payload := range payloads {
			// fetch the etag required for settingPath
			etag, err := func(uri string) ([]string, error) {
				resp, err := d.BMC.GetClient().Get(uri)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close() // nolint: errcheck
				return resp.Header["Etag"], nil
			}(settingPath)

			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get Etag for %v. error %v", settingPath, err))
				continue
			}

			data := map[string]interface{}{"Attributes": payload}
			if applyTime != "" {
				data["@Redfish.SettingsApplyTime"] = map[string]string{"ApplyTime": string(applyTime)}
			}
			var header = make(map[string]string)
			if etag != nil {
				header["If-Match"] = etag[0]
			}

			err = func(uri string, data map[string]any, header map[string]string) error {
				resp, err := d.BMC.GetClient().PatchWithHeaders(uri, data, header)
				if err != nil {
					return err
				}
				defer resp.Body.Close() // nolint: errcheck
				return nil
			}(settingPath, data, header)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to patch settings at %v. error %v", settingPath, err))
				continue
			}
		}

		if len(errs) > 0 {
			return fmt.Errorf("some settings failed to apply %v", errs)
		}
	}
	return nil
}

// "Dell": {
// 	"@odata.type": "#DellOem.v1_3_0.DellOemLinks",
// 	"DellAttributes": [
// 	  {
// 		"@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellAttributes/iDRAC.Embedded.1"
// 	  },
// 	  {
// 		"@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellAttributes/System.Embedded.1"
// 	  },
// 	  {
// 		"@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellAttributes/LifecycleController.Embedded.1"
// 	  }
// 	],
// 	"DellAttributes@odata.count": 3,
// 	"DellJobService": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellJobService"
// 	},
// 	"DellLCService": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellLCService"
// 	},
// 	"DellLicensableDeviceCollection": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellLicensableDevices"
// 	},
// 	"DellLicenseCollection": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellLicenses"
// 	},
// 	"DellLicenseManagementService": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellLicenseManagementService"
// 	},
// 	"DellOpaqueManagementDataCollection": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellOpaqueManagementData"
// 	},
// 	"DellPersistentStorageService": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellPersistentStorageService"
// 	},
// 	"DellSwitchConnectionCollection": {
// 	  "@odata.id": "/redfish/v1/Systems/System.Embedded.1/NetworkPorts/Oem/Dell/DellSwitchConnections"
// 	},
// 	"DellSwitchConnectionService": {
// 	  "@odata.id": "/redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellSwitchConnectionService"
// 	},
// 	"DellSystemManagementService": {
// 	  "@odata.id": "/redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellSystemManagementService"
// 	},
// 	"DellSystemQuickSyncCollection": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellSystemQuickSync"
// 	},
// 	"DellTimeService": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellTimeService"
// 	},
// 	"DellUSBDeviceCollection": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellUSBDevices"
// 	},
// 	"DelliDRACCardService": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DelliDRACCardService"
// 	},
// 	"DellvFlashCollection": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/DellvFlash"
// 	},
// 	"Jobs": {
// 	  "@odata.id": "/redfish/v1/Managers/iDRAC.Embedded.1/Oem/Dell/Jobs"
// 	}
//   }
// },
