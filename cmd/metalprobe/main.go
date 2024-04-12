// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"os"

	"github.com/afritzler/metal-operator/internal/probe"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	var registryURL string
	var serverUUID string

	flag.StringVar(&registryURL, "registry-url", "", "Registry URL where the probe will register itself.")
	flag.StringVar(&serverUUID, "server-uuid", "", "Agent UUID to register with the registry.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if serverUUID == "" {
		setupLog.Error(nil, "server uuid is missing")
		os.Exit(1)
	}

	if registryURL == "" {
		setupLog.Error(nil, "registry URL is missing")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	setupLog.Info("starting registry agent")
	agent := probe.NewAgent(serverUUID, registryURL)
	if err := agent.Start(ctx); err != nil {
		setupLog.Error(err, "problem running probe agent")
		os.Exit(1)
	}
}
