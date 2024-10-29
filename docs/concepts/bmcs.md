# BMCs

The BMC Custom Resource Definition (CRD) represents a Baseboard Management Controller. 
It is designed to manage and monitor the state of BMC devices and the systems (servers) they control. The primary 
purpose of the BMC resource is to reconcile the BMC state and detect all systems it manages by creating the 
corresponding [`Server`](servers.md) resources.

## Example BMC Resource

Using `endpointRef`:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: my-bmc
spec:
  endpointRef:
    name: my-bmc-endpoint
  bmcSecretRef:
    name: my-bmc-secret
  protocol:
    name: Redfish
    port: 8000
  consoleProtocol:
    name: SSH
    port: 22
```

Using inline `endpoint`:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: my-bmc-inline
spec:
  endpoint:
    macAddress: "00:1A:2B:3C:4D:5E"
    ip: "192.168.100.10"
  bmcSecretRef:
    name: my-bmc-secret
  protocol:
    name: Redfish
    port: 8000
  consoleProtocol:
    name: SSH
    port: 22
```

## Usage

The BMC CRD is essential for managing and monitoring BMC devices. It is used to:

- **Reconcile BMC State**: Continuously monitor the BMC's status and update its state.
- **Detect Managed Systems**: Identify all systems (servers) managed by the BMC and create corresponding [`Server`](servers.md) resources.
- **Automate Hardware Management**: Enable automated power control, firmware updates, and health monitoring of physical servers through the BMC.

## Reconciliation Process

The `BMCReconciler` is a controller that processes BMC resources to:

1. **Access BMC Device**: Uses the `endpointRef` or `endpoint`, along with `bmcSecretRef`, to establish a connection 
with the BMC using the specified `protocol`.

2. **Retrieve BMC Information**: Gathers details such as manufacturer, model, serial number, firmware version, and 
power state.

3. **Update BMCStatus**: Populates the `status` field of the BMC resource with the retrieved information.

4. **Detect Managed Systems**: Identifies all systems (servers) that the BMC manages.

5. **Create Server Resources**: For each detected system, the `BMCReconciler` creates a corresponding [`Server`](servers.md)
resource to represent the physical server.
