# BMCSecrets

The `BMCSecret` Custom Resource Definition (CRD) is a Kubernetes resource used to store sensitive credentials required 
to access a Baseboard Management Controller (BMC). This resource holds the `username` and `password` needed for 
authentication with the BMC devices. The `BMCSecret` is utilized by the `BMCReconciler` to construct clients that 
interact with BMCs.

## Example BMCSecret Resource

An example of how to define an `BMCSecret` resource:

```yaml
apiVersion: v1alpha1
kind: BMCSecret
metadata:
  name: my-bmc-secret
stringData:
  username: admin
  password: supersecretpassword
type: Opaque
```

## Usage

The `BMCSecret` resource is essential for securely managing credentials required to access BMC devices. It is used by 
the `BMCReconciler` to:

- **Construct BMC Clients**: Utilize the credentials to authenticate and establish connections with BMC devices.
- **Automate Hardware Management**: Enable automated operations such as power control, firmware updates, and 
health monitoring by authenticating with the BMC.

## Credential Sources

- **Endpoint-Based Discovery**: When BMCs are discovered through an [`Endpoint`](endpoints.md) resource and a MAC Prefix Database, 
the credentials (`username` and `password`) are derived automatically based on the MAC address prefixes.
- **Manual Configuration**: Users can manually create BMCSecret resources with the required credentials to interact with specific BMCs.

## Reconciliation Process

The `BMCReconciler` uses the `bmcSecretRef` field in the BMC resource's specification to reference the corresponding
`BMCSecret`. It retrieves the credentials from the BMCSecret to authenticate with the BMC device.
