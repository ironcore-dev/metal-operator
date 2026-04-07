# BMC Certificate Management

## Overview

The metal-operator can automatically install TLS certificates on BMCs (Baseboard Management Controllers) via the Redfish API. Certificate management is decoupled from certificate provisioning - you manage certificates using your preferred tool (cert-manager, external-secrets-operator, manual creation, etc.), and metal-operator installs them on the BMCs.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Certificate Management (Your Choice)                       │
│  - cert-manager                                             │
│  - external-secrets-operator                                 │
│  - Vault integration                                        │
│  - Manual creation                                           │
│  - Custom tooling                                            │
└──────────────────────┬──────────────────────────────────────┘
                       │ Creates/Updates
                       ▼
                ┌──────────────┐
                │ TLS Secret   │
                │ (k8s.io/tls) │
                └──────┬───────┘
                       │ Referenced by
                       ▼
                ┌──────────────┐
                │     BMC      │
                │  Resource    │
                └──────┬───────┘
                       │ Watches secret changes
                       ▼
           ┌───────────────────────┐
           │  metal-operator       │
           │  BMC Controller       │
           └───────┬───────────────┘
                   │ Installs certificate via Redfish
                   ▼
           ┌───────────────┐
           │  BMC Hardware │
           └───────────────┘
```

## Quick Start

### 1. Create a TLS Secret

You have several options:

#### Option A: Manual Creation

```bash
# Create certificate and private key (example with openssl)
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 365 -nodes

# Create Kubernetes secret
kubectl create secret tls bmc-tls-cert \
  --cert=cert.pem \
  --key=key.pem \
  --namespace=default
```

#### Option B: Using cert-manager

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: bmc-certificate
  namespace: default
spec:
  secretName: bmc-tls-cert  # Secret that will be created
  duration: 2160h  # 90 days
  renewBefore: 720h  # Renew 30 days before expiration
  
  commonName: bmc-server-01.example.com
  dnsNames:
    - bmc-server-01.example.com
    - bmc.internal.example.com
  ipAddresses:
    - 192.168.1.100
  
  issuerRef:
    name: my-issuer
    kind: Issuer
```

#### Option C: Using external-secrets-operator

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: bmc-external-secret
  namespace: default
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: bmc-tls-cert
    creationPolicy: Owner
    template:
      type: kubernetes.io/tls
  data:
    - secretKey: tls.crt
      remoteRef:
        key: bmc/certificates/server-01
        property: certificate
    - secretKey: tls.key
      remoteRef:
        key: bmc/certificates/server-01
        property: private-key
```

### 2. Create BMC Resource

Reference the TLS secret in your BMC resource:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: bmc-server-01
spec:
  # Network configuration
  access:
    ip: 192.168.1.100
    macAddress: "00:1A:2B:3C:4D:5E"
  
  # Authentication
  bmcSecretRef:
    name: bmc-credentials
  
  # Protocol
  protocol:
    name: Redfish
    port: 443
    scheme: https
  
  # TLS Certificate (NEW!)
  tlsSecretRef:
    name: bmc-tls-cert
    namespace: default  # Optional, defaults to operator namespace
```

### 3. Verify Installation

Check the BMC status to see if the certificate was installed:

```bash
kubectl get bmc bmc-server-01 -o yaml
```

Look for the `CertificateReady` condition:

```yaml
status:
  conditions:
    - type: CertificateReady
      status: "True"
      reason: Issued
      message: "Certificate installed successfully"
  certificateSecretRef:
    name: bmc-tls-cert
```

## How It Works

### Certificate Installation Flow

1. **Secret Watch**: The BMC controller watches for changes to `kubernetes.io/tls` type secrets
2. **Secret Change Detection**: When a secret referenced by a BMC is created or updated, the controller is notified
3. **Validation**: The controller validates the secret:
   - Must be type `kubernetes.io/tls`
   - Must contain `tls.crt` and `tls.key` keys
   - Certificate must be valid PEM format
4. **Installation Check**: The controller checks if installation is needed:
   - Compares certificate serial numbers
   - Checks expiration (30-day renewal buffer)
5. **Redfish Installation**: If needed, installs the certificate via BMC's Redfish API
6. **Status Update**: Updates BMC status with installation result

### Automatic Certificate Renewal

When using cert-manager or similar tools:

