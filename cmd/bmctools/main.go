// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/ironcore-dev/metal-operator/cmd/bmctools/cmds"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

func main() {
	if err := cmds.NewCommand().ExecuteContext(signals.SetupSignalHandler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
