# BMC Certificate Management

This guide explains how to configure and manage TLS certificates for BMC (Baseboard Management Controller) resources using [cert-manager](https://cert-manager.io/).

## Overview

The metal-operator integrates with cert-manager to automate TLS certificate lifecycle management for BMCs. When enabled, the operator can:

- Request and provision certificates from cert-manager Issuers
- Install certificates on BMC devices via Redfish API
- Monitor certificate status and renewal
- Automatically update certificates before expiration

## Prerequisites

### 1. Install cert-manager

cert-manager must be installed in your cluster before using certificate management features.

**Using kubectl:**

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.0/cert-manager.yaml
```

**Using Helm:**

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set installCRDs=true
```

Verify the installation:

```bash
kubectl get pods -n cert-manager
```

Expected output:
```text
NAME                                       READY   STATUS    RESTARTS   AGE
cert-manager-7d9f6d9d8c-xxxxx              1/1     Running   0          1m
cert-manager-cainjector-5c7d9f5f5-xxxxx    1/1     Running   0          1m
cert-manager-webhook-6b8f9f5d5-xxxxx       1/1     Running   0          1m
```

### 2. Enable Certificate Management

Certificate management is controlled by the `--enable-bmc-certificate-management` flag in the metal-operator deployment.

**For production deployments**, edit the deployment manifest or Helm values:

```bash
# Using kubectl edit
kubectl edit deployment -n metal-operator-system metal-operator-controller-manager

# Add or modify the flag under containers.args:
args:
  - --enable-bmc-certificate-management=true
```

**For development**, start the operator with the flag:

```bash
make run ARGS="--enable-bmc-certificate-management=true"
```

## Configuring Issuers

cert-manager uses Issuers or ClusterIssuers to represent certificate authorities. You must create at least one Issuer before configuring BMC certificates.

### Self-Signed Issuer (Development/Testing)

Suitable for development, testing, and non-production environments where self-signed certificates are acceptable.

```yaml
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
# SPDX-License-Identifier: Apache-2.0

apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: default
spec:
  selfSigned: {}
```

Apply the issuer:

```bash
kubectl apply -f selfsigned-issuer.yaml
```

### Let's Encrypt ACME Issuer (Production)

For production environments with publicly accessible BMCs, use Let's Encrypt with DNS-01 challenge.

**Prerequisites:**
- DNS provider integration (e.g., Route53, CloudFlare, Google Cloud DNS)
- API credentials for DNS provider
- Public DNS records for BMC hostnames

**Example with Route53:**

```yaml
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
# SPDX-License-Identifier: Apache-2.0

apiVersion: v1
kind: Secret
metadata:
  name: route53-credentials
  namespace: default
type: Opaque
stringData:
  secret-access-key: "YOUR_AWS_SECRET_ACCESS_KEY"
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: letsencrypt-prod
  namespace: default
spec:
  acme:
    # Production ACME server
    server: https://acme-v02.api.letsencrypt.org/directory

    # Email for certificate expiration notifications
    email: admin@example.com

    # Secret to store ACME account private key
    privateKeySecretRef:
      name: letsencrypt-prod-account-key

    # DNS-01 challenge solver
    solvers:
      - dns01:
          route53:
            region: us-west-2
            accessKeyID: YOUR_AWS_ACCESS_KEY_ID
            secretAccessKeySecretRef:
              name: route53-credentials
              key: secret-access-key
```

**Example with CloudFlare:**

```yaml
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
# SPDX-License-Identifier: Apache-2.0

apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-api-token
  namespace: default
type: Opaque
stringData:
  api-token: "YOUR_CLOUDFLARE_API_TOKEN"
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: letsencrypt-prod
  namespace: default
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@example.com
    privateKeySecretRef:
      name: letsencrypt-prod-account-key
    solvers:
      - dns01:
          cloudflare:
            apiTokenSecretRef:
              name: cloudflare-api-token
              key: api-token
```

Apply the issuer:

```bash
kubectl apply -f letsencrypt-issuer.yaml
```

### CA Issuer (Enterprise PKI)

For organizations with internal PKI, use a CA Issuer with your enterprise root CA.

```yaml
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
# SPDX-License-Identifier: Apache-2.0

apiVersion: v1
kind: Secret
metadata:
  name: enterprise-ca
  namespace: default
type: kubernetes.io/tls
data:
  tls.crt: LS0tLS1CRUdJTi... # Base64-encoded CA certificate
  tls.key: LS0tLS1CRUdJTi... # Base64-encoded CA private key
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: enterprise-ca-issuer
  namespace: default
spec:
  ca:
    secretName: enterprise-ca
```

Apply the issuer:

```bash
kubectl apply -f ca-issuer.yaml
```

## Configuring BMCs with Certificates

Once you have an Issuer configured, you can enable certificate management on BMC resources.

### Basic Configuration

Minimal configuration with required fields only:

```yaml
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
# SPDX-License-Identifier: Apache-2.0

apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: my-bmc
spec:
  access:
    ip: 192.168.1.100
  bmcSecretRef:
    name: bmc-credentials
  protocol:
    name: Redfish
    port: 443
    scheme: https

  # Certificate configuration
  certificate:
    issuerRef:
      name: selfsigned-issuer
      kind: Issuer
```

### Complete Configuration

All available certificate options:

```yaml
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
# SPDX-License-Identifier: Apache-2.0

apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: my-bmc
spec:
  access:
    ip: 192.168.1.100
    macAddress: "00:1A:2B:3C:4D:5E"
  bmcSecretRef:
    name: bmc-credentials
  protocol:
    name: Redfish
    port: 443
    scheme: https

  # Hostname is recommended for certificate management
  hostname: "bmc-server-01.example.com"

  # Certificate configuration
  certificate:
    # Reference to cert-manager Issuer or ClusterIssuer
    issuerRef:
      name: letsencrypt-prod  # Your Issuer name
      kind: Issuer            # "Issuer" or "ClusterIssuer"

    # Certificate validity duration (optional, defaults to issuer's default)
    # Recommended: 2160h (90 days) for Let's Encrypt
    duration: 2160h

    # Common name (optional, typically the primary hostname)
    commonName: "bmc-server-01.example.com"

    # DNS names for Subject Alternative Names (optional)
    dnsNames:
      - "bmc-server-01.example.com"
      - "bmc-server-01.internal.example.com"

    # IP addresses for Subject Alternative Names (optional)
    ipAddresses:
      - "192.168.1.100"
```

Apply the BMC configuration:

```bash
kubectl apply -f bmc.yaml
```

### Using ClusterIssuer

If you have a ClusterIssuer (cluster-wide issuer), reference it with `kind: ClusterIssuer`:

```yaml
certificate:
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer  # ClusterIssuer instead of Issuer
```

## Monitoring Certificate Status

### Check BMC Status

View certificate status in the BMC resource:

```bash
kubectl get bmc my-bmc -o yaml
```

Look for the `status.conditions` array:

```yaml
status:
  conditions:
    - type: CertificateReady
      status: "True"
      reason: Issued
      message: "Certificate successfully issued and installed"
      lastTransitionTime: "2024-03-17T10:00:00Z"
  certificateSecretRef:
    name: my-bmc-cert
  certificateRequestName: my-bmc-cert-xxxxx
```

### Certificate Condition Types

| Status | Reason | Description |
|--------|--------|-------------|
| `True` | `Issued` | Certificate successfully issued and installed on BMC |
| `False` | `Pending` | Certificate request is pending (awaiting cert-manager) |
| `False` | `Failed` | Certificate request failed (check cert-manager logs) |

### Check Certificate Request

View the underlying cert-manager Certificate resource:

```bash
# List certificates
kubectl get certificate

# Describe specific certificate
kubectl describe certificate my-bmc-cert
```

### Check Certificate Request Details

For troubleshooting, view CertificateRequest:

```bash
# Get CertificateRequest name from BMC status
CERT_REQUEST=$(kubectl get bmc my-bmc -o jsonpath='{.status.certificateRequestName}')

# Describe the request
kubectl describe certificaterequest $CERT_REQUEST
```

## Troubleshooting

### Certificate Stuck in Pending

**Symptoms:**
- BMC condition shows `CertificateReady: False, Reason: Pending`
- Certificate not installed on BMC

**Common causes:**

1. **Issuer not ready**
   ```bash
   kubectl get issuer selfsigned-issuer -o yaml
   ```
   Ensure `status.conditions` shows `Ready: True`.

2. **DNS-01 challenge failing** (for ACME issuers)
   ```bash
   kubectl describe challenge
   ```
   Check DNS propagation and provider credentials.

3. **cert-manager webhook issues**
   ```bash
   kubectl logs -n cert-manager deployment/cert-manager-webhook
   ```

### Certificate Request Failed

**Symptoms:**
- BMC condition shows `CertificateReady: False, Reason: Failed`

**Diagnosis:**

```bash
# Check Certificate status
kubectl describe certificate my-bmc-cert

# Check CertificateRequest
CERT_REQUEST=$(kubectl get bmc my-bmc -o jsonpath='{.status.certificateRequestName}')
kubectl describe certificaterequest $CERT_REQUEST

# Check cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager -f
```

**Common causes:**
- Invalid issuer reference
- DNS validation failures (ACME)
- CA signing errors
- Rate limits (Let's Encrypt)

### Certificate Not Installing on BMC

**Symptoms:**
- Certificate issued successfully in cert-manager
- BMC still shows pending or failed status

**Diagnosis:**

```bash
# Check metal-operator controller logs
kubectl logs -n metal-operator-system deployment/metal-operator-controller-manager -f

# Check BMC connectivity
kubectl get bmc my-bmc -o jsonpath='{.status.state}'
```

**Common causes:**
- BMC not reachable (network issues)
- Invalid BMC credentials
- Redfish API not supporting certificate operations
- BMC firmware version incompatible

### Certificate Management Not Enabled

**Symptoms:**
- No certificate processing happening
- Certificate configuration ignored

**Solution:**

Verify the `--enable-bmc-certificate-management` flag is set:

```bash
kubectl get deployment -n metal-operator-system metal-operator-controller-manager -o yaml | grep enable-bmc-certificate-management
```

If not present, add the flag to the deployment.

## Migration Guide

### Migrating Existing BMCs

To enable certificate management on existing BMC resources:

1. **Backup existing BMC configuration:**
   ```bash
   kubectl get bmc my-bmc -o yaml > bmc-backup.yaml
   ```

2. **Enable certificate management flag** (if not already enabled):
   ```bash
   kubectl edit deployment -n metal-operator-system metal-operator-controller-manager
   ```

3. **Update BMC resource** with certificate configuration:
   ```bash
   kubectl edit bmc my-bmc
   ```

   Add the `certificate` section to `spec`.

4. **Verify certificate issuance:**
   ```bash
   kubectl get bmc my-bmc -o jsonpath='{.status.conditions[?(@.type=="CertificateReady")]}'
   ```

### Migration Order

For large deployments, migrate BMCs gradually:

1. **Start with non-production BMCs** to validate configuration
2. **Test certificate installation** and verify BMC accessibility
3. **Monitor certificate renewal** (wait for at least one renewal cycle)
4. **Roll out to production BMCs** in batches

## Rollback Instructions

### Disabling Certificate Management

To disable certificate management for a BMC:

1. **Remove certificate configuration** from BMC spec:
   ```bash
   kubectl edit bmc my-bmc
   ```

   Delete the `certificate` section from `spec`.

2. **The existing certificate remains on the BMC** but will not be renewed by the operator.

### Complete Rollback

To completely disable certificate management:

1. **Remove certificate configs from all BMCs:**
   ```bash
   # List BMCs with certificates
   kubectl get bmc -o json | jq -r '.items[] | select(.spec.certificate != null) | .metadata.name'

   # Edit each BMC to remove certificate config
   kubectl edit bmc <bmc-name>
   ```

2. **Disable certificate management flag:**
   ```bash
   kubectl edit deployment -n metal-operator-system metal-operator-controller-manager
   ```

   Set `--enable-bmc-certificate-management=false` or remove the flag.

3. **Optional: Clean up certificate requests:**
   ```bash
   # Delete CertificateRequest resources created by metal-operator
   kubectl delete certificaterequest -l app.kubernetes.io/managed-by=metal-operator

   # Or list and delete individual certificate requests referenced in BMC status
   kubectl get bmc -o json | jq -r '.items[] | select(.status.certificateRequestName != null) | .status.certificateRequestName' | xargs -I {} kubectl delete certificaterequest {}
   ```

## Best Practices

### Certificate Duration

- **Let's Encrypt:** Use 90 days (2160h) to align with Let's Encrypt defaults
- **Enterprise CA:** Follow your organization's PKI policy
- **Self-signed:** Use shorter durations (30-90 days) for development

### DNS Configuration

- **Always configure `hostname`** in BMC spec for certificate management
- **Use fully qualified domain names (FQDN)** in `dnsNames`
- **Include all hostnames** the BMC is accessed by in `dnsNames`
- **Include IP addresses** in `ipAddresses` if clients connect via IP

### Issuer Selection

- **Development/Testing:** Use `selfSigned` issuer
- **Production (public):** Use Let's Encrypt with DNS-01 challenge
- **Production (private):** Use enterprise CA issuer
- **Use ClusterIssuer** for cluster-wide certificate authority

### Monitoring

- **Set up alerts** for BMC certificate condition changes
- **Monitor cert-manager** for certificate renewal issues
- **Log certificate installation failures** for troubleshooting
- **Track certificate expiration** dates

### Security

- **Use RBAC** to restrict access to BMC credentials
- **Rotate BMC credentials** regularly (separate from certificate management)
- **Secure Issuer credentials** (AWS keys, CloudFlare tokens, CA private keys)
- **Use namespaced Issuers** when possible for better isolation

## Reference

### BMC Certificate Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `issuerRef` | Object | Yes | Reference to cert-manager Issuer or ClusterIssuer |
| `issuerRef.name` | String | Yes | Name of the Issuer or ClusterIssuer |
| `issuerRef.kind` | String | No | "Issuer" (default) or "ClusterIssuer" |
| `duration` | Duration | No | Certificate validity period (e.g., "2160h" for 90 days) |
| `commonName` | String | No | Certificate common name |
| `dnsNames` | []String | No | DNS names for Subject Alternative Names |
| `ipAddresses` | []String | No | IP addresses for Subject Alternative Names |

### BMC Certificate Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `certificateSecretRef` | Object | Reference to Secret containing certificate and key |
| `certificateRequestName` | String | Name of the cert-manager CertificateRequest |
| `conditions[type=CertificateReady]` | Condition | Certificate readiness condition |

### Related Resources

- [cert-manager documentation](https://cert-manager.io/docs/)
- [Redfish API specification](https://www.dmtf.org/standards/redfish)
- [Kubernetes TLS management](https://kubernetes.io/docs/tasks/tls/)
- [metal-operator API reference](./api-reference/api)

## Support

For issues and questions:
- [GitHub Issues](https://github.com/ironcore-dev/metal-operator/issues)
- [Community Discussions](https://github.com/ironcore-dev/metal-operator/discussions)