1. **cert-manager renews** the certificate (based on `renewBefore` setting)
2. **Secret is updated** with new certificate
3. **BMC controller detects** the secret change (via watch)
4. **Controller automatically installs** the new certificate on the BMC

This happens completely automatically - no manual intervention required!

## Configuration

### Namespace Handling

The `tlsSecretRef` supports cross-namespace references:

```yaml
spec:
  tlsSecretRef:
    name: bmc-tls-cert
    namespace: certificates  # Explicit namespace
```

If `namespace` is omitted, the operator's manager namespace is used (typically where the operator is deployed).

### Certificate Expiry Buffer

The controller maintains a 30-day renewal buffer. Certificates expiring within 30 days will be automatically reinstalled on the BMC, even if cert-manager hasn't renewed them yet.

## Monitoring

### Status Conditions

The BMC resource provides a `CertificateReady` condition with the following possible states:

| Status | Reason | Description |
|--------|--------|-------------|
| True | Issued | Certificate successfully installed on BMC |
| False | Pending | Certificate installation in progress |
| False | Failed | Certificate installation failed (check message for details) |

### Checking Certificate Status

```bash
# Get certificate condition
kubectl get bmc bmc-server-01 -o jsonpath='{.status.conditions[?(@.type=="CertificateReady")]}'

# Watch for certificate updates
kubectl get bmc -w
```

### Logs

View BMC controller logs to troubleshoot certificate issues:

```bash
kubectl logs -n metal-operator-system deployment/metal-operator-controller-manager -c manager -f | grep certificate
```

## Troubleshooting

### Certificate Not Installing

**Symptom**: `CertificateReady` condition shows `Failed`

**Possible Causes**:
1. **Secret not found**: Verify secret exists in correct namespace
   ```bash
   kubectl get secret bmc-tls-cert -n default
   ```

2. **Wrong secret type**: Secret must be `kubernetes.io/tls`
   ```bash
   kubectl get secret bmc-tls-cert -o jsonpath='{.type}'
   # Should output: kubernetes.io/tls
   ```

3. **Missing keys**: Secret must contain `tls.crt` and `tls.key`
   ```bash
   kubectl get secret bmc-tls-cert -o jsonpath='{.data}'
   ```

4. **BMC doesn't support Redfish CertificateService**: Check BMC firmware version
   ```bash
   curl -k -u admin:password https://bmc-ip/redfish/v1/CertificateService
   ```

5. **RBAC permissions**: Ensure operator has permissions to read secrets
   ```bash
   kubectl auth can-i get secrets --as=system:serviceaccount:metal-operator-system:metal-operator-controller-manager
   ```

### Certificate Not Renewing

**Symptom**: Old certificate remains on BMC after cert-manager renewal

**Solutions**:
1. **Check secret was updated**:
   ```bash
   kubectl describe secret bmc-tls-cert
   ```
   Look for recent "Data" update timestamp

2. **Verify secret watch is working**: Check operator logs for secret change events

3. **Manual trigger**: Delete and recreate the BMC resource to force reconciliation

### Invalid Certificate Format

**Symptom**: `Failed to decode PEM certificate` or `Failed to parse certificate`

**Solutions**:
1. **Verify PEM format**: Certificate should start with `-----BEGIN CERTIFICATE-----`
   ```bash
   kubectl get secret bmc-tls-cert -o jsonpath='{.data.tls\.crt}' | base64 -d
   ```

2. **Check for multiple certificates**: Some CAs include chain in `tls.crt`. Only leaf certificate should be used.

3. **Regenerate certificate**: If corrupt, delete and recreate

## Best Practices

### 1. Use Short-Lived Certificates

BMCs support automatic renewal, so prefer shorter validity periods:

```yaml
spec:
  duration: 2160h  # 90 days
  renewBefore: 720h  # Renew 30 days before expiration
```

### 2. Include IP Addresses in SAN

BMCs are typically accessed by IP address:

```yaml
spec:
  ipAddresses:
    - 192.168.1.100
  dnsNames:
    - bmc-server-01.example.com  # Optional
```

### 3. Monitor Certificate Expiration

Set up alerts for certificate expiration:

