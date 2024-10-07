package main

import (
	"flag"
	"os"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/job"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(metalv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		jobTypeString string
		serverBIOSRef string
		insecure      bool
	)

	pflag.StringVar(&jobTypeString, "job-type", "", "job type")
	pflag.StringVar(&serverBIOSRef, "server-bios-ref", "", "server bios ref")
	pflag.BoolVar(&insecure, "insecure", true, "use insecure connection to BMC")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if jobTypeString == "" {
		setupLog.Error(nil, "job type is required")
		os.Exit(1)
	}

	if serverBIOSRef == "" {
		setupLog.Error(nil, "server bios ref is required")
		os.Exit(1)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		setupLog.Error(err, "unable to get in cluster config")
		os.Exit(1)
	}
	kubeClient, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		setupLog.Error(err, "unable to create kubernetes client")
		os.Exit(1)
	}

	executor := job.New(kubeClient)
	ctx := ctrl.SetupSignalHandler()
	if err = executor.Run(ctx, jobTypeString, serverBIOSRef); err != nil {
		os.Exit(1)
	}
}
