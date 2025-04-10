# BIOSSettings

`BIOSSettings` represents a BIOS Setting update operation for a physical server (compute system). It updates the bios settings on physical server's BIOS. 

## Key Points

- `BIOSSettings` maps a BIOS version and settings as map for a given server.
- Only one `BIOSSettings` can be active per `Server` at a time. 
- When the required version does not match to that of physical server's BIOS Version `BIOSSettings` waits for the upgrade of the version, before applying the required settings (if changes are present). 
- When bios version match and required settings does not match with the physical server's BIOS settings, `BIOSSettings` applyies the required settings.
- If the requested settings update requires a reboot of the server, the `BIOSSettings` waits for the `server` to be in `maintenance` before applying the setting.
    - `ServerMaintenance` resource is used to transistion the `server` to `maintenance` state.
    - `ServerMaintenance` Can be provided using the Spec or `BIOSSettings` requests the `server` to be transistioned to maintenance state by creating a new `ServerMaintenance`
        - `policy` used by `ServerMaintenance` is to be provided through Spec `ServerMaintenancePolicyTemplate` in `BIOSSettings`
- Once the Settings has been applied, and server rebooted if required. the `BIOSSettings` verifies the setting has been configured correctly.
    - the `BIOSSettings` moves to `SyncSettingsCompleted` state once the bios setting are synced. 
    - any subsequent changes will move the `BIOSSettings` out of `SyncSettingsCompleted` to check and apply settings if required.
- If the `BIOSSettings` fails to apply the bios setting. The `BIOSSettings` moves to `Failed` state until Manually moved out of this state. 

## Workflow


1. A separate operator (e.g., `biosSettingsSet`) or user creates a `BIOSSettings` resource referencing a 
   specific `Server`.
2. Provided settings are checked against the current BIOS setting.
3. If settings are same as on the server, the state is moved to `SyncSettingsCompleted` (even if the version does not match)
4.  If the settings needs update, `BIOSSettings` check the version of BIOS and if required version does not match, it waits for the bios version to reach the spec version.
5. `BIOSSettings` checks if the required setting update needs physical server reboot. 
6. If reboot is needed and `ServerMaintenance` is not provided already. it requests for one and waits for the `server` to enter `Maintenance` state.
7. Setting update process is started and the server is rebooted if required. 
8. `BIOSSettings` verfiy the setting has been applied and trasistions the state to synced. removes the `ServerMaintenance` resource if created by self.  
9. Any further update to the `BIOSSettings` Spec will restart the process. 

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BIOSSettings
metadata:
  labels:
    app.kubernetes.io/name: metal-operator
    app.kubernetes.io/managed-by: kustomize
  name: biossettings-sample
spec:
  serverRef:
    name: endpoint-sample-system-0
  biosSettings:
    version: 2.10.3
    settings:
      PxeDev1EnDis: Disable
      PxeDev2EnDis: Enabled
      OtherSettings: "123"
      someother: Disabled
  ServerMaintenancePolicyTemplate: OwnerApproval

```
