# BMCSettings

`BMCSettings` represents a BMC Setting update operation for a physical server's BMC (compute system). It updates the BMC settings on physical server's BMC. 

## Key Points

- `BMCSettings` maps a BMC version and settings as map for a given server.
- Only one `BMCSettings` can be active per `BMC` at a time. 
- `BMCSettings` related changes are applied once the BMC version matches with the physical server's BMC version.
- `BMCSettings` handles reboots of BMC
- `BMCSettings` requests for `Maintenance`, `ServerMaintenancePolicy` used for maintenance type
- Once`BMCSettings` moves to `Failed` state, It stays in this state unless Manually moved out of this state. 

## Workflow

1. A separate operator (e.g., `bmcSettingsSet`) or user creates a `BMCSettings` resource referencing a specific `BMC` 
2. Provided settings are checked against the current BMC setting.
3. If settings are same as on the server, the state is moved to `Applied` (even if the version does not match)
4. If the settings needs update, `BMCSettings` check the version of BMC and if required version does not match, it waits for the BMC version to reach the spec version.
5. If `ServerMaintenance` is not provided already. it requests one per `server` managed by `BMC` and waits for all the `server` to enter `Maintenance` state.
6. Setting update process is started and the physical server's BMC is rebooted if required. 
7. `BMCSettings` verfiy the setting has been applied and trasistions the state to `Applied`. removes all the `ServerMaintenance` resource if created by self.
8. Any further update to the `BMCSettings` Spec will restart the process. 
9. If the `BMCSettings` fails to apply the bmc setting. The `BMCSettings` moves to `Failed` state until Manually moved out of this state. 

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCSettings
metadata:
  name: bmcsettings-sample
spec:
  bmcRef:
    name: sample-bmc
  version: 2.10.3
  settings:
    otherSettings: "123"
    someother: Disabled
  serverMaintenancePolicy: OwnerApproval
```
