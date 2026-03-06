# Prometheus Metrics

The metal-operator exposes custom Prometheus metrics to provide visibility into the state and health of managed servers. These metrics are exposed at the `/metrics` endpoint alongside standard controller-runtime metrics.

## Accessing Metrics

### Local Development

```bash
# Port-forward to the metrics endpoint
kubectl -n metal-operator-system port-forward deployment/metal-operator-controller-manager 8443:8443

# Query metrics (skip TLS verification for dev)
curl -k https://localhost:8443/metrics | grep metal_
```

### Production

The operator includes a ServiceMonitor configured for Prometheus Operator:

```yaml
# config/prometheus/monitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: metal-operator-controller-manager-metrics-monitor
  namespace: metal-operator-system
spec:
  endpoints:
  - bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    path: /metrics
    port: https
    scheme: https
    tlsConfig:
      insecureSkipVerify: true
  selector:
    matchLabels:
      control-plane: controller-manager
```

## Available Metrics

### Server State Distribution (`metal_server_state`)

**Type:** Gauge
**Description:** Current count of servers in each state
**Labels:**
- `state`: ServerState value (Initial, Discovery, Available, Reserved, Error, Maintenance)

**Example values:**
```text
metal_server_state{state="Available"} 5
metal_server_state{state="Reserved"} 2
metal_server_state{state="Error"} 0
metal_server_state{state="Maintenance"} 1
```

**Use cases:**
- Monitor available server capacity
- Alert on servers in error states
- Track server lifecycle distribution

### Server Power State Distribution (`metal_server_power_state`)

**Type:** Gauge
**Description:** Current count of servers in each power state
**Labels:**
- `power_state`: ServerPowerState value (On, Off, PoweringOn, PoweringOff, Paused)

**Example values:**
```text
metal_server_power_state{power_state="On"} 7
metal_server_power_state{power_state="Off"} 1
metal_server_power_state{power_state="PoweringOn"} 0
```

**Use cases:**
- Track power operations in progress
- Identify stuck power transitions
- Energy consumption estimation

### Server Condition Status (`metal_server_condition_status`)

**Type:** Gauge
**Description:** Count of servers with each condition status
**Labels:**
- `condition_type`: Condition type (e.g., "Ready", "PoweringOn", "Discovered")
- `status`: Condition status (True, False, Unknown)

**Example values:**
```text
metal_server_condition_status{condition_type="Ready",status="True"} 1
metal_server_condition_status{condition_type="Discovered",status="True"} 1
metal_server_condition_status{condition_type="PoweringOn",status="False"} 0
```

**Use cases:**
- Track server health conditions
- Alert on specific condition failures
- Monitor discovery and power operation progress

### Server Reconciliation Total (`metal_server_reconciliation_total`)

**Type:** Counter
**Description:** Total number of server reconciliations by result
**Labels:**
- `result`: Operation result (success, error_fetch, error_reconcile)

**Example values:**
```text
metal_server_reconciliation_total{result="success"} 1523
metal_server_reconciliation_total{result="error_fetch"} 2
metal_server_reconciliation_total{result="error_reconcile"} 15
```

**Use cases:**
- Monitor reconciliation error rates
- Track controller performance
- Debug reconciliation issues

## Example Queries

### Server Inventory

```promql
# Total servers by state
sum by (state) (metal_server_state)

# Available server capacity
metal_server_state{state="Available"}

# Servers requiring attention
metal_server_state{state="Error"} + metal_server_state{state="Maintenance"}

# Percentage of servers in error state
metal_server_state{state="Error"} / sum(metal_server_state) * 100
```

### Power Operations

```promql
# Servers currently powered on
metal_server_power_state{power_state="On"}

# Servers in transition states (possibly stuck)
metal_server_power_state{power_state="PoweringOn"} + metal_server_power_state{power_state="PoweringOff"}

# Power state distribution
sum by (power_state) (metal_server_power_state)
```

### Health and Conditions

```promql
# Count of servers with Ready=True
sum(metal_server_condition_status{condition_type="Ready",status="True"})

# Servers with failed power operations
metal_server_condition_status{condition_type="PoweringOn",status="False"}
```

### Reconciliation Performance

```promql
# Reconciliation error rate (errors per second over 5 minutes)
rate(metal_server_reconciliation_total{result=~"error_.*"}[5m])

# Success ratio
rate(metal_server_reconciliation_total{result="success"}[5m])
  / rate(metal_server_reconciliation_total[5m])

# Total reconciliation rate
sum(rate(metal_server_reconciliation_total[5m]))
```

