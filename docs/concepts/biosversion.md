# BIOSVersion

`BIOSVersion` represents a BIOS Version upgrade operation for a physical server (compute system). It updates the bios Version on physical server's BIOS. 

## Key Points

- `BIOSVersion` maps a BIOS version required for a given server's BIOS.
    - `BIOSVersion` Spec contains the required details to upgrade the BIOS to required version.
- Only one `BIOSVersion` can be active per `Server` at a time. 
- `BIOSVersion` starts the version upgrade of the BIOS using redfish `SimpleUpgrade` API.
- `BIOSVersion` handles reboots of server using `ServerMaintenance` resource.
- Once`BIOSVersion` moves to `Failed` state, It stays in this state unless Manually moved out of this state. 

## Workflow

1. A separate operator (e.g., `biosVersionSet`) or user creates a `BIOSVersion` resource referencing a 
   specific `Server`.
2. Provided BIOS Version is checked against the current BIOS version.
3. If version is same as on the server's BIOS, the state is moved to `Completed`.
4. If the version needs upgrade, `BIOSVersion` checks the current version of BIOS and if required version is lower than the requested, `BIOSVersion` moves the state to `Failed`
5. If `ServerMaintenance` is not provided already. it requests for one and waits for the `server` to enter `Maintenance` state.
    - `policy` used by `ServerMaintenance` is to be provided through Spec `ServerMaintenancePolicy` in `BIOSVersion`
6. `BIOSVersion` issues the bios upgrade using redfish `SimpleUpgrade` API. and monitors the `upgrade task` created by the API.
7. the `BIOSVersion` moves to `Failed` state:
    - If `SimpleUpgade` is issued but unable to get the task to monitor the progress of bios upgrade
    - If the `upgrade task` created by SimpleUpgade fails and does not reach completed state.
    - If the bios version requested is lower than that of the current bios version
8. `BIOSVersion` moves to reboot the server once the `upgrade task` has been completed. 
9. `BIOSVersion` verfiy the bios version post reboot, removes the `ServerMaintenance` resource if created by self. and transistion to `Completed` state
9. Any further update to the `BIOSVersion` Spec will restart the process. 

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BIOSVersion
metadata:
  name: biosversion-sample
spec:
  version: 2.10.3
  image:
    URI:  "http://foo.com/dell-bios-2.10.3.bin"
    transferProtocol: "http"
    imageSecretRef:
      name: sample-secret
  forceUpdate: false
  serverRef:
    name: endpoint-sample-system-0
  serverMaintenancePolicy: OwnerApproval
```
