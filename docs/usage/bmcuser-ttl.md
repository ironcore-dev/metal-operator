# Temporary BMC Users with TTL

BMCUser resources support automatic expiration for temporary users. This is useful for debugging, maintenance windows, or temporary access scenarios.

## Overview

You can create temporary BMC users that are automatically deleted after a specified duration (TTL) or at a specific time (ExpiresAt). The operator handles both the Kubernetes object deletion and BMC account cleanup.

## Configuration Options

### Time-To-Live (TTL)

Specify a duration from creation:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCUser
metadata:
  name: debug-user
spec:
  userName: debugger
  roleID: "Administrator"
  ttl: 8h0m0s  # User expires 8 hours after creation
  bmcRef:
    name: my-bmc
```

### Absolute Expiration (ExpiresAt)

Specify an exact timestamp:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCUser
metadata:
  name: maintenance-user
spec:
  userName: maintenance
  roleID: "Operator"
  expiresAt: "2026-04-11T02:00:00Z"  # User expires at this time
  bmcRef:
    name: my-bmc
```

**Note:** TTL and ExpiresAt are mutually exclusive - only one can be set.

## Expiration Lifecycle

1. **Creation**: User is created on BMC with credentials
2. **Active Period**: User is fully functional
3. **Warning Period**: Condition changes to "ExpiringSoon" (last hour or 10% of TTL)
4. **Expiration**: User is automatically deleted from both BMC and Kubernetes

## Monitoring Expiration

### Check Expiration Time

```bash
kubectl get bmcuser -o wide
kubectl get bmcuser my-user -o jsonpath='{.status.expiresAt}'
```

### Check Expiration Status

```bash
kubectl describe bmcuser my-user
```

Look for the `Active` condition:
- `Status: True, Reason: Active` - User is active
- `Status: True, Reason: ExpiringSoon` - User will expire soon
- `Status: False, Reason: Expired` - User has expired (being deleted)

## Important Notes

1. **Immutable Expiration**: Once calculated, the expiration time cannot be changed by updating TTL or ExpiresAt
2. **Automatic Cleanup**: Both the Kubernetes object and BMC account are deleted
3. **Password Rotation**: Works normally for temporary users during their lifetime
4. **Permanent Users**: Users without TTL or ExpiresAt are never automatically deleted

## Use Cases

### Debugging Session

```yaml
# Create admin user for 4-hour debugging session
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCUser
metadata:
  name: debug-session-1
spec:
  userName: debug-admin
  roleID: "Administrator"
  ttl: 4h
  bmcRef:
    name: problematic-server
```

### Maintenance Window

```yaml
# User expires at end of maintenance window
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCUser
metadata:
  name: maintenance-2026-04-10
spec:
  userName: maintenance-tech
  roleID: "Operator"
  expiresAt: "2026-04-11T06:00:00Z"
  bmcRef:
    name: datacenter-bmc
```

### Temporary Vendor Access

```yaml
# Give vendor access for 24 hours
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCUser
metadata:
  name: vendor-support
spec:
  userName: vendor-tech
  roleID: "ReadOnly"
  ttl: 24h
  description: "Vendor support access"
  bmcRef:
    name: bmc-needing-support
```

## kubectl Output Examples

### List Users with Expiration

```bash
$ kubectl get bmcuser -o wide

NAME                        ID   USERNAME          ROLEID         EXPIRESAT               LASTROTATION   AGE
bmcuser-permanent           5    admin             Administrator  <none>                  2h ago         3d
bmcuser-debug-temporary     6    debug-admin       Administrator  2026-04-10T23:00:00Z    1h ago         7h
bmcuser-maintenance-window  7    maintenance-user  Operator       2026-04-11T02:00:00Z    <none>         4h
```

### Check User Status

```bash
$ kubectl describe bmcuser bmcuser-debug-temporary

...
Status:
  Conditions:
    Last Transition Time:  2026-04-10T22:30:00Z
    Message:              User will expire in 30m at 2026-04-10T23:00:00Z
    Reason:               ExpiringSoon
    Status:               True
    Type:                 Active
  Expires At:             2026-04-10T23:00:00Z
  ID:                     6
  ...
```

## CLI Workflow

```bash
# Create temporary user for 4 hours
cat <<EOF | kubectl apply -f -
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCUser
metadata:
  name: quick-debug
spec:
  userName: debuguser
  roleID: "Administrator"
  ttl: 4h
  bmcRef:
    name: my-bmc
EOF

# Check when user expires
kubectl get bmcuser quick-debug -o jsonpath='{.status.expiresAt}'

# User is automatically cleaned up after 4 hours
# No manual deletion needed!
```
