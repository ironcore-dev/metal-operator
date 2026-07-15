# ServerClaims

The `ServerClaim` Custom Resource Definition (CRD) is a Kubernetes resource used to claim ownership of a bare metal 
[`Server`](servers.md) resource that is in the `Available` state. It allows users to specify the desired 
operating system image and ignition configuration for booting the server. The `ServerClaimReconciler` handles the 
allocation of servers to claims and manages the lifecycle of the claim and the server.

## Example ServerClaim Resource

Claiming a Specific Server with Ignition Configuration:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerClaim
metadata:
  name: my-server-claim
  namespace: default
spec:
  power: "On"
  serverRef:
    name: "my-server"
  image: "my-osimage:latest"
  ignitionSecretRef:
    name: "my-ignition-secret"
```

Claiming a Server Using a Selector:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: ServerClaim
metadata:
  name: selector-server-claim
  namespace: default
spec:
  power: "On"
  serverSelector:
    matchLabels:
      hardwareType: gpu-node
      location: datacenter-1
  image: my-osimage:latest
  ignitionSecretRef:
    name: my-ignition-secret
```

## Reconciliation Process

- [`ServerBootConfiguration`](serverbootconfigurations.md):
    - The `ServerClaimReconciler` creates a [`ServerBootConfiguration`](serverbootconfigurations.md) resource under the hood.
    - This resource specifies how the server should be booted, including the image and ignition configuration.

- **State Transitions**:
    - `Available` → `Reserved`: When a server is successfully claimed.
    - `Reserved` → `Available`: When the `ServerClaim` is deleted and the server's
      [`spec.reclaimPolicy`](servers.md#reclaim-policy) is `Recycle` (the default).
    - `Reserved` → `Released`: When the `ServerClaim` is deleted and the server's
      [`spec.reclaimPolicy`](servers.md#reclaim-policy) is `Retain`. The server stays in `Released`
      with `spec.serverClaimRef` set until an operator clears the reference, at which point it
      transitions back to `Available`.

- **Release Process**:
    - On claim deletion the server is powered off and its `BootConfigurationRef` is cleared.
    - With `Recycle`, `spec.serverClaimRef` is also removed automatically and the server returns to `Available`.
    - With `Retain`, `spec.serverClaimRef` is preserved so the binding can be inspected before the
      server is released back into the pool.

## Cordoned Servers

A server can be **cordoned** via [`spec.unschedulable`](servers.md#cordoning) to prevent new
`ServerClaim`s from binding to it. See [Cordoning](servers.md#cordoning) for details on how cordon
interacts with claim binding and already-bound claims.
