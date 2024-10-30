# ServerBootConfigurations

The `ServerBootConfiguration` Custom Resource Definition (CRD) is a Kubernetes resource used to signal the need to 
initiate a boot process for a bare metal server. It serves as an indicator for external components responsible for 
configuring network boot environments, such as PXE or HTTPBoot servers. The `ServerBootConfiguration` resource allows 
the `metal-operator` to delegate the boot preparation process to third-party operators like the 
[`boot-operator`](https://github.com/ironcore-dev/boot-operator) or tools like OpenStack Ironic.

## Example ServerBootConfiguration Resource

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerBootConfiguration
metadata:
  name: my-server-boot-config
  namespace: defauilt
spec:
  serverRef:
    name: my-server
  image: my-osimage:latest
  ignitionSecretRef:
    name: my-ignition-secret
```

## Integration with Third-Party Components

The actual preparation of the boot environment is performed by external components, which may include:
- boot-operator: A custom operator that handles boot environment preparation as part of the IronCore project.
- OpenStack Ironic: A service for managing and provisioning bare metal servers.

These components watch for `ServerBootConfiguration` resources and perform the necessary actions to set up the boot 
environment according to the specifications provided.

## Why externalizing the boot preparation to a Third-Party?

**Separation of Concerns**: By abstracting the boot preparation into a separate resource, the `metal-operator` 
remains agnostic to the specifics of the boot process, allowing for flexibility in different deployment scenarios.

**Custom Implementations**: Users can implement their own components to handle the `ServerBootConfiguration`, enabling 
integration with various provisioning systems or custom workflows.

## Reconciliation Process

The `ServerReconciler` checks the `ServerBootConfiguration` status before powering on the server. Servers are not 
powered on until the boot environment is confirmed to be `ready`.
