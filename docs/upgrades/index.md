# Upgrades

This section covers upgrading the metal-operator between versions. Always check the
[release notes](https://github.com/ironcore-dev/metal-operator/releases) of the target version
before upgrading.

General rules:

- **Removed resources:** if a release removes a CRD, delete all custom resources of that kind
  *before* upgrading, while the old operator is still running. The old operator must process the
  finalizers (e.g. deleting user accounts on the BMC, transitioning servers out of the
  `Maintenance` state). Once the new operator runs, nothing is left to clean them up.
- **CRD cleanup:** with `kubectl apply`/Kustomize installs, removed CRDs are not deleted
  automatically. Delete them manually after the upgrade. The same applies to Helm installs when
  `crd.keep: true` (the chart default), since kept CRDs are never pruned on upgrade.

## Upgrade guides

- [Upgrading from v0.7 to v0.8](/upgrades/v0.7-to-v0.8)
