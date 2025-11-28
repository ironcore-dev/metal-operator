# Metal Operator Helm Chart - Ignition Template Customization

This Helm chart allows you to optionally override the default hardcoded Ignition template used by the metal-operator for bare metal server provisioning.

## Overview

The metal-operator uses [Ignition](https://coreos.github.io/ignition/) templates to configure bare metal servers during their first boot. By default, it uses a hardcoded template. You can optionally enable ConfigMap-based template customization to meet your specific requirements.

## Configuration

### Basic Configuration

The ignition configuration is controlled by the `ignition` section in `values.yaml`:

```yaml
ignition:
  enable: false                         # Enable/disable ignition ConfigMap override (default: false)
  configMapName: "ignition-template"    # Name suffix for the ConfigMap
  configMapKey: "ignition-template.yaml"  # Key in the ConfigMap containing the template
  template: |                           # The actual Ignition template content (only used when enable: true)
    # Your custom Ignition template here
```

**Default Behavior**: When `ignition.enable: false` (default), the operator uses its built-in hardcoded template.
**Override Behavior**: When `ignition.enable: true`, the operator uses the ConfigMap template and falls back to hardcoded if ConfigMap is unavailable.

### Template Variables

Your custom template must include these template variables for proper operation:

- `{{.Image}}` - Docker image for the metalprobe container
- `{{.Flags}}` - Command-line flags for metalprobe (includes --registry-url and --server-uuid)
- `{{.SSHPublicKey}}` - SSH public key for server access
- `{{.PasswordHash}}` - Bcrypt hash of the user password

### Example: Custom Template

```yaml
ignition:
  enable: true
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

### Using Default Hardcoded Template (Recommended)

```bash
helm install my-metal-operator ./
```

This uses the built-in hardcoded template. No ConfigMap is created, and the operator works immediately.

### Using Custom Template Override

1. Enable ignition customization:
```bash
helm install my-metal-operator ./ --set ignition.enable=true
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

### ConfigMap Not Found

If you see errors about ConfigMap not being found:

1. Verify the ConfigMap was created:
```bash
kubectl get configmap -n <namespace>
```

2. Check the manager logs:
```bash
kubectl logs -n <namespace> deployment/metal-operator-controller-manager
```

### Template Syntax Errors

If ignition generation fails due to template syntax:

1. Validate your template syntax offline
2. Check that all required template variables are included
3. Verify the Ignition format is valid

### Permission Issues

If the controller cannot read the ConfigMap:
1. Verify RBAC permissions include ConfigMap access
2. Check that the ConfigMap is in the correct namespace

## Examples

See `values-custom-example.yaml` for a complete example of custom template configuration with:
- Custom storage mounts
- Additional Docker configuration
- Custom metalprobe service settings
- Multiple users
- Custom scripts and files