# Metal Operator Helm Chart - Ignition Template Customization

This Helm chart allows you to optionally override the default Ignition template used by the metal-operator for bare metal server provisioning.

## Overview

The metal-operator uses [Ignition](https://coreos.github.io/ignition/) templates to configure bare metal servers during their first boot. The operator includes a default template file baked into the container at `/etc/metal-operator/ignition-template.yaml`. You can optionally override this template by mounting a ConfigMap at the same location.

## Configuration

### Basic Configuration

The ignition configuration is controlled by the `ignition` section in `values.yaml`:

```yaml
ignition:
  override: false       # Enable/disable ignition ConfigMap override (default: false)
  template: |           # The actual Ignition template content (only used when override: true)
    # Your custom Ignition template here
```

**Default Behavior**: When `ignition.override: false` (default), the operator uses the template file baked into the container image at `/etc/metal-operator/ignition-template.yaml`.

**Override Behavior**: When `ignition.override: true`, a ConfigMap is created and mounted to `/etc/metal-operator/ignition-template.yaml`, replacing the default template file.

### Template Variables

Your custom template must include these template variables for proper operation:

- `{{.Image}}` - Docker image for the metalprobe container
- `{{.Flags}}` - Command-line flags for metalprobe (includes --registry-url and --server-uuid)
- `{{.SSHPublicKey}}` - SSH public key for server access
- `{{.PasswordHash}}` - Bcrypt hash of the user password

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

## Deployment

### Using Default Template (Recommended)

```bash
helm install my-metal-operator ./
```

This uses the template file baked into the container image. No ConfigMap is created, and the operator works immediately with the default configuration.

### Using Custom Template Override

1. Enable ignition customization:
```bash
helm install my-metal-operator ./ --set ignition.override=true
```

2. Or create a custom values file:
```bash
cp values.yaml my-values.yaml
# Edit my-values.yaml with your customizations
```

3. Deploy with custom values:
```bash
helm install my-metal-operator ./ -f my-values.yaml
```

### Upgrading Template

To update just the template content:

```bash
helm upgrade my-metal-operator ./ -f my-values.yaml
```

The ConfigMap will be updated and new server provisioning will use the updated template.

## Advanced Customization

### Adding Custom Services

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

### Custom Storage Configuration

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

### Multiple Users

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

## Troubleshooting

### Template File Not Found

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

### Template Syntax Errors

If ignition generation fails due to template syntax:

1. Validate your template syntax offline
2. Check that all required template variables are included
3. Verify the Ignition format is valid
4. If using a custom template, ensure the ConfigMap is properly mounted

### Volume Mount Issues

If the ConfigMap override is not working:
1. Verify the ConfigMap is mounted correctly:
```bash
kubectl describe pod -n <namespace> -l control-plane=controller-manager
```
2. Check that `ignition.override: true` is set in values
3. Verify RBAC permissions include ConfigMap access

## Examples

See `values-custom-example.yaml` for a complete example of custom template configuration with:
- Custom storage mounts
- Additional Docker configuration
- Custom metalprobe service settings
- Multiple users
- Custom scripts and files