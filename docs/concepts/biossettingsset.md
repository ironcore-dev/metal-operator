# BIOSSettingsSet

`BIOSSettingsSet` represents a Set of `BIOSSettings` to perform operation for all selected physical server through labels. It updates the bios Settings on all selected physical server's BIOS through `BIOSSettings`. 

## Key Points

- `BIOSSettingsSet` uses label selector to select the `Servers` to create `BIOSSettings` for.
- `BIOSSettingsSet` creates `BIOSSettings` for each server which matches the label.
    - Only one `BIOSSettings` can be active per `Server` at a time. 
- `BIOSSettingsSet` monitors changes to `Server` resource and creates/deletes `BIOSSettings`

## Workflow

1. `BIOSSettingsSet` filters `Servers` matching the provided label
2. `BIOSSettingsSet` creates `BIOSSettings` CRD for each `Server` selected
3. `BIOSSettingsSet` monitors the created `BIOSSettings` and updates the status
4. `BIOSSettingsSet` creates or deletes `BIOSSettings` based on the changes to `Server` CRD>

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BIOSSettingsSet
metadata:
  name: biossettingsset-sample
spec:
  biosVersionTemplate:
    version: "U59 v2.34 (10/04/2024)"
    serverMaintenancePolicy: OwnerApproval
    settings:
      foo: bar
  ServerSelector:
    matchLabels: 
      manufacturer: "dell"
```
