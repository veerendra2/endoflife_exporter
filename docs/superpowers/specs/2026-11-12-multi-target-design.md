# Multi-target exporter pattern

Issue: [#65](https://github.com/veerendra2/endoflife_exporter/issues/65)
Prior art: [shelly_device_exporter#29](https://github.com/veerendra2/shelly_device_exporter/issues/29), [Prometheus guide](https://prometheus.io/docs/guides/multi-target-exporter/)

## Intent

Today the product list lives in the exporter's `config.yml`. Adding a product needs a
config edit and an exporter restart, the `instance` label is the exporter (not the
product), and a failed API call silently drops series instead of marking a target down.

Multi-target fixes all three: Prometheus owns the target list, `instance` is the
product, and `endoflife_probe_success` reports per-product failure.

## Goals

- `GET /probe?target=<product>` scrapes one product on demand and returns its metrics.
- Product list, release selection, and `instance` label all come from `prometheus.yml`.
- Per-product success/failure signal, blackbox-style.
- No exporter config file, no restart to add a product.

## Non-goals

- Keeping the old `products:` config mode. Breaking change, same as shelly #29.
- Caching API responses between scrapes.
- Changing the four existing `endoflife_*` metric names or their labels.
- Per-product API base URL or auth (the public API needs none).

## Anti-goals

- A new config-file format that re-lists products in the exporter.
- Magic `module` names that duplicate what a query param can express.

## Approaches

**A. Config-file modules** (blackbox-style `modules:`, closest to shelly's auth modules).
Keeps `CONFIG_FILE` and a place for defaults; every product still needs a config entry,
so the "add a product without touching the exporter" win is only half-realised.

**B. Query params only — picked.** The release selection becomes request parameters, so
`prometheus.yml` carries everything. Deletes `internal/config`, `sample.config.yml`, the
`--config` flag, and the compose config mounts. The exporter becomes stateless.

## Design

### Endpoint

```
GET /probe?target=mongo&releases=8.0,7.0
GET /probe?target=ubuntu&releases=all
GET /probe?target=redis              # default: latest
```

- `target` — endoflife.date product slug. Required; missing → `400`.
- `releases` — comma-separated release cycles. `all` fetches every cycle
  (`GetProductDetails`). Absent or empty → `latest`.
  `shortcut: "all" is a reserved value; no real cycle is named that. Rename the param if the API ever ships one.`
- Both `target` and every cycle name must match `^[A-Za-z0-9][A-Za-z0-9._-]*$` (they become API
  path segments); anything else is `400`. This is the trust boundary, so it is validated in the
  handler rather than escaped later.
- Unknown product / API error → `200` with `endoflife_probe_success 0` and no data
  metrics, so `up` tracks the exporter and `probe_success` tracks the product.
- Any requested cycle failing sets `probe_success 0`; cycles that did succeed are still
  emitted.

### Metrics

Unchanged: `endoflife_product_info`, `endoflife_latest_version_timestamp_seconds`,
`endoflife_release_cycle_timestamp_seconds`, `endoflife_eol_from_timestamp_seconds`.

Added: `endoflife_probe_success` (gauge, labels: `product_name`),
`endoflife_probe_duration_seconds` (gauge, labels: `product_name`).

`product_name` stays on the four existing metrics even though it now duplicates
`instance` — dropping it would break existing queries and dashboards for no gain.

### Code

| Path | Change |
|------|--------|
| `internal/probe/probe.go` | New. `Handler(client endoflife.Client) http.Handler`, per-request registry, release parsing, probe metrics. Absorbs today's collector loop. |
| `internal/probe/probe_test.go` | New. Fake `endoflife.Client`: default release, explicit list, `all`, missing target (400), API error (`probe_success 0`). |
| `internal/collector/` | Deleted — its only job was iterating configured products. |
| `internal/config/` | Deleted, with `config_test.go` and `sample.config.yml`. |
| `pkg/endoflife/endoflife.go` | Drop the unexported `doRequest` from the `Client` interface so tests can fake it. |
| `main.go` | Drop config loading and the `Exporter` registration; add `/probe`; `/metrics` keeps only Go/process metrics. `WriteTimeout` 10s → 15s so a 10s probe can still write its result. |
| `Taskfile.yml` | Drop `CONFIG_FILE` from `task run`. |
| `README.md`, `CLAUDE.md` | Rewrite config + Prometheus sections around `/probe` and relabeling. |
| `compose.yml`, `compose.dev.yml` | Drop the config mount; dev Prometheus gets a `/probe` job with relabeling. |

### Prometheus config (README and dev stack)

```yaml
scrape_configs:
  - job_name: endoflife            # the exporter itself
    static_configs:
      - targets: ["endoflife_exporter:8080"]

  - job_name: endoflife-products
    metrics_path: /probe
    scrape_interval: 168h
    scrape_timeout: 30s
    static_configs:
      - targets: ["mongo"]
        labels:
          releases: "8.0,7.0"
      - targets: ["redis"]
      - targets: ["ubuntu"]
        labels:
          releases: "all"
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - source_labels: [releases]
        target_label: __param_releases
      - target_label: __address__
        replacement: endoflife_exporter:8080
```

### Timeout

`context.WithTimeout(r.Context(), 10*time.Second)`. Prometheus's own `scrape_timeout`
already cancels `r.Context()`; the scrape-timeout-header plumbing shelly uses is not
needed for a single fast HTTP call. Revisit if `releases=all` on a large product gets
close to 10s.

## My calls (check these while reading)

- `releases=all` as one param instead of a separate `all_releases=true` boolean.
- `probe_success` labelled by `product_name` rather than `target`, matching the existing
  metric family.
- Probe metrics live in `internal/probe`, not a separate `internal/collector`.
- Fixed 10s probe timeout instead of reading `X-Prometheus-Scrape-Timeout-Seconds`.
