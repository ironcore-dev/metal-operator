# BMCVersion

`BMCVersion` represents a BMC Version upgrade operation for a physical server's Manager. It updates the BMC Version on physical server's BMC. 

## Key Points

- `BMCVersion` maps a BMC version required for a given server's BMC.
    - `BMCVersion` Spec contains the required details to upgrade the BMC to required version.
- Only one `BMCVersion` can be active per `BMC` at a time. 
- `BMCVersion` starts the version upgrade of the BMC using redfish `SimpleUpgrade` API.
- `BMCVersion` handles reboots of BMC.
- `BMCVersion` requests for `Maintenance` if `ServerMaintenancePolicy` is set to "OwnerApproval".
- Once`BMCVersion` moves to `Failed` state, It stays in this state unless Manually moved out of this state. 

## Workflow

1. A separate operator (e.g., `bmcVersionSet`) or user creates a `BMCVersion` resource referencing a specific `BMC`.
2. Provided settings are checked against the current BMC version.
3. If version is same as on the server's BMC, the state is moved to `Completed`.
5. If "OwnerApproval" `ServerMaintenancePolicy` type is requested and `ServerMaintenance` is not provided already. It requests one per `server` managed by `BMC` and waits for all the `server` to enter `Maintenance` state.
6. `BMCVersion` issues the BMC upgrade using redfish "SimpleUpgrade" API. and monitors the `upgrade task` created by the API.
7. `BMCVersion` moves to `Failed` state:
    - If `SimpleUpgade` is issued but unable to get the task to monitor the progress of BMC upgrade
    - If the `upgrade task` created by SimpleUpgade fails and does not reach completed state.
    - If the BMC version requested is lower than that of the current BMC version
8. `BMCVersion` moves to reboot the BMC once the `upgrade task` has been completed. 
9. `BMCVersion` verfiy the BMC version post reboot, removes the `ServerMaintenance` resource if created by self. and transistion to `Completed` state
9. Any further update to the `BMCVersion` Spec will restart the process. 

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCVersion
metadata:
  name: biosversion-sample
spec:
  version: 2.10.3
  image:
    URI: "http://foo.com/dell-idrac-bmc-2.10.3.bin"
    transferProtocol: "http"
    imageSecretRef:
      name: sample-secret
  updatePolicy: Normal
  BMCRef:
    name: BMC-sample
  serverMaintenancePolicy: Enforced
```
