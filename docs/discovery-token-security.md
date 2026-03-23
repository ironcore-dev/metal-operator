# Discovery Token Security

## Overview

The metal-operator uses **HMAC-SHA256 signed discovery tokens** to authenticate servers during the discovery process. This prevents unauthorized systems from submitting fake discovery data to the registry.

## Design

### Token Format

Discovery tokens use a JWT-like design with HMAC-SHA256 signatures:

```
token = base64url(systemUUID||timestamp||HMAC-SHA256(systemUUID||timestamp, secret))
```

**Components:**

- **systemUUID**: The server's unique identifier from SMBIOS
- **timestamp**: Unix timestamp (seconds) when token was generated
- **signature**: HMAC-SHA256 signature of the payload
- **secret**: 32-byte (256-bit) shared secret between controller and registry

### Security Properties

✅ **Authentication**: Only the controller can generate valid tokens
✅ **Integrity**: Tokens cannot be tampered with
✅ **Binding**: Each token is bound to a specific systemUUID
✅ **Freshness**: Tokens expire after 1 hour to prevent replay
✅ **Timing-safe**: HMAC uses constant-time comparison

### Threat Model

**Protected Against:**

- ❌ Rogue systems submitting fake discovery data
- ❌ UUID spoofing (attacker claiming another server's identity)
- ❌ Token tampering or forgery
- ❌ Replay attacks (tokens expire after 1 hour)
- ❌ Timing attacks (constant-time HMAC comparison)

**NOT Protected Against** (out of scope):

- ⚠️ MITM attacks (requires TLS - separate concern)
- ⚠️ Token leakage via logs (tokens not logged)
- ⚠️ Compromised signing secret (rotate if compromised)

## Architecture

### Token Lifecycle

```
┌──────────────┐                  ┌──────────┐                  ┌──────────┐
│  Controller  │                  │ Registry │                  │  Server  │
└──────┬───────┘                  └────┬─────┘                  └────┬─────┘
       │                               │                             │
       │ 1. Generate Signing Secret    │                             │
       │    (K8s Secret)               │                             │
       │◄──────────────────────────────┤                             │
       │                               │                             │
       │ 2. Generate Signed Token      │                             │
       │    for systemUUID             │                             │
       │──────────────────────────────►│                             │
       │                               │                             │
       │ 3. Pass token in ignition     │                             │
       │───────────────────────────────┼─────────────────────────────►
       │                               │                             │
       │                               │ 4. Server boots with token  │
       │                               │◄─────────────────────────────
       │                               │                             │
       │                               │ 5. Verify HMAC signature    │
       │                               │    Extract systemUUID       │
       │                               │    Check expiry             │
       │                               │                             │
       │                               │ 6. Accept/Reject data       │
       │                               ├─────────────────────────────►
```

### Components

**1. Controller (`internal/controller/server_controller.go`)**

- Generates or retrieves signing secret from K8s Secret
- Creates signed tokens when generating boot configurations
- Passes tokens to metalprobe via ignition flags

**2. Registry (`internal/registry/server.go`)**

- Loads signing secret on startup
- Verifies token signatures on `/register` and `/bootstate` endpoints
- Extracts and validates systemUUID from tokens
- Rejects requests with invalid/expired tokens

**3. Metalprobe (`cmd/metalprobe/main.go`)**

- Receives discovery token via `--discovery-token` flag
- Includes token in all HTTP requests to registry
- Handles authentication errors gracefully

**4. Token Library (`internal/token/token.go`)**

- `GenerateSigningSecret()`: Creates 32-byte cryptographic secret
- `GenerateSignedDiscoveryToken()`: Signs systemUUID with HMAC-SHA256
- `VerifySignedDiscoveryToken()`: Verifies signature and extracts payload

## Implementation Details

### Signing Secret Management

The signing secret is stored in a Kubernetes Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: discovery-token-signing-secret
  namespace: metal-operator-system
type: Opaque
data:
  signing-key: <base64-encoded-32-byte-secret>
```

**Secret Lifecycle:**

- Created automatically by controller on first boot config generation
- Shared between controller and registry
- Persists across restarts (not ephemeral)
- Should be backed up with cluster state

**Secret Rotation:**
To rotate the signing secret:

1. Delete the existing secret: `kubectl delete secret discovery-token-signing-secret -n metal-operator-system`
2. Restart the controller pod (regenerates secret)
3. Restart the registry pod (loads new secret)
4. Note: In-flight discovery sessions will fail (servers will retry)

### Token Expiry

Tokens include a timestamp and are validated for freshness:

```go
// Token expires after 1 hour
maxAge := 3600 seconds

// Allow 5 minutes clock skew
clockSkew := 300 seconds

valid := (currentTime - tokenTime) <= maxAge &&
         (currentTime - tokenTime) >= -clockSkew
```

**Why 1 hour?**

- Discovery typically completes in minutes
- Allows time for troubleshooting
- Prevents long-term token reuse
- Balances security vs. operational flexibility

### Error Handling

**Registry Responses:**

- `201 Created`: Token valid, data accepted
- `401 Unauthorized`: Token missing, invalid, or expired
- `500 Internal Server Error`: Signing secret not loaded

**Metalprobe Behavior:**

- Retries on transient errors (500)
- Does NOT retry on authentication errors (401)
- Logs errors for debugging

**Controller Behavior:**

- Fails reconciliation if token generation fails
- Retries on next reconciliation loop
- Logs errors with context

## Comparison with Alternatives

### vs. OpenStack Ironic

**Similarities:**

- Both use random tokens for discovery authentication
- Both bind tokens to specific machines
- Both use timing-safe comparison

**Improvements in metal-operator:**

- ✅ Signed tokens (HMAC) vs. random tokens (lookup required)
- ✅ Stateless verification (no registry storage needed)
- ✅ Automatic expiry (timestamp-based)
- ✅ K8s Secret storage vs. database field

### vs. mTLS (Mutual TLS)

**Why not mTLS?**

- Requires PKI infrastructure
- Complex certificate provisioning for ephemeral systems
- Boot environment may not support TLS client certs
- HMAC tokens are simpler and sufficient for discovery

**When to use TLS:**

- Add TLS for MITM protection (controller ↔ registry ↔ probe)
- Use cert-manager for easy certificate management
- Tokens + TLS = defense in depth

### vs. IP Allowlist

**Why not IP allowlist?**

- DHCP assigns dynamic IPs during discovery
- NAT/proxies obscure source IPs
- Doesn't prevent UUID spoofing from allowed IPs
- Tokens provide better granularity

## Security Considerations

### Token Leakage Prevention

**DO:**

- ✅ Pass tokens via ignition (secure channel)
- ✅ Use structured logging (tokens not in log fields)
- ✅ Store in K8s Secrets (not ConfigMaps)

**DON'T:**

- ❌ Log token values (even at debug level)
- ❌ Expose tokens in API responses
- ❌ Store tokens in Git or documentation

### Deployment Security

**Recommendations:**

1. **Enable TLS**: Add TLS termination at registry for MITM protection
2. **Network Policies**: Restrict registry access to authorized pods
3. **RBAC**: Limit access to signing secret to controller/registry ServiceAccounts
4. **Audit Logging**: Enable K8s audit logs for secret access
5. **Backup**: Include signing secret in cluster backup/restore procedures

### Monitoring

**Key Metrics to Monitor:**

- Token validation failures (registry logs)
- Signing secret access (K8s audit logs)
- Discovery timeouts (may indicate token issues)
- 401 Unauthorized responses (registry metrics)

**Alerts to Configure:**

- High rate of token validation failures
- Signing secret missing/invalid on registry startup
- Repeated 401 errors from specific servers

## Troubleshooting

### Problem: "401 Unauthorized" on `/register`

**Symptoms:**

- Metalprobe logs: "authentication failed with registry"
- Registry logs: "Rejected request with invalid discovery token"

**Causes:**

1. Token expired (>1 hour old)
2. Signing secret mismatch between controller and registry
3. Token tampered or corrupted
4. Clock skew between controller and registry

**Resolution:**

```bash
# Check if signing secret exists
kubectl get secret discovery-token-signing-secret -n metal-operator-system

# Verify registry loaded the secret
kubectl logs -n metal-operator-system deployment/metal-operator-registry | grep "signing secret"

# Check controller logs for token generation
kubectl logs -n metal-operator-system deployment/metal-operator-controller-manager \
  | grep "Generated signed discovery token"

# If mismatch, restart both pods to reload secret
kubectl rollout restart deployment/metal-operator-controller-manager -n metal-operator-system
kubectl rollout restart deployment/metal-operator-registry -n metal-operator-system
```

### Problem: "Signing secret not loaded"

**Symptoms:**

- Registry logs: "Signing secret not loaded, cannot validate tokens"
- All discovery attempts fail with 500 errors

**Causes:**

1. Secret doesn't exist yet (first boot)
2. Registry started before controller created secret
3. Wrong namespace or secret name
4. RBAC prevents registry from reading secret

**Resolution:**

```bash
# Force controller to create secret
kubectl delete serverbootconfiguration <name> -n <namespace>
# (Controller will recreate it, generating secret in the process)

# Or manually create the secret
kubectl create secret generic discovery-token-signing-secret \
  --from-literal=signing-key=$(openssl rand -base64 32) \
  -n metal-operator-system

# Verify registry can read it
kubectl auth can-i get secrets/discovery-token-signing-secret \
  --as=system:serviceaccount:metal-operator-system:metal-operator-registry
```

### Problem: Discovery timeout

**Symptoms:**

- Server stuck in Discovery state
- No data appears in registry
- Metalprobe logs show repeated failures

**Check:**

```bash
# 1. Verify metalprobe received token
kubectl logs <server-pod> | grep "discovery-token"
# (Token value should NOT appear in logs)

# 2. Check registry validation
kubectl logs -n metal-operator-system deployment/metal-operator-registry \
  | grep -E "Validated|Rejected"

# 3. Verify system UUID matches
kubectl get server <name> -o jsonpath='{.spec.systemUUID}'
# Should match the UUID in registry logs

# 4. Check token expiry
# If discovery takes >1 hour, increase expiry in internal/token/token.go
```

## Testing

### Unit Tests

Token generation and verification:

```bash
cd internal/token
go test -v
```

### Integration Tests

Full discovery flow with signed tokens:

```bash
make test
```

### Manual Testing

1. **Generate a test token:**

```go
secret, _ := token.GenerateSigningSecret()
token, _ := token.GenerateSignedDiscoveryToken(secret, "test-uuid-123")
fmt.Println("Token:", token)
```

2. **Verify the token:**

```go
uuid, timestamp, valid, _ := token.VerifySignedDiscoveryToken(secret, token)
fmt.Printf("UUID: %s, Valid: %t, Age: %ds\n", uuid, valid, time.Now().Unix()-timestamp)
```

3. **Test with metalprobe:**

```bash
metalprobe --server-uuid=test-uuid \
           --registry-url=http://localhost:8080 \
           --discovery-token=<generated-token>
```

## References

- [OpenStack Ironic Agent Token Design](https://docs.openstack.org/ironic/latest/admin/security.html#agent-token)
- [HMAC-SHA256 (RFC 2104)](https://www.rfc-editor.org/rfc/rfc2104)
- [Kubernetes Secrets Best Practices](https://kubernetes.io/docs/concepts/security/secrets-good-practices/)
- [Issue #749: Secure Discovery Boot Data](https://github.com/ironcore-dev/metal-operator/issues/749)

## Future Enhancements

### Potential Improvements:

1. **Short-lived tokens**: Generate new tokens on each registration attempt
2. **Token rotation**: Periodically rotate signing secret without downtime
3. **Multi-secret support**: Support multiple signing secrets for graceful rotation
4. **Audit trail**: Log all token validations to audit log
5. **Rate limiting**: Prevent brute-force token guessing attempts
6. **Token revocation**: Explicit token revocation API for compromised tokens

### Not Planned:

- JWT standard format (overhead not needed for internal tokens)
- Public key crypto (HMAC sufficient for shared-secret scenario)
- Token refresh (discovery is short-lived, not needed)
