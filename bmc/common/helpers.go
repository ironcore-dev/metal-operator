// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"errors"
	"fmt"

	"github.com/stmcginnis/gofish/schemas"
)

type InvalidBMCSettingsError struct {
	SettingName  string
	SettingValue any
	Message      string
}

func (e *InvalidBMCSettingsError) Error() string {
	return fmt.Sprintf("invalid BMC setting %s=%v: %s", e.SettingName, e.SettingValue, e.Message)
}

func CheckAttribues(
	attrs schemas.SettingsAttributes,
	filtered map[string]schemas.Attributes,
) (reset bool, err error) {
	reset = false
	var errs []error
	// TODO: add more types like maps and Enumerations
	for name, value := range attrs {
		entryAttribute, ok := filtered[name]
		if !ok {
			err := &InvalidBMCSettingsError{
				SettingName:  name,
				SettingValue: value,
				Message:      "attribute not found or is immutable/hidden",
			}
			errs = append(errs, err)
			continue
		}
		if entryAttribute.ResetRequired {
			reset = true
		}
		switch entryAttribute.Type {
		case schemas.IntegerAttributeType:
			if _, ok := value.(int); !ok {
				err := &InvalidBMCSettingsError{
					SettingName:  name,
					SettingValue: value,
					Message: fmt.Sprintf("attribute value has wrong type. needed '%s'",
						entryAttribute.Type,
					),
				}
				errs = append(errs, err)
			}
		case schemas.StringAttributeType:
			if _, ok := value.(string); !ok {
				err := &InvalidBMCSettingsError{
					SettingName:  name,
					SettingValue: value,
					Message: fmt.Sprintf("attribute value has wrong type. needed '%s'",
						entryAttribute.Type,
					),
				}
				errs = append(errs, err)
			}
		case schemas.EnumerationAttributeType:
			if _, ok := value.(string); !ok {
				err := &InvalidBMCSettingsError{
					SettingName:  name,
					SettingValue: value,
					Message: fmt.Sprintf("attribute value has wrong type. needed '%s'",
						entryAttribute.Type,
					),
				}
				errs = append(errs, err)
				break
			}
			var validEnum bool
			for _, attrValue := range entryAttribute.Value {
				if attrValue.ValueName == value.(string) {
					validEnum = true
					break
				}
			}
			if !validEnum {
				err := &InvalidBMCSettingsError{
					SettingName:  name,
					SettingValue: value,
					Message:      fmt.Sprintf("attributes value is unknown. Valid Attributes %v", entryAttribute.Value),
				}
				errs = append(errs, err)
			}
		default:
			err := &InvalidBMCSettingsError{
				SettingName:  name,
				SettingValue: value,
				Message: fmt.Sprintf("attribute value has wrong type. needed '%s'",
					entryAttribute.Type,
				),
			}
			errs = append(errs, err)
		}
	}
	return reset, errors.Join(errs...)
}
