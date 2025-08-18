// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	webhookmetalv1alpha1 "github.com/ironcore-dev/metal-operator/internal/webhook/v1alpha1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/yaml"

	"github.com/ironcore-dev/metal-operator/bmc"

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

func main() { // nolint: gocyclo
	var (
		metricsAddr                        string
		metricsCertPath                    string
		metricsCertName                    string
		metricsCertKey                     string
		webhookCertPath                    string
		webhookCertName                    string
		webhookCertKey                     string
		enableLeaderElection               bool
		probeAddr                          string
		secureMetrics                      bool
		enableHTTP2                        bool
		macPrefixesFile                    string
		insecure                           bool
		managerNamespace                   string
		probeImage                         string
		probeOSImage                       string
		registryPort                       int
		registryProtocol                   string
		registryURL                        string
		registryResyncInterval             time.Duration
		webhookPort                        int
		enforceFirstBoot                   bool
		enforcePowerOff                    bool
		serverResyncInterval               time.Duration
		powerPollingInterval               time.Duration
		powerPollingTimeout                time.Duration
		resourcePollingInterval            time.Duration
		resourcePollingTimeout             time.Duration
		discoveryTimeout                   time.Duration
		biosSettingsApplyTimeout           time.Duration
		serverMaxConcurrentReconciles      int
		serverClaimMaxConcurrentReconciles int
	)

	flag.IntVar(&serverMaxConcurrentReconciles, "server-max-concurrent-reconciles", 5,
		"The maximum number of concurrent Server reconciles.")
	flag.IntVar(&serverClaimMaxConcurrentReconciles, "server-claim-max-concurrent-reconciles", 5,
		"The maximum number of concurrent ServerClaim reconciles.")
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
	flag.IntVar(&webhookPort, "webhook-port", 9443, "The port to use for webhook server.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.DurationVar(&biosSettingsApplyTimeout, "bios-setting-timeout", 2*time.Hour,
		"Timeout for BIOS Settings Controller")
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

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		var err error
		webhookCertWatcher, err = certwatcher.New(
			filepath.Join(webhookCertPath, webhookCertName),
			filepath.Join(webhookCertPath, webhookCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	webhookServer := webhook.NewServer(webhook.Options{
		Port:    webhookPort,
		TLSOpts: webhookTLSOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
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
		BMCOptions: bmc.Options{
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
		Client:                  mgr.GetClient(),
		Cache:                   mgr.GetCache(),
		Scheme:                  mgr.GetScheme(),
		MaxConcurrentReconciles: serverClaimMaxConcurrentReconciles,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServerClaim")
		os.Exit(1)
	}
	if err = (&controller.ServerMaintenanceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServerMaintenance")
		os.Exit(1)
	}

	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookmetalv1alpha1.SetupEndpointWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Endpoint")
			os.Exit(1)
		}
	}
	if err = (&controller.BiosSettingsReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ManagerNamespace: managerNamespace,
		Insecure:         insecure,
		ResyncInterval:   serverResyncInterval,
		BMCOptions: bmc.Options{
			BasicAuth:               true,
			PowerPollingInterval:    powerPollingInterval,
			PowerPollingTimeout:     powerPollingTimeout,
			ResourcePollingInterval: resourcePollingInterval,
			ResourcePollingTimeout:  resourcePollingTimeout,
		},
		TimeoutExpiry: biosSettingsApplyTimeout,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BIOSSettings")
		os.Exit(1)
	}
	if err = (&controller.UserReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Insecure: insecure,
		BMCOptions: bmc.Options{
			BasicAuth:               true,
			PowerPollingInterval:    powerPollingInterval,
			PowerPollingTimeout:     powerPollingTimeout,
			ResourcePollingInterval: resourcePollingInterval,
			ResourcePollingTimeout:  resourcePollingTimeout,
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "User")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookmetalv1alpha1.SetupBIOSSettingsWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "BIOSSettings")
			os.Exit(1)
		}
	}
	if err = (&controller.BIOSVersionReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ManagerNamespace: managerNamespace,
		Insecure:         insecure,
		ResyncInterval:   serverResyncInterval,
		BMCOptions: bmc.Options{
			BasicAuth:               true,
			PowerPollingInterval:    powerPollingInterval,
			PowerPollingTimeout:     powerPollingTimeout,
			ResourcePollingInterval: resourcePollingInterval,
			ResourcePollingTimeout:  resourcePollingTimeout,
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BIOSVersion")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookmetalv1alpha1.SetupBIOSVersionWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "BIOSVersion")
			os.Exit(1)
		}
	}
	if err = (&controller.BMCSettingsReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ManagerNamespace: managerNamespace,
		ResyncInterval:   serverResyncInterval,
		Insecure:         insecure,
		BMCOptions: bmc.Options{
			BasicAuth:               true,
			PowerPollingInterval:    powerPollingInterval,
			PowerPollingTimeout:     powerPollingTimeout,
			ResourcePollingInterval: resourcePollingInterval,
			ResourcePollingTimeout:  resourcePollingTimeout,
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BMCSettings")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookmetalv1alpha1.SetupBMCSettingsWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "BMCSettings")
			os.Exit(1)
		}
	}
	if err = (&controller.BMCVersionReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ManagerNamespace: managerNamespace,
		Insecure:         insecure,
		ResyncInterval:   serverResyncInterval,
		BMCOptions: bmc.Options{
			BasicAuth:               true,
			PowerPollingInterval:    powerPollingInterval,
			PowerPollingTimeout:     powerPollingTimeout,
			ResourcePollingInterval: resourcePollingInterval,
			ResourcePollingTimeout:  resourcePollingTimeout,
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BMCVersion")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookmetalv1alpha1.SetupBMCVersionWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "BMCVersion")
			os.Exit(1)
		}
	}
	if err = (&controller.BIOSVersionSetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BIOSVersionSet")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = webhookmetalv1alpha1.SetupServerWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Server")
			os.Exit(1)
		}
	}
	if err = (&controller.BMCVersionSetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BMCVersionSet")
		os.Exit(1)
	}
	if err = (&controller.BIOSSettingsSetReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BIOSSettingsSet")
		os.Exit(1)
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
