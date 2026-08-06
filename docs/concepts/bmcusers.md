# BMCUser

> **Deprecation Notice:** The `BMCUser` resource is deprecated and will be removed in a future release.

`BMCUser` manages a user account on the BMC of a single `BMC` resource, including credential management and password rotation.

## What It Does

- Targets one BMC through `spec.bmcRef`.
- Creates or updates the BMC user account with `spec.userName` and `spec.roleID`.
- Uses credentials from an existing `BMCSecret` (`spec.bmcSecretRef`) or generates a secure password and stores it in a new `BMCSecret`.
- Rotates the password according to `spec.rotationPeriod` by generating a new `BMCSecret` and updating the account on the BMC. Rotation can also be forced by setting the annotation `metal.ironcore.dev/operation: rotate-credentials` or when the BMC-reported password expiration has passed.
- Removes the account from the BMC on deletion (finalizer-based cleanup).

## Spec Reference

| Field | Required | Description |
|---|---|---|
| `spec.userName` | Yes | Username of the BMC user. |
| `spec.roleID` | Yes | Role ID to assign to the user. |
| `spec.description` | No | Description of the BMC user. |
| `spec.bmcRef.name` | No | Target BMC object. If not set, the resource is not reconciled. |
| `spec.bmcSecretRef.name` | No | `BMCSecret` containing the credentials. If not set, the operator generates a password and creates one. |
| `spec.rotationPeriod` | No | How often the password is rotated. If not set, the password is not rotated. |

## Status Fields In Detail

| Field | What it means | How to use it for debugging |
|---|---|---|
| `status.id` | Identifier of the user account in the BMC system. | Empty means the account has not been created on the BMC yet. |
| `status.effectiveBMCSecretRef` | The `BMCSecret` currently in use, which may differ from `spec.bmcSecretRef` if the operator generated the password. | Points at the secret holding the working credentials. |
| `status.lastRotation` | Timestamp of the last password rotation. | Check whether rotation is running as expected. |
| `status.passwordExpiration` | Timestamp when the current password will expire. | Next scheduled rotation point. |

## Example

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCUser
metadata:
  name: bmcuser-sample
spec:
  userName: operator
  roleID: "Administrator"
  bmcRef:
    name: bmc-sample
  rotationPeriod: 720h
```
