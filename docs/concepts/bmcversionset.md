# BMCVersionSet

`BMCVersionSet` represents a Set of `BMCVersion` to perform operation for all selected physical BMC through labels. It updates the BMC Version on all selected physical server's BMC through `BMCVersion`. 

## Key Points

- `BMCVersionSet` uses label selector to select the `BMC` to create `BMCVersion` for.
- `BMCVersionSet` creates `BMCVersion` for each BMC which matches the label.
    - Only one `BMCVersion` can be active per `BMC` at a time. 
- `BMCVersionSet` monitors changes to `BMC` resource and creates/deletes `BMCVersion`

## Workflow

1. `BMCVersionSet` filters `BMC` matching the provided label
2. `BMCVersionSet` creates `BMCVersion` CRD for each `BMC` selected
3. `BMCVersionSet` monitors the created `BMCVersion` and updates the status
4. `BMCVersionSet` creates or deletes `BMCVersion` based on the changes to `BMC` CRD.

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCVersionSet
metadata:
  name: bmcversionset-sample
spec:
  bmcVersionTemplate:
    version: "U59 v2.34 (10/04/2024)"
    image:
      URI: "https://foo-2.34_10_04_2024.signed.flash"
      transferProtocol: "HTTPS"
    updatePolicy: Normal
    serverMaintenancePolicy: OwnerApproval
  BMCSelector:
    matchLabels: 
      manufacturer: "dell"
```
