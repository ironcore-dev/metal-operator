# Endpoints

The Endpoint Custom Resource Definition (CRD) is a Kubernetes resource used to represent and identify devices or 
entities within an out-of-band (OOB) network. It serves as a means to catalog and manage devices such as Baseboard 
Management Controllers (BMCs) by capturing their unique identifiers, specifically the MAC address and IP address. 
The `EndpointReconciler` leverages this information to determine the nature of the device, its vendor, and any initial 
credentials required for further interactions.

## Example Endpoint Resource

An example of how to define an Endpoint resource:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: Endpoint
metadata:
  name: device-12345
spec:
  macAddress: "00:1A:2B:3C:4D:5E"
  ip: "192.168.100.10"
```

## MAC Prefix Database and EndpointReconciler Configuration

The `EndpointReconciler` can be configured with a MAC Prefix Database to determine the characteristics of devices based
on their MAC addresses. This database maps MAC address prefixes to device information such as the manufacturer, 
protocol, port, type, default credentials, and console settings.

### Configuration

The MAC Prefix Database is typically configured using a YAML file, which is passed to the `metal-operator` using the 
`--mac-prefixes-file` flag. This file contains mappings of MAC address prefixes to device specifications.

Example YAML Configuration:

```yaml
macPrefixes:
  - macPrefix: "23"
    manufacturer: "Foo"
    protocol: "Redfish"
    port: 8000
    type: "bmc"
    defaultCredentials:
      - username: "foo"
        password: "bar"
    console:
      type: "ssh"
      port: 22
```

**Key Fields**:

- macPrefix (`string`): The prefix of the MAC address used to identify the device manufacturer or type.
- manufacturer (`string`): The name of the device manufacturer.
- protocol (`string`): The communication protocol used (e.g., `Redfish`).
- port (`int`): The network port used for communication.
- type (`string`): The type of device (e.g., `bmc`).
- defaultCredentials (`list`): A list of default credentials for accessing the device.
    - username (`string`): The default username.
    - password (`string`): The default password.
- console (`dict`): Console access configuration.
    - type (string): The console protocol (e.g., ssh).
    - port (int): The port used for console access.

### Using `--mac-prefixes-file` Flag

The `metal-operator` accepts the `--mac-prefixes-file` flag to specify the path to the MAC Prefix Database YAML file:

```shell
metal-operator --mac-prefixes-file /path/to/mac_prefixes.yaml
```

## Reconciliation Process

1. **MAC Address Matching**: When the `EndpointReconciler` processes an `Endpoint` resource, it extracts the
`macAddress` from the `spec`.

2. **Prefix Lookup**: It compares the MAC address prefix against the entries in the MAC Prefix Database.

3. **Device Identification**: If a matching prefix is found, the device is identified with the associated manufacturer, 
type, and protocol.

4. **Credential Assignment**: The default credentials specified in the database are used for initial authentication with 
the device.

5. **BMC and BMCSecret Creation**: When the `EndpointReconciler` detects that the device is a Baseboard Management
Controller (`type: "bmc"`), it automatically creates a [`BMC`](bmcs.md) and a [`BMCSecret`](bmcsecrets.md)
object using the data from the MAC Prefix Database. These objects are used to manage and authenticate with the BMC device.

6. **Configuration Application**: Additional settings such as console access and communication ports are applied based 
on the database entries.
