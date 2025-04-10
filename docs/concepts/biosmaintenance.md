# BiosMaintenance

`BiosMaintenance` represents a bios maintenance operation for a physical server (compute system). It updates the bios version and settings on physical server's BIOS. 

## Key Points

- `BiosMaintenance` maps a BIOS version and settings as map for a given server.
- Only one `BiosMaintenance` can be active per `Server` at a time. 
    - if multiple `BiosMaintenance` resources are created for a server, latest BIOS version one will reference the server. others will be de-referenced.
- When the required version does not match to that of physical server's BIOS Version `BiosMaintenance` upgrades the version, before applying the required settings. 
- When bios version match and required settings does not match with the physical server's BIOS settings, `BiosMaintenance` applyies the required settings.
- If the requested settings update or version upgrade requires a reboot of the server, the `BiosMaintenance` waits for the `server` to be in `maintenance` before applying the setting or upgrading the version.
    - `ServerMaintenance` resource is used to transistion the `server` to `maintenance` state.
    - `ServerMaintenance` Can be provided using the Spec or `BiosMaintenance` requests the `server` to be transistioned to maintenance state by creating a new `ServerMaintenance`
        - `policy` used by `ServerMaintenance` is to be provided through Spec `ServerMaintenancePolicyTemplate` in `BiosMaintenance`
- Once the Settings has been applied, and server rebooted if required. the `BiosMaintenance` verifies the setting has been configured correctly.
    - the `BiosMaintenance` moves to `SyncSettingsCompleted` state once the bios setting are synced. 
    - any subsequent changes will move the `BiosMaintenance` out of `SyncSettingsCompleted` to check and apply settings if required.
- If the `BiosMaintenance` fails to apply the bios setting or version upgrade. The `BiosMaintenance` moves to `Failed` state until Manually moved out of this state. 

## Workflow

1. A separate operator (e.g., `biosMaintenanceSet`) or user creates a `BiosMaintenance` resource referencing a 
   specific `Server`.
2. `BiosMaintenance` check the version of BIOS and if required version is greater than current BIOS version, starts the version upgrade workflow.
3. If the `ServerMaintenance` is not provided through the spec (Optional). It creates the `ServerMaintenance` and waits for the `server` to enter `Maintenance` state.
4. Upgrade process is started once`server` is in `Maintenance` state and waits for the upgrade to complete and then moves to checking required settings of BIOS. 
5. Provided settings are checked against the current BIOS setting once the bios version matches with required. 
6. If the settings needs update, Starts the settings update workflow.
7. `BiosMaintenance` checks if the required setting update needs physical server reboot. 
8. If reboot is needed and `ServerMaintenance` is not created/provided already. it requests for one and waits for the `server` to enter `Maintenance` state.
9. Setting update process is started and the server is rebooted if required. 
10. `BiosMaintenance` verfiy the setting has been applied and trasistions the state to synced. removes the `ServerMaintenance` resource if created by self.  
11. Any further update to the `BiosMaintenance` Spec will restart the process. 

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BiosMaintenance
metadata:
  labels:
    app.kubernetes.io/name: metal-operator
    app.kubernetes.io/managed-by: kustomize
  name: biosmaintenance-sample
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
