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

**Type:** Gauge (enum pattern)
**Description:** Server state as enum metric — emits all possible states for each server with value 1 for the current state and 0 for all others. This pattern prevents series churn when servers change state.
**Labels:**
- `server`: Server resource name
- `state`: ServerState value (Initial, Discovery, Available, Reserved, Error, Maintenance)

**Example values:**
```text
# Server srv-001 is currently in Available state
metal_server_state{server="srv-001", state="Initial"} 0
metal_server_state{server="srv-001", state="Discovery"} 0
metal_server_state{server="srv-001", state="Available"} 1
metal_server_state{server="srv-001", state="Reserved"} 0
metal_server_state{server="srv-001", state="Error"} 0
metal_server_state{server="srv-001", state="Maintenance"} 0
```

**Use cases:**
- Monitor available server capacity: `count(metal_server_state{state="Available"} == 1)`
- Alert on specific servers in error states: `metal_server_state{state="Error"} == 1`
- Track server lifecycle distribution: `count by (state) (metal_server_state == 1)`

### Server Power State Distribution (`metal_server_power_state`)

**Type:** Gauge (enum pattern)
**Description:** Server power state as enum metric — emits all possible power states for each server with value 1 for the current state and 0 for all others.
**Labels:**
- `server`: Server resource name
- `power_state`: ServerPowerState value (On, Off, PoweringOn, PoweringOff, Paused)

**Example values:**
```text
# Server srv-001 is currently powered On
metal_server_power_state{server="srv-001", power_state="On"} 1
metal_server_power_state{server="srv-001", power_state="Off"} 0
metal_server_power_state{server="srv-001", power_state="Paused"} 0
metal_server_power_state{server="srv-001", power_state="PoweringOn"} 0
metal_server_power_state{server="srv-001", power_state="PoweringOff"} 0
```

**Use cases:**
- Track power operations in progress
- Identify specific servers with stuck power transitions
- Energy consumption estimation

### Server Condition Status (`metal_server_condition_status`)

**Type:** Gauge
**Description:** Current condition status of each server (value is always 1)
**Labels:**
- `server`: Server resource name
- `condition_type`: Condition type (e.g., "Ready", "PoweringOn", "Discovered")
- `status`: Condition status (True, False, Unknown)

**Example values:**
```text
metal_server_condition_status{server="srv-001", condition_type="Ready", status="True"} 1
metal_server_condition_status{server="srv-001", condition_type="Discovered", status="True"} 1
metal_server_condition_status{server="srv-002", condition_type="Ready", status="False"} 1
```

**Use cases:**
- Track individual server health conditions
- Alert on specific servers with condition failures
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
# Count of servers by state
count by (state) (metal_server_state == 1)

# Number of available servers
count(metal_server_state{state="Available"} == 1)

# List servers in error state
metal_server_state{state="Error"} == 1

# Count of servers requiring attention (Error or Maintenance)
count(metal_server_state{state=~"Error|Maintenance"} == 1)

# Percentage of servers in error state
count(metal_server_state{state="Error"} == 1) / count(metal_server_state{state="Available"} == 1 or metal_server_state{state="Reserved"} == 1 or metal_server_state{state="Error"} == 1) * 100
```

### Power Operations

```promql
# Count of servers currently powered on
count(metal_server_power_state{power_state="On"} == 1)

# List servers in transition states (possibly stuck)
metal_server_power_state{power_state=~"PoweringOn|PoweringOff"} == 1

# Count servers in transition states
count(metal_server_power_state{power_state=~"PoweringOn|PoweringOff"} == 1)

# Power state distribution
count by (power_state) (metal_server_power_state == 1)
```

### Health and Conditions

```promql
# Count of servers with Ready=True
count(metal_server_condition_status{condition_type="Ready", status="True"})

# List servers with Ready=False
metal_server_condition_status{condition_type="Ready", status="False"}

# Servers with failed power operations
metal_server_condition_status{condition_type="PoweringOn", status="False"}
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

Example PrometheusRule resource (see `config/prometheus/server_alerts.yaml` for the full version):

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
      expr: (count(metal_server_state{state="Available"}) or vector(0)) < 1 and (count(metal_server_state{state="Reserved"}) or vector(0)) < 1
      for: 5m
      annotations:
        summary: "No available or reserved servers in the fleet"
        description: "The fleet is completely idle with no servers in Available or Reserved state"
      labels:
        severity: warning

    - alert: ServersInErrorState
      expr: metal_server_state{state="Error"} == 1
      for: 2m
      annotations:
        summary: "Server {{ $labels.server }} is in Error state"
        description: "Server {{ $labels.server }} is in Error state and requires attention"
      labels:
        severity: critical

    - alert: ServersPoweringOnTooLong
      expr: metal_server_power_state{power_state="PoweringOn"} == 1
      for: 10m
      annotations:
        summary: "Server {{ $labels.server }} stuck in PoweringOn state"
        description: "Server {{ $labels.server }} has been in PoweringOn state for over 10 minutes"
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
      expr: (count(metal_server_state{state="Available"}) or vector(0)) < 2
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
count by (state) (metal_server_state == 1)
```

### Server Power State Timeline (Graph)

```promql
count by (power_state) (metal_server_power_state == 1)
```

### Reconciliation Error Rate (Graph)

```promql
rate(metal_server_reconciliation_total{result="success"}[5m])
rate(metal_server_reconciliation_total{result=~"error_.*"}[5m])
```

### Available Server Capacity (Gauge)

```promql
count(metal_server_state{state="Available"} == 1)
```

## Implementation Details

### Metric Collection Strategy

The operator uses a **custom Collector pattern** with **enum metrics** to emit per-server state information:

1. On each Prometheus scrape (default: 30s interval), the collector lists all Server resources
2. For each server, it emits enum metrics for all possible states (value=1 for current state, value=0 for others)
3. This **enum pattern** prevents series churn when servers change state — values flip but all series remain active

**Benefits:**
- Per-server visibility enables targeted alerting (e.g., \"Server X is in Error state\")
- Accurate counts via `count(metric == 1)` aggregation
- No stale series when state changes (unlike single-value-per-state approaches)
- Works correctly with `rate()`, `changes()`, and other Prometheus functions
- Resilient to operator restarts

**Performance considerations:**
- ServerList operation uses watch cache (fast)
- Default scrape interval is 30s (adjustable)
- Cardinality: (servers × 6 states) + (servers × 5 power states) + conditions
- For very large clusters (>1000 servers), consider increasing scrape interval

### Cardinality Control

Metrics include the `server` label to enable per-server alerting and filtering. Label cardinality is controlled by using **bounded label value sets** for state-related labels:

- `server`: One value per Server resource (scales with fleet size)
- `state`: 6 possible values
- `power_state`: 5 possible values
- `condition_type`: ~10 typical values
- `result`: 3 values

**Never used as labels:**
- Server UUIDs
- IP addresses or MAC addresses
- Timestamps

For very large server fleets (>1000 servers), monitor Prometheus memory usage and consider increasing the scrape interval if needed.

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
