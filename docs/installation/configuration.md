# Manager Configuration

The metal-operator manager is configured via command line flags. When deploying
with Helm or Kustomize, flags are passed as container args of the manager
Deployment.

## Watch filter

| Flag | Default | Description |
|---|---|---|
| `--watch-filter` | `""` | Watch filter value selecting the resources this instance owns. Only resources labeled `metal.ironcore.dev/watch-filter=<value>` are watched and reconciled. If empty, only resources *without* the watch-filter label are handled. See [Watch Filter](/concepts/watch-filter). |

## Server reconciliation

| Flag | Default | Description |
|---|---|---|
| `--server-max-concurrent-reconciles` | `5` | Maximum number of concurrent Server reconciles. |
| `--server-claim-max-concurrent-reconciles` | `5` | Maximum number of concurrent ServerClaim reconciles. |
| `--server-resync-interval` | `2m` | Interval at which a Server is polled. |
| `--enforce-first-boot` | `false` | Enforce first boot probing of a Server even if it is powered on in the Initial state. |
| `--enforce-power-off` | `false` | Forcibly power off a Server when graceful shutdown fails. |
| `--default-failed-auto-retry-count` | `0` | Default number of auto retries for a CR when it fails. `0` for no retries. |
| `--maintenance-resync-interval` | `2m` | Interval at which the CR performing maintenance is polled. |
| `--dns-record-template-path` | `""` | Path to the DNS record template file used for creating DNS records for Servers. |

## BMC access

| Flag | Default | Description |
|---|---|---|
| `--protocol` | `""` | Protocol for BMC connections: `http` or `https`. If unset, derived from `--insecure` for compatibility. |
| `--skip-cert-validation` | `false` | Skip TLS certificate validation when using HTTPS. |
| `--insecure` | `true` | Deprecated: use `--protocol` and `--skip-cert-validation` instead. |
| `--bmc-failure-reset-delay` | `0s` | Reset the BMC after this duration of consecutive failures. `0` to disable. |
| `--bmc-reset-resync-interval` | `2m` | Interval at which the BMC is polled while a reset is in progress. |
| `--bmc-reset-waiting-interval` | `2m` | Wait time before reconciling again after a BMC reset. |
| `--manager-namespace` | `default` | Namespace the manager is running in. |

## Discovery and registry

| Flag | Default | Description |
|---|---|---|
| `--probe-image` | `""` | Image for the first boot probing of a Server. Required. |
| `--probe-os-image` | `""` | OS image for the first boot probing of a Server. Required. |
| `--registry-url` | `""` | URL of the registry. Derived from the `REGISTRY_ADDRESS` environment variable if unset. |
| `--registry-protocol` | `http` | Protocol to use for the registry. |
| `--registry-port` | `10000` | Port to use for the registry. |
| `--registry-resync-interval` | `10s` | Interval at which the registry is polled for new server information. |
| `--registry-client-timeout` | `5s` | Timeout for HTTP requests to the registry. |
| `--registry-data-max-age` | `2m` | Maximum age of registry data to accept for discovery completion. |
| `--discovery-timeout` | `30m` | Timeout for discovery boot. |
| `--discovery-ignition-path` | `/etc/metal-operator/ignition-template.yaml` | Path to the ignition template file. |
| `--mac-prefixes-file` | `""` | Location of the MAC prefixes file. |

## Polling

| Flag | Default | Description |
|---|---|---|
| `--resource-polling-interval` | `5s` | Interval between polling resources. |
| `--resource-polling-timeout` | `2m` | Timeout for polling resources. |
| `--power-polling-interval` | `5s` | Interval between polling power state. |
| `--power-polling-timeout` | `2m` | Timeout for polling power state. |
| `--bios-setting-timeout` | `2h` | Timeout for the BIOS settings controller. |

## Observability

| Flag | Default | Description |
|---|---|---|
| `--metrics-bind-address` | `:8080` | Address the metrics endpoint binds to. |
| `--metrics-secure` | `true` | Serve the metrics endpoint securely. |
| `--health-probe-bind-address` | `:8081` | Address the probe endpoint binds to. |
| `--event-url` | `""` | URL of the server events endpoint for alerts and metrics. Falls back to the `EVENT_ADDRESS` environment variable. |
| `--event-protocol` | `http` | Protocol for the server events endpoint. |
| `--event-port` | `10001` | Port of the server events endpoint. |

## TLS and webhooks

| Flag | Default | Description |
|---|---|---|
| `--webhook-port` | `9443` | Port for the webhook server. |
| `--webhook-cert-path` / `--webhook-cert-name` / `--webhook-cert-key` | `""`, `tls.crt`, `tls.key` | Serving certificate for webhooks. |
| `--metrics-cert-path` / `--metrics-cert-name` / `--metrics-cert-key` | `""`, `tls.crt`, `tls.key` | Serving certificate for the metrics endpoint. |
| `--enable-http2` | `false` | Enable HTTP/2 for the metrics and webhook servers. Disabled by default due to CVE-2023-44487. |

## General

| Flag | Default | Description |
|---|---|---|
| `--leader-elect` | `false` | Enable leader election. Ensures only one active replica per deployment. |
| `--kubeconfig` | `""` | Path to a kubeconfig. Only required out-of-cluster. |
| `--zap-*` | | Logging flags: `--zap-devel`, `--zap-encoder`, `--zap-log-level`, `--zap-stacktrace-level`, `--zap-time-encoding`. |

The authoritative, always up-to-date list is `metal-operator --help`.