```yaml
# Prometheus alert example
- alert: BMCCertificateExpiringSoon
  expr: |
    bmc_certificate_expiry_seconds < 30 * 24 * 3600
  for: 1h
  annotations:
    summary: "BMC {{ $labels.bmc }} certificate expiring soon"
```

### 4. Use Separate Secrets Per BMC

While multiple BMCs can share a secret, using separate secrets provides:
- Finer-grained access control
- Independent renewal cycles
- Easier troubleshooting

### 5. Test Certificate Installation

Before rolling out to production:
1. Test with self-signed certificates
2. Verify Redfish CertificateService support
3. Test renewal flow
4. Validate monitoring and alerting

## Integration Patterns

### With cert-manager

Full automation with automatic renewal:

```yaml
# 1. Create Issuer
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned
spec:
  selfSigned: {}
---
# 2. Create Certificate (cert-manager creates secret)
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: bmc-cert
spec:
  secretName: bmc-tls-cert
  dnsNames: [bmc.example.com]
  issuerRef:
    name: selfsigned
---
# 3. Reference in BMC (metal-operator installs on BMC)
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: my-bmc
spec:
  tlsSecretRef:
    name: bmc-tls-cert
```

### With Vault (via external-secrets)

Centralized secret management:

```yaml
# 1. Create SecretStore
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: vault-backend
spec:
  provider:
    vault:
      server: https://vault.example.com
      path: secret
      auth:
        kubernetes:
          mountPath: kubernetes
          role: external-secrets
---
# 2. Create ExternalSecret
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: bmc-vault-secret
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: bmc-tls-cert
    template:
      type: kubernetes.io/tls
  data:
    - secretKey: tls.crt
      remoteRef:
        key: bmc/certs/server-01
        property: certificate
    - secretKey: tls.key
      remoteRef:
        key: bmc/certs/server-01
        property: private_key
---
# 3. Reference in BMC
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: my-bmc
spec:
  tlsSecretRef:
    name: bmc-tls-cert
```

## Security Considerations

### 1. Secret Permissions

Ensure secrets are properly protected:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bmc-tls-cert
  namespace: default
type: kubernetes.io/tls
# Note: RBAC should restrict access to this secret
```

### 2. TLS Version Support

BMCs may have limited TLS support. Ensure certificates are compatible with BMC firmware capabilities.

### 3. Private Key Protection

- Never commit private keys to version control
- Use secret management tools (Vault, Sealed Secrets, etc.)
- Rotate keys regularly

### 4. Certificate Validation

The operator validates certificates before installation:
- PEM format check
- Expiration check
- Serial number verification

## Migration from Old API

If you were using the old `spec.certificate` field (cert-manager integration), migrate to the new approach:

### Old API (Deprecated - Removed)

```yaml
spec:
  certificate:
    issuerRef:
      name: my-issuer
      kind: Issuer
    dnsNames:
      - bmc.example.com
```

### New API (Current)

```yaml
# 1. Create cert-manager Certificate
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: bmc-cert
spec:
  secretName: bmc-tls-cert
  dnsNames:
    - bmc.example.com
  issuerRef:
    name: my-issuer
---
# 2. Reference secret in BMC
spec:
  tlsSecretRef:
    name: bmc-tls-cert
```

**Benefits of new approach**:
- ✅ No cert-manager dependency in metal-operator
- ✅ Works with any certificate management tool
- ✅ Simpler controller logic
- ✅ Better separation of concerns
- ✅ More flexible secret management

## API Reference

### BMCSpec.TLSSecretRef

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `name` | string | Name of the TLS secret | Yes |
| `namespace` | string | Namespace of the secret (defaults to operator namespace) | No |

### Secret Requirements

The referenced secret must:
- Be of type `kubernetes.io/tls`
- Contain `tls.crt` key with PEM-encoded certificate
- Contain `tls.key` key with PEM-encoded private key

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `certificateSecretRef.name` | string | Name of the currently installed certificate secret |
| `conditions[type=CertificateReady]` | Condition | Certificate installation status |

## Examples

See `config/samples/bmc_with_tls_secret.yaml` for complete examples including:
- Manual TLS secret creation
- cert-manager integration
- external-secrets-operator integration
- Basic BMC configuration

## Support

For issues or questions:
- GitHub Issues: https://github.com/ironcore-dev/metal-operator/issues
- Documentation: https://github.com/ironcore-dev/metal-operator/tree/main/docs
