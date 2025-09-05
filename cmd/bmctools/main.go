package main

import (
	"fmt"
	"os"

	"github.com/ironcore-dev/metal-operator/cmd/bmc/cmds"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

func main() {
	if err := cmds.NewCommand().ExecuteContext(signals.SetupSignalHandler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
