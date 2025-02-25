// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	webhookmetalv1alpha1 "github.com/ironcore-dev/metal-operator/internal/webhook/v1alpha1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/ironcore-dev/metal-operator/bmc"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/yaml"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/api/macdb"
	"github.com/ironcore-dev/metal-operator/internal/controller"
	"github.com/ironcore-dev/metal-operator/internal/registry"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(metalv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr                   string
		enableLeaderElection          bool
		probeAddr                     string
		secureMetrics                 bool
		enableHTTP2                   bool
		macPrefixesFile               string
		insecure                      bool
		managerNamespace              string
		probeImage                    string
		probeOSImage                  string
		registryPort                  int
		registryProtocol              string
		registryURL                   string
		registryResyncInterval        time.Duration
		webhookPort                   int
		enforceFirstBoot              bool
		enforcePowerOff               bool
		serverResyncInterval          time.Duration
		powerPollingInterval          time.Duration
		powerPollingTimeout           time.Duration
		resourcePollingInterval       time.Duration
		resourcePollingTimeout        time.Duration
		discoveryTimeout              time.Duration
		serverMaxConcurrentReconciles int
	)

	flag.IntVar(&serverMaxConcurrentReconciles, "server-max-concurrent-reconciles", 5,
		"The maximum number of concurrent Server reconciles.")
	flag.DurationVar(&discoveryTimeout, "discovery-timeout", 30*time.Minute, "Timeout for discovery boot")
	flag.DurationVar(&resourcePollingInterval, "resource-polling-interval", 5*time.Second,
		"Interval between polling resources")
	flag.DurationVar(&resourcePollingTimeout, "resource-polling-timeout", 2*time.Minute, "Timeout for polling resources")
	flag.DurationVar(&powerPollingInterval, "power-polling-interval", 5*time.Second,
		"Interval between polling power state")
	flag.DurationVar(&powerPollingTimeout, "power-polling-timeout", 2*time.Minute, "Timeout for polling power state")
	flag.DurationVar(&registryResyncInterval, "registry-resync-interval", 10*time.Second,
		"Defines the interval at which the registry is polled for new server information.")
	flag.DurationVar(&serverResyncInterval, "server-resync-interval", 2*time.Minute,
		"Defines the interval at which the server is polled.")
	flag.StringVar(&registryURL, "registry-url", "", "The URL of the registry.")
	flag.StringVar(&registryProtocol, "registry-protocol", "http", "The protocol to use for the registry.")
	flag.IntVar(&registryPort, "registry-port", 10000, "The port to use for the registry.")
	flag.StringVar(&probeImage, "probe-image", "", "Image for the first boot probing of a Server.")
	flag.StringVar(&probeOSImage, "probe-os-image", "", "OS image for the first boot probing of a Server.")
	flag.StringVar(&managerNamespace, "manager-namespace", "default", "Namespace the manager is running in.")
	flag.BoolVar(&insecure, "insecure", true, "If true, use http instead of https for connecting to a BMC.")
	flag.StringVar(&macPrefixesFile, "mac-prefixes-file", "", "Location of the MAC prefixes file.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enforceFirstBoot, "enforce-first-boot", false,
		"Enforce the first boot probing of a Server even if it is powered on in the Initial state.")
	flag.BoolVar(&enforcePowerOff, "enforce-power-off", false,
		"Enforce the power off of a Server when graceful shutdown fails.")
	flag.IntVar(&webhookPort, "webhook-port", 9445, "The port to use for webhook server.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if probeOSImage == "" {
		setupLog.Error(nil, "probe OS image must be set")
		os.Exit(1)
	}

	// Load MACAddress DB
	macPRefixes := &macdb.MacPrefixes{}
	if macPrefixesFile != "" {
		macPrefixesData, err := os.ReadFile(macPrefixesFile)
		if err != nil {
			setupLog.Error(err, "unable to read MACAddress DB")
			os.Exit(1)
		}
		if err := yaml.Unmarshal(macPrefixesData, macPRefixes); err != nil {
			setupLog.Error(err, "failed to unmarshal the MAC prefixes file")
			os.Exit(1)
		}
	}

	// set the correct registry URL by getting the address from the environment
	var registryAddr string
	if registryURL == "" {
		registryAddr = os.Getenv("REGISTRY_ADDRESS")
		if registryAddr == "" {
			setupLog.Error(nil, "failed to set the registry URL as no address is provided")
			os.Exit(1)
		}
		registryURL = fmt.Sprintf("%s://%s:%d", registryProtocol, registryAddr, registryPort)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		Port:    webhookPort,
		TLSOpts: tlsOpts,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f26702e4.ironcore.dev",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.EndpointReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		MACPrefixes: macPRefixes,
		Insecure:    insecure,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Endpoints")
		os.Exit(1)
	}
	if err = (&controller.BMCSecretReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BMCSecret")
		os.Exit(1)
	}
	if err = (&controller.BMCReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Insecure: insecure,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BMC")
		os.Exit(1)
	}
	if err = (&controller.ServerReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		Insecure:                insecure,
		ManagerNamespace:        managerNamespace,
		ProbeImage:              probeImage,
		ProbeOSImage:            probeOSImage,
		RegistryURL:             registryURL,
		RegistryResyncInterval:  registryResyncInterval,
		ResyncInterval:          serverResyncInterval,
		EnforceFirstBoot:        enforceFirstBoot,
		EnforcePowerOff:         enforcePowerOff,
		MaxConcurrentReconciles: serverMaxConcurrentReconciles,
		BMCOptions: bmc.BMCOptions{
			BasicAuth:               true,
			PowerPollingInterval:    powerPollingInterval,
			PowerPollingTimeout:     powerPollingTimeout,
			ResourcePollingInterval: resourcePollingInterval,
			ResourcePollingTimeout:  resourcePollingTimeout,
		},
		DiscoveryTimeout: discoveryTimeout,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Server")
		os.Exit(1)
	}
	if err = (&controller.ServerBootConfigurationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServerBootConfiguration")
		os.Exit(1)
	}
	if err = (&controller.ServerClaimReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServerClaim")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookmetalv1alpha1.SetupEndpointWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Endpoint")
			os.Exit(1)
		}
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	setupLog.Info("starting registry server", "RegistryURL", registryURL)
	registryServer := registry.NewServer(fmt.Sprintf(":%d", registryPort))
	go func() {
		if err := registryServer.Start(ctx); err != nil {
			setupLog.Error(err, "problem running registry server")
			os.Exit(1)
		}
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
