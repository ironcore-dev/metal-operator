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

| Key                                               | Description                                                                 | Default Value                  |
|---------------------------------------------------|-----------------------------------------------------------------------------|--------------------------------|
| `controllerManager.replicas`                      | Number of replicas for the manager deployment                               | `1`                            |
| `controllerManager.strategy.type`                 | Deployment strategy for the manager pod                                     | `Recreate`                     |
| `controllerManager.manager.image.repository`      | Image repository for the manager container                                  | `controller`                   |
| `controllerManager.manager.image.tag`             | Image tag for the manager container                                         | `latest`                       |
| `controllerManager.manager.args`                  | Arguments for the manager container                                         | `--leader-elect`, `--metrics-bind-address=:8443`, `--health-probe-bind-address=:8081` |
| `controllerManager.manager.resources`             | Resource requests and limits for the manager container                      | `{cpu: 300m, memory: 200Mi}` (limits), `{cpu: 300m, memory: 50Mi}` (requests) |
| `controllerManager.manager.livenessProbe`         | Liveness probe configuration for the manager container                      | `{initialDelaySeconds: 15, periodSeconds: 20, httpGet: {path: /healthz, port: 8081}}` |
| `controllerManager.manager.readinessProbe`        | Readiness probe configuration for the manager container                     | `{initialDelaySeconds: 5, periodSeconds: 10, httpGet: {path: /readyz, port: 8081}}` |
| `controllerManager.manager.securityContext`       | Security context for the manager container                                  | `{allowPrivilegeEscalation: false, capabilities: {drop: ["ALL"]}}` |
| `controllerManager.podSecurityContext`            | Security context for the manager pod                                        | `{runAsNonRoot: true, seccompProfile: {type: RuntimeDefault}}` |
| `controllerManager.terminationGracePeriodSeconds` | Termination grace period for the manager pod                                | `10`                           |
| `controllerManager.serviceAccountName`            | Service account name for the manager pod                                    | `metal-operator-controller-manager` |
| `controllerManager.hostNetwork`                   | Enable host networking for the manager pod                                  | `true`                         |

- **rbac**: Enable or disable RBAC.
- **crd**: Enable or disable CRDs.
- **metrics**: Enable or disable metrics export.
- **webhook**: Enable or disable webhooks.
- **prometheus**: Enable or disable Prometheus ServiceMonitor.
- **certmanager**: Enable or disable cert-manager injection.
- **networkPolicy**: Enable or disable NetworkPolicies.

Refer to the `values.yaml` file for more details on each configuration option.

## Uninstallation

To uninstall the Metal Operator, run the following command:

```sh
helm uninstall metal-operator
```

This will remove all the resources associated with the Metal Operator.

## Additional Information

For more detailed information, refer to the official documentation and Helm chart repository.

