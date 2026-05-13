// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package probe

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
)

func cleanDisks(_ context.Context, _ logr.Logger, _ string) error {
	return fmt.Errorf("disk cleaning is only supported on Linux")
}
