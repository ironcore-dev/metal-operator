// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package probe

import (
	"context"
	"fmt"
)

func cleanDisks(_ context.Context, _ DiskCleaningMode) error {
	return fmt.Errorf("disk cleaning is only supported on Linux")
}