## Alerting Rules

Example PrometheusRule resource:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: metal-operator-server-alerts
  namespace: metal-operator-system
spec:
  groups:
  - name: metal_operator_servers
    interval: 30s
    rules:
    - alert: NoAvailableServers
      expr: metal_server_state{state="Available"} == 0
      for: 5m
      annotations:
        summary: "No available servers in the fleet"
        description: "All servers are either Reserved, in Maintenance, or in Error state"
      labels:
        severity: warning

    - alert: ServersInErrorState
      expr: metal_server_state{state="Error"} > 0
      for: 2m
      annotations:
        summary: "Servers are in Error state"
        description: "{{ $value }} server(s) are in Error state and require attention"
      labels:
        severity: critical

    - alert: ServersPoweringOnTooLong
      expr: metal_server_power_state{power_state="PoweringOn"} > 0
      for: 10m
      annotations:
        summary: "Servers stuck in PoweringOn state"
        description: "{{ $value }} server(s) have been in PoweringOn state for over 10 minutes"
      labels:
        severity: warning

    - alert: HighReconciliationErrorRate
      expr: rate(metal_server_reconciliation_total{result=~"error_.*"}[5m]) > 0.1
      for: 5m
      annotations:
        summary: "High server reconciliation error rate"
        description: "Server reconciliation errors are occurring at {{ $value | humanize }} per second"
      labels:
        severity: warning

    - alert: LowAvailableServerCapacity
      expr: metal_server_state{state="Available"} < 2
      for: 5m
      annotations:
        summary: "Low available server capacity"
        description: "Only {{ $value }} server(s) are available"
      labels:
        severity: warning
```

## Grafana Dashboard

Example dashboard queries for visualization:

### Server State Distribution Panel (Pie Chart)

```promql
sum by (state) (metal_server_state)
```

### Server Power State Timeline (Graph)

```promql
metal_server_power_state
```

### Reconciliation Error Rate (Graph)

```promql
rate(metal_server_reconciliation_total{result="success"}[5m])
rate(metal_server_reconciliation_total{result=~"error_.*"}[5m])
```

### Available Server Capacity (Gauge)

```promql
metal_server_state{state="Available"}
```

## Implementation Details

### Metric Collection Strategy

The operator uses a **custom Collector pattern** to ensure accurate metric counts:

1. On each Prometheus scrape (default: 30s interval), the collector lists all Server resources
2. Counts are computed in-memory and emitted as gauge metrics
3. This ensures metrics always reflect current cluster state, not accumulated values

**Benefits:**
- Accurate counts even if reconciliation loop misses updates
- No metric staleness from deleted servers
- Resilient to operator restarts

**Performance considerations:**
- ServerList operation uses watch cache (fast)
- Default scrape interval is 30s (adjustable)
- For very large clusters (>1000 servers), consider increasing scrape interval

### Cardinality Control

All metrics use **bounded label value sets** to prevent cardinality explosion:

- `state`: 6 possible values
- `power_state`: 5 possible values
- `condition_type`: ~10 typical values
- `result`: 3 values

**Never used as labels:**
- Server names, UUIDs, or namespaces
- IP addresses or MAC addresses
- Timestamps

This ensures Prometheus performance remains optimal even with large server fleets.

## Troubleshooting

### Metrics Not Appearing

1. Verify ServiceMonitor is deployed:
   ```bash
   kubectl -n metal-operator-system get servicemonitor
   ```

2. Check Prometheus targets:
   ```bash
   kubectl -n monitoring port-forward svc/prometheus-operated 9090:9090
   # Open http://localhost:9090/targets
   # Verify "metal-operator-controller-manager-metrics-monitor" target is UP
   ```

3. Check manager logs for metric registration:
   ```bash
   kubectl -n metal-operator-system logs deployment/metal-operator-controller-manager -c manager | grep metrics
   # Should see: "Registered custom server metrics collector"
   ```

### Incorrect Metric Values

1. Verify servers are reconciling:
   ```bash
   kubectl get servers
   ```

2. Check reconciliation metrics:
   ```promql
   rate(metal_server_reconciliation_total[5m])
   ```

3. Query specific label combinations:
   ```bash
   curl -k https://localhost:8443/metrics | grep metal_server_state
   ```

### High Cardinality Warning

If Prometheus shows cardinality warnings for metal-operator metrics:

1. Verify no custom labels were added
2. Check for metric label explosion (should never happen with current implementation)
3. Review Prometheus storage settings if total metrics exceed capacity
