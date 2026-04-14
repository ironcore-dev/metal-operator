---
name: Bug report
about: Report a bug in the metal-operator
title: ''
labels: bug
assignees: ''
---

**Which component is affected?**
<!-- Which resource(s) or controller(s) does this bug relate to? e.g. Server, ServerClaim, ServerBootConfiguration, BMC, Endpoint, ServerMaintenance, BIOSSettings, etc. -->

**Describe the bug**
A clear and concise description of what the bug is.

**To reproduce**
Steps to reproduce the behavior:
1. Apply the following resource(s): ...
2. Observe controller behavior / resource status ...
3. See error

<details>
<summary>Resource definitions used</summary>

```yaml
# Paste relevant CRs here (sanitize any sensitive BMC credentials)
```
</details>

**Expected behavior**
A clear and concise description of what you expected to happen.

**Actual behavior**
What happened instead. Include any error messages from the controller logs.

<details>
<summary>Controller logs</summary>

```
# Paste relevant controller-manager logs here
# kubectl logs -n metal-operator-system deployment/metal-operator-controller-manager -c manager
```
</details>

**Environment**
- metal-operator version/commit:
- Kubernetes version (`kubectl version`):
- Deployment method (Helm / kustomize / local `make run`):
- Hardware / BMC type (if relevant):

**Additional context**
Add any other context about the problem here, such as relevant `kubectl describe` output for the affected resources.
