# CraftStack Monitoring

CraftStack exposes metrics via **Prometheus** and optionally pushes to
**InfluxDB v2**. Grafana is intentionally *not* provisioned from code —
follow the standard "add data source → import dashboard" flow.

## Prometheus

Enable in `configs/master.yaml`:

```yaml
observability:
  prometheus:
    enabled: true
    path: "/metrics"
```

The `/metrics` endpoint has no application-session auth. Protect it at the
network layer — bind scrapers to localhost or a private network, or put a
reverse proxy in front.

Example `prometheus.yml` scrape config:

```yaml
scrape_configs:
  - job_name: craftstack
    static_configs:
      - targets: ["craftstack-master.internal:8080"]
    metrics_path: /metrics
    scrape_interval: 15s
```

### Exported metrics

| Metric | Type | Labels |
|---|---|---|
| `craftstack_node_cpu_percent` | gauge | `node` |
| `craftstack_node_memory_percent` | gauge | `node` |
| `craftstack_node_memory_used_mb` | gauge | `node` |
| `craftstack_node_memory_total_mb` | gauge | `node` |
| `craftstack_node_disk_percent` | gauge | `node` |
| `craftstack_instance_cpu_percent` | gauge | `instance`, `node`, `type` |
| `craftstack_instance_memory_used_mb` | gauge | `instance`, `node`, `type` |
| `craftstack_instance_memory_limit_mb` | gauge | `instance`, `node`, `type` |
| `craftstack_instance_net_rx_bytes` | counter | `instance`, `node`, `type` |
| `craftstack_instance_net_tx_bytes` | counter | `instance`, `node`, `type` |
| `craftstack_instance_block_read_bytes` | counter | `instance`, `node`, `type` |
| `craftstack_instance_block_write_bytes` | counter | `instance`, `node`, `type` |

## InfluxDB v2

Enable in `configs/master.yaml`:

```yaml
observability:
  influxdb:
    enabled: true
    url: "http://influxdb:8086"
    token: "YOUR_API_TOKEN"
    org: "craftstack"
    bucket: "craftstack"
    interval_ms: 15000
```

The master pushes line-protocol points to `POST /api/v2/write` on the
configured interval. Two measurements are written:

- `craftstack_node` — tags: `node`; fields: `cpu_percent`, `mem_percent`,
  `mem_used_mb`, `mem_total_mb`, `disk_percent`
- `craftstack_instance` — tags: `instance`, `node`, `type`; fields:
  `cpu_percent`, `mem_used_mb`, `mem_limit_mb`, `net_rx`, `net_tx`,
  `blk_r`, `blk_w`

## Grafana (standard flow)

1. Add data source: **Prometheus** → `http://prometheus:9090`
   *or* **InfluxDB** (Flux) → `http://influxdb:8086`, org `craftstack`, bucket `craftstack`.
2. Dashboards → Import → upload `docs/grafana/craftstack-dashboard.json`.
3. Pick the data source you added and Import.

Optional: set `observability.grafana_url` in `master.yaml` to display a
"Open Grafana" link in the CraftStack UI.
