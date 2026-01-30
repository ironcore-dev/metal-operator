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

## Ignition Template Customization

The metal-operator uses [Ignition](https://coreos.github.io/ignition/) templates to configure bare metal servers during their first boot. The operator includes a default template file in the container at `/etc/metal-operator/ignition-template.yaml`. You can optionally override this template by mounting a ConfigMap at the same location.

### Configuration

The ignition configuration is controlled by the `ignition` section in `values.yaml`:

```yaml
ignition:
  override: false       # Enable/disable ignition ConfigMap override (default: false)
  template: |           # The actual Ignition template content (only used when override: true)
    # Your custom Ignition template here
```

**Default Behavior**: When `ignition.override: false` (default), the operator uses the template file in the container image at `/etc/metal-operator/ignition-template.yaml`.

**Override Behavior**: When `ignition.override: true`, a ConfigMap is created and mounted to `/etc/metal-operator/ignition-template.yaml`, replacing the default template file.

### Template Variables

Your custom template must include these template variables for proper operation:

- <span v-pre>`{{.Image}}`</span> - Docker image for the metalprobe container
- <span v-pre>`{{.Flags}}`</span> - Command-line flags for metalprobe (includes --registry-url and --server-uuid)
- <span v-pre>`{{.SSHPublicKey}}`</span> - SSH public key for server access
- <span v-pre>`{{.PasswordHash}}`</span> - Bcrypt hash of the user password

### Example: Custom Template

```yaml
ignition:
  override: true
  template: |
    variant: fcos
    version: "1.3.0"
    systemd:
      units:
        - name: metalprobe.service
          enabled: true
          contents: |-
            [Unit]
            Description=Metal Probe Service
            [Service]
            Restart=on-failure
            ExecStartPre=/usr/bin/docker pull {{.Image}}
            ExecStart=/usr/bin/docker run --name metalprobe {{.Image}} {{.Flags}}
            [Install]
            WantedBy=multi-user.target
    passwd:
      users:
        - name: metal
          password_hash: {{.PasswordHash}}
          groups: [ "wheel" ]
          ssh_authorized_keys: [ {{.SSHPublicKey}} ]
```

### Using Default Template (Recommended)

```bash
helm install metal-operator dist/chart
```

This uses the template file in the container image. No ConfigMap is created, and the operator works immediately with the default configuration.

### Using Custom Template Override

1. Enable ignition customization:
```bash
helm install metal-operator dist/chart --set ignition.override=true
```

2. Or create a custom values file:
```bash
cp dist/chart/values.yaml my-values.yaml
# Edit my-values.yaml with your customizations
```

3. Deploy with custom values:
```bash
helm install metal-operator dist/chart -f my-values.yaml
```

### Upgrading Template

To update just the template content:

```bash
helm upgrade metal-operator dist/chart -f my-values.yaml
```

The ConfigMap will be updated and new server provisioning will use the updated template.

### Advanced Customization

#### Adding Custom Services

You can add additional systemd services to the template:

```yaml
systemd:
  units:
    - name: my-custom-service.service
      enabled: true
      contents: |-
        [Unit]
        Description=My Custom Service
        [Service]
        Type=simple
        ExecStart=/usr/local/bin/my-script.sh
        [Install]
        WantedBy=multi-user.target
```

#### Custom Storage Configuration

Modify the storage section to add custom files and filesystems:

```yaml
storage:
  files:
    - path: /etc/my-config.conf
      mode: 0644
      contents:
        inline: |
          # My custom configuration
          key=value
  filesystems:
    - name: data
      device: /dev/sdb
      format: ext4
```

#### Multiple Users

Add additional users to the system:

```yaml
passwd:
  users:
    - name: metal
      password_hash: {{.PasswordHash}}
      groups: [ "wheel" ]
      ssh_authorized_keys: [ {{.SSHPublicKey}} ]
    - name: admin
      password_hash: {{.PasswordHash}}
      groups: [ "wheel", "sudo" ]
      ssh_authorized_keys: [ {{.SSHPublicKey}} ]
```

### Troubleshooting

#### Template File Not Found

If you see errors about template file not being found:

1. Verify the default file exists in the container:
```bash
kubectl exec -n <namespace> deployment/metal-operator-controller-manager -- ls -la /etc/metal-operator/
```

2. If using ConfigMap override, verify it was created and mounted:
```bash
kubectl get configmap -n <namespace>
kubectl describe deployment -n <namespace> metal-operator-controller-manager
```

3. Check the manager logs:
```bash
kubectl logs -n <namespace> deployment/metal-operator-controller-manager
```

#### Template Syntax Errors

If ignition generation fails due to template syntax:

1. Validate your template syntax offline
2. Check that all required template variables are included
3. Verify the Ignition format is valid
4. If using a custom template, ensure the ConfigMap is properly mounted

#### Volume Mount Issues

If the ConfigMap override is not working:
1. Verify the ConfigMap is mounted correctly:
```bash
kubectl describe pod -n <namespace> -l control-plane=controller-manager
```
2. Check that `ignition.override: true` is set in values
3. Verify RBAC permissions include ConfigMap access

## Additional Information

For more detailed information, refer to the official documentation and Helm chart repository.

