// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"errors"
	"fmt"

	"github.com/stmcginnis/gofish/redfish"
)

// todo: merge with checkBiosAttribues after #298
func CheckAttribues(
	attrs redfish.SettingsAttributes,
	filtered map[string]redfish.Attribute,
) (reset bool, err error) {
	reset = false
	var errs []error
	//TODO: add more types like maps and Enumerations
	for name, value := range attrs {
		entryAttribute, ok := filtered[name]
		if !ok {
			errs = append(errs, fmt.Errorf("attribute %s not found or immutable/hidden", name))
			continue
		}
		if entryAttribute.ResetRequired {
			reset = true
		}
		switch entryAttribute.Type {
		case redfish.IntegerAttributeType:
			if _, ok := value.(int); !ok {
				errs = append(
					errs,
					fmt.Errorf(
						"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name,
						value,
						entryAttribute.Type,
						entryAttribute,
					))
			}
		case redfish.StringAttributeType:
			if _, ok := value.(string); !ok {
				errs = append(
					errs,
					fmt.Errorf(
						"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name,
						value,
						entryAttribute.Type,
						entryAttribute,
					))
			}
		case redfish.EnumerationAttributeType:
			if _, ok := value.(string); !ok {
				errs = append(
					errs,
					fmt.Errorf(
						"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name,
						value,
						entryAttribute.Type,
						entryAttribute,
					))
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
				errs = append(errs, fmt.Errorf("attribute %s value is unknown. needed %v", name, entryAttribute.Value))
			}
		default:
			errs = append(
				errs,
				fmt.Errorf(
					"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
					name,
					value,
					entryAttribute.Type,
					entryAttribute,
				))
		}
	}
	return reset, errors.Join(errs...)
}
