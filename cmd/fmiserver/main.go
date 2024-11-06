package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/fmi"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = slog.Default().WithGroup("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(metalv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		port                   int
		shutdownTimeoutSeconds int
		insecureBMC            bool
		taskRunnerType         string
	)

	flag.IntVar(&port, "port", 9080, "The port to listen on")
	flag.IntVar(&shutdownTimeoutSeconds, "shutdown-timeout-seconds", 10, "The timeout to wait for the server to shutdown")
	flag.BoolVar(&insecureBMC, "insecure-bmc", true, "Whether to use insecure connection to BMC")
	flag.StringVar(&taskRunnerType, "task-runner-type", "default", "The type of task runner to use")
	flag.Parse()

	cfg := ctrl.GetConfigOrDie()
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error("unable to create client", slog.Any("err", err))
		os.Exit(1)
	}

	setupLog.Info("starting fmi server")
	config := fmi.ServerConfig{
		Port:            port,
		ShutdownTimeout: time.Second * time.Duration(shutdownTimeoutSeconds),
		KubeClient:      c,
		TaskRunnerType:  taskRunnerType,
	}
	server, err := fmi.NewServerGRPC(config, insecureBMC)
	if err != nil {
		setupLog.Error("failed to create server", slog.Any("err", err))
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		setupLog.Error("failed to start server", slog.Any("err", err))
		os.Exit(1)
	}
}
