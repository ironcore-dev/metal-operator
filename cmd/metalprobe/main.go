/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
