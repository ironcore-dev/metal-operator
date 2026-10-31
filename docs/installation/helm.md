# Helm Installation

This guide will help you install the Metal Operator using Helm.

## Prerequisites

- Kubernetes cluster (v1.16+)
- Helm (v3.0.0+)

## Steps

1. **Install the Chart**

   Install the Metal Operator chart with the default values.

   ```sh
   helm install metal-operator dist/chart
   ```

   To customize the installation, you can override the default values using a `values.yaml` file or the `--set` flag.

   ```sh
   helm install metal-operator dist/chart -f /path/to/your/values.yaml
   ```

2. **Verify the Installation**

   Check the status of the Helm release to ensure that the Metal Operator is installed successfully.

   ```sh
   helm status metal-operator
   ```

   You should see output indicating that the Metal Operator pods are running.

## Configuration

The `values.yaml` file allows you to configure various aspects of the Metal Operator. Below are some of the key configurations:

### Controller Manager

| Key                                     | Description                                                                 | Default Value                  |
|-----------------------------------------|-----------------------------------------------------------------------------|--------------------------------|
| `manager.enabled`                       | Install the manager Deployment                                              | `true`                         |
| `manager.replicas`                      | Number of replicas for the manager deployment                               | `1`                            |
| `manager.strategy`                      | Deployment strategy for the manager pod                                     | not set (Kubernetes default `RollingUpdate`) |
| `manager.image.repository`              | Image repository for the manager container                                  | `controller`                   |
| `manager.image.tag`                     | Image tag for the manager container                                         | `Chart.appVersion`             |
| `manager.image.pullPolicy`              | Image pull policy for the manager container                                 | `IfNotPresent`                 |
| `manager.imagePullSecrets`              | Image pull secrets for the manager pod                                      | not set                        |
| `manager.args`                          | Extra arguments for the manager container (the chart adds the metrics and certificate flags) | `--leader-elect` |
| `manager.env`                           | Environment variables for the manager container                             | `ENABLE_WEBHOOKS=true`         |
| `manager.envOverrides`                  | Map of extra environment variables (`NAME: value`)                          | not set                        |
| `manager.resources`                     | Resource requests and limits for the manager container                      | `{cpu: 500m, memory: 128Mi}` (limits), `{cpu: 10m, memory: 64Mi}` (requests) |
| `manager.securityContext`               | Security context for the manager container                                  | `{allowPrivilegeEscalation: false, capabilities: {drop: ["ALL"]}}` |
| `manager.podSecurityContext`            | Security context for the manager pod                                        | `{runAsNonRoot: true, seccompProfile: {type: RuntimeDefault}}` |
| `manager.extraVolumes`                  | Extra volumes for the manager pod                                           | not set                        |
| `manager.extraVolumeMounts`             | Extra volume mounts for the manager container                               | not set                        |
| `manager.affinity`                      | Affinity for the manager pod                                                | `{}`                           |
| `manager.nodeSelector`                  | Node selector for the manager pod                                           | `{}`                           |
| `manager.tolerations`                   | Tolerations for the manager pod                                             | `[]`                           |
| `manager.topologySpreadConstraints`     | Topology spread constraints for the manager pod                             | not set                        |
| `manager.priorityClassName`             | Priority class name for the manager pod                                     | not set                        |
| `manager.terminationGracePeriodSeconds` | Termination grace period for the manager pod                                | `10`                           |
| `manager.labels`                        | Extra labels for the manager Deployment                                     | not set                        |
| `manager.annotations`                   | Annotations for the manager Deployment                                      | not set                        |
| `manager.pod.labels`                    | Extra labels for the manager pod                                            | not set                        |
| `manager.pod.annotations`               | Extra annotations for the manager pod                                       | not set                        |

The liveness (`/healthz`) and readiness (`/readyz`) probes on port `8081` are fixed in the template. The chart has no
`hostNetwork` option; the manager pod uses the pod network.

### Other Settings

| Key                          | Description                                                                 | Default Value |
|------------------------------|-----------------------------------------------------------------------------|---------------|
| `rbac.namespaced`            | Use a Role/RoleBinding in the release namespace instead of a ClusterRole/ClusterRoleBinding | `false` |
| `rbac.helpers.enable`        | Install admin/editor/viewer roles for the CRDs                              | `false`       |
| `serviceAccount.enable`      | Create the manager ServiceAccount                                           | `true`        |
| `serviceAccount.name`        | Existing ServiceAccount to use when `serviceAccount.enable` is `false`      | not set       |
| `crd.enable`                 | Install the CRDs with the chart                                             | `true`        |
| `crd.keep`                   | Keep the CRDs when the chart is uninstalled                                 | `true`        |
| `metrics.enable`             | Expose the `/metrics` endpoint                                              | `true`        |
| `metrics.port`               | Metrics server port                                                         | `8443`        |
| `metrics.secure`             | Serve metrics over HTTPS with authn/authz                                   | `true`        |
| `certManager.enable`         | Use cert-manager for the webhook and metrics certificates                   | `true`        |
| `webhook.enable`             | Install the webhooks                                                        | `true`        |
| `webhook.port`               | Webhook server port                                                         | `9443`        |
| `prometheus.enable`          | Install a Prometheus ServiceMonitor (requires prometheus-operator)          | `false`       |
| `metaldata.image.repository` | Image repository for the metaldata DaemonSet                                | `ghcr.io/ironcore-dev/metaldata` |
| `metaldata.image.tag`        | Image tag for the metaldata DaemonSet                                       | `Chart.appVersion` |
| `metaldata.additionalLabels` | Extra labels for the metaldata DaemonSet                                    | `{}`          |

Refer to the `values.yaml` file for more details on each configuration option.

## Uninstallation

To uninstall the Metal Operator, run the following command:

```sh
helm uninstall metal-operator
```

This will remove all the resources associated with the Metal Operator.

## Additional Information

For more detailed information, refer to the official documentation and Helm chart repository.

