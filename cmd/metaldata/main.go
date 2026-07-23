// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/metaldata"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(metalv1alpha1.AddToScheme(scheme))
}

// stringMapFlag is a flag.Value that accumulates repeated key=value pairs.
type stringMapFlag map[string]string

func (m *stringMapFlag) String() string {
	if m == nil || *m == nil {
		return ""
	}
	keys := make([]string, 0, len(*m))
	for k := range *m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+(*m)[k])
	}
	return strings.Join(pairs, ",")
}

func (m *stringMapFlag) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok {
		return fmt.Errorf("expected key=value, got %q", s)
	}
	if *m == nil {
		*m = make(stringMapFlag)
	}
	(*m)[k] = v
	return nil
}

func main() {
	os.Exit(Main())
}

func Main() int {
	var (
		listenPort         int
		metricsBindAddress string
		healthBindAddress  string
		staticMetadata     stringMapFlag
	)

	flag.IntVar(&listenPort, "metaldata-port", 10002, "The port the metadata HTTP server listens on.")
	flag.StringVar(&metricsBindAddress, "metrics-bind-address", ":8080",
		"The address the metrics endpoint binds to. Use \"0\" to disable.")
	flag.StringVar(&healthBindAddress, "health-probe-bind-address", ":8081",
		"The address the health probe endpoint binds to. Use \"0\" to disable.")
	flag.Var(&staticMetadata, "static-metadata",
		"Static metadata key=value pair exposed to all servers. May be repeated.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	for _, reserved := range []string{"server-name", "user-data"} {
		if _, ok := staticMetadata[reserved]; ok {
			setupLog.Error(nil, "Reserved static-metadata key", "key", reserved)
			return 1
		}
	}

	// metaldata is a stateless HTTP service; deploy multiple replicas and
	// rely on a Service for HA. Leader election is intentionally disabled.
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		LeaderElection:         false,
		HealthProbeBindAddress: healthBindAddress,
		Metrics: metricsserver.Options{
			BindAddress: metricsBindAddress,
		},
	})
	if err != nil {
		setupLog.Error(err, "Failed to create manager")
		return 1
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to add healthz check")
		return 1
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to add readyz check")
		return 1
	}

	idx := metaldata.NewIndex(ctrl.Log.WithName("index"), staticMetadata)

	informer, err := mgr.GetCache().GetInformer(context.Background(), &metalv1alpha1.Server{})
	if err != nil {
		setupLog.Error(err, "Failed to get Server informer")
		return 1
	}
	if _, err := informer.AddEventHandler(idx.EventHandler()); err != nil {
		setupLog.Error(err, "Failed to add event handler")
		return 1
	}

	// ServerClaim and Secret reads happen per /v1/user-data request and bypass
	// the cache so we don't watch every Secret in the cluster.
	srv := metaldata.NewServer(ctrl.Log.WithName("http"), idx, mgr.GetAPIReader(), fmt.Sprintf(":%d", listenPort))
	if err := mgr.Add(srv); err != nil {
		setupLog.Error(err, "Failed to add HTTP server")
		return 1
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager exited with error")
		return 1
	}

	return 0
}
