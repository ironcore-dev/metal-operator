# Observability

The metal-operator provides comprehensive observability capabilities for monitoring and troubleshooting bare metal server infrastructure.

## Metrics

The operator exposes custom Prometheus metrics for monitoring server state, power operations, and reconciliation performance.

**[View Metrics Documentation →](./metrics.md)**

Key metrics include:
- **Server State Distribution** - Count of servers by state (Available, Reserved, Error, etc.)
- **Server Power State** - Count of servers by power state (On, Off, PoweringOn, etc.)
- **Server Conditions** - Health status of server conditions (Ready, Discovered, etc.)
- **Reconciliation Metrics** - Success/error counts for reconciliation operations

## Quick Start

```bash
# Port-forward to metrics endpoint
kubectl -n metal-operator-system port-forward deployment/metal-operator-controller-manager 8443:8443

# Query server metrics
curl -k https://localhost:8443/metrics | grep metal_server
```

## Contents

- [Metrics](./metrics.md) - Detailed metrics documentation with example queries and alerting rules
