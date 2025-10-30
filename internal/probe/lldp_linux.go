// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"k8s.io/apimachinery/pkg/util/wait"
)

func collectLLDPInfo(ctx context.Context, interval, duration time.Duration) (registry.LLDP, error) {
	lldp := registry.LLDP{}
	var out bytes.Buffer
	tool, err := exec.LookPath("networkctl")
	if err != nil {
		//TODO: exit or just skip?
		return lldp, fmt.Errorf("networkctl is not present: %w", err)
	}

	if err := wait.PollUntilContextTimeout(ctx, interval, duration, true, func(ctx context.Context) (done bool, err error) {
		cmd := exec.Command(tool, "lldp", "--json=short")
		cmd.Stdout = &out
		err = cmd.Run()
		if err != nil {
			return false, fmt.Errorf("running networkctl encountered a problem: %w", err)
		}
		if len(out.Bytes()) == 0 {
			return false, nil
		}
		if err := json.Unmarshal(out.Bytes(), &lldp); err != nil {
			var syntaxErr *json.SyntaxError
			if errors.As(err, &syntaxErr) {
				// @afritzler: ignoring for now as networkctl lldp --json doesn't work on systemd versions < 257
				return true, nil
			}
			return false, fmt.Errorf("can't unmarshal networkctl output: %w", err)
		}
		return true, nil
	}); err != nil {
		return registry.LLDP{}, err
	}

	return lldp, nil
}
