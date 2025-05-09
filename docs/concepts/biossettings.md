# BIOSSettings

`BIOSSettings` represents a BIOS Setting update operation for a physical server (compute system). It updates the bios settings on physical server's BIOS. 

## Key Points

- `BIOSSettings` maps a BIOS version and settings as map for a given server.
- Only one `BIOSSettings` can be active per `Server` at a time.
- `BIOSSettings` related changes are applied once the bios version matches with the physical server's bios.
- `BIOSSettings` handles reboots of server (if required) using `ServerMaintenance` resource 
- Once`BIOSSettings` moves to `Failed` state, It stays in this state unless Manually moved out of this state. 

## Workflow

1. A separate operator (e.g., `biosSettingsSet`) or user creates a `BIOSSettings` resource referencing a 
   specific `Server`.
2. Provided settings are checked against the current BIOS setting.
3. If settings are same as on the server, the state is moved to `Applied` (even if the version does not match)
4. If the settings needs update, `BIOSSettings` check the version of BIOS and if required version does not match, it waits for the bios version to reach the spec version.
5. If `ServerMaintenance` is not provided already. it requests for one and waits for the `server` to enter `Maintenance` state.
    - `policy` used by `ServerMaintenance` is to be provided through Spec `ServerMaintenancePolicy` in `BIOSSettings`
6. `BIOSSettings` checks if the required setting update needs physical server reboot. 
7. Setting update process is started and the server is rebooted if required. 
8. `BIOSSettings` verfiy the setting has been applied and trasistions the state to `Applied`. removes the `ServerMaintenance` resource if created by self.
9. Any further update to the `BIOSSettings` Spec will restart the process. 
10. If the `BIOSSettings` fails to apply the bios setting. The `BIOSSettings` moves to `Failed` state until Manually moved out of this state. 

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BIOSSettings
metadata:
  name: biossettings-sample
spec:
  serverRef:
    name: endpoint-sample-system-0
  version: 2.10.3
  settings:
    PxeDev1EnDis: Disable
    PxeDev2EnDis: Enabled
    OtherSettings: "123"
    someother: Disabled
ServerMaintenancePolicy: OwnerApproval

```
