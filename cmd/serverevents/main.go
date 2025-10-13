package main

import (
	"flag"
	"os"
	"time"

	"github.com/ironcore-dev/metal-operator/internal/serverevents"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	var registryURL string
	var serverUUID string
	var duration time.Duration

	flag.StringVar(&registryURL, "registry-url", "", "Registry URL where the probe will register itself.")
	flag.StringVar(&serverUUID, "server-uuid", "", "Agent UUID to register with the registry.")
	flag.DurationVar(&duration, "duration", 5*time.Second, "Duration of time to wait between checks.")

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

	setupLog.Info("starting serverevent agent")
	server := serverevents.NewServer(setupLog, ":8888")

	if err := server.Start(ctx); err != nil {
		setupLog.Error(err, "problem running telemetry server")
		os.Exit(1)
	}
}
