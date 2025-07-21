# ⏳ End-of-Life Exporter

A Prometheus exporter that exposes product versions and their End-of-Life (EOL) dates as metrics using the [endoflife.date](https://endoflife.date) API. Information is fetched only when Prometheus scrapes the `/metrics` endpoint.

## ⚙️ Configuration

Configure products and their release cycles as shown below.

> 💡 Always verify product names on [endoflife.date](https://endoflife.date/). If you don't specify a release, it defaults to the `latest` available one.

```yaml
---
products:
  - name: mongo
    releases:
      - "8.0"
```

## 🚀 Deployment

### Usage

```bash
Usage: endoflife_exporter [flags]

Flags:
  -h, --help                   Show context-sensitive help.
      --address=":8080"        The address where the server should listen on ($ADDRESS).
      --config="config.yml"    Configuration file path ($CONFIG_FILE)
      --log.format="json"      Set the output format of the logs. Must be "console" or "json" ($LOG_FORMAT).
      --log.level=INFO         Set the log level. Must be "DEBUG", "INFO", "WARN" or "ERROR" ($LOG_LEVEL).
      --log.add-source         Whether to add source file and line number to log records ($LOG_ADD_SOURCE).
```

### Docker Compose

```yaml
---
services:
  endoflife_exporter:
    image: ghcr.io/veerendra2/endoflife_exporter:latest
    container_name: endoflife_exporter
    restart: unless-stopped
    environment:
      ADDRESS: ":8080"
      CONFIG_FILE: "/config.yml"
      LOG_FORMAT: "console"
    volumes:
      - ./config.yml:/config.yml
    ports:
      - 8080:8080
```

```bash
docker compose up -d
```

## 🔥 Prometheus Configuration

Below is an example scrape configuration for Prometheus.

```yaml
---
scrape_configs:
  - job_name: "endoflife_exporter"
    scrape_interval: 168h # Every week
    scrape_timeout: 2m # If you have a long list of products, you might want to increase the timeout.
    static_configs:
      - targets: ["endoflife_exporter:8080"]
```

### Alerting Rules Example

```yaml
groups:
  - name: endoflife.rules
    rules:
      - alert: ProductVersionEOLReachingSoon
        expr: avg by (product_name, release_cycle_name) (endoflife_eol_from_timestamp_seconds - time()) < (21 * 24 * 3600)
        for: 1h
        labels:
          severity: error
        annotations:
          message: 'Product ''{{ $labels.product_name }}'' release cycle ''{{ $labels.release_cycle_name }}'' reached its End-of-Life on {{ ($value | timestamp "2006-01-02") }}.'
```

## 📊 Metrics

Here are some example metrics exposed by the exporter:

```
# HELP endoflife_eol_from_timestamp_seconds Unix timestamp when a product's release cycle reaches its End-of-Life (EOL) or maintenance end.
# TYPE endoflife_eol_from_timestamp_seconds gauge
endoflife_eol_from_timestamp_seconds{product_name="mongo",release_cycle_name="8.0"} 0

# HELP endoflife_latest_version_timestamp_seconds Unix timestamp of the latest version release date for a product's release cycle.
# TYPE endoflife_latest_version_timestamp_seconds gauge
endoflife_latest_version_timestamp_seconds{product_name="mongo",release_cycle_name="8.0"} 1.7527104e+09

# HELP endoflife_product_info Information about the End-of-Life (EOL) status and details of a product.
# TYPE endoflife_product_info gauge
endoflife_product_info{is_eol="false",is_lts="false",is_maintained="true",latest_version="8.0.12",product_name="mongo",release_cycle_name="8.0"} 1

# HELP endoflife_release_cycle_timestamp_seconds Unix timestamp of the release cycle's official release date.
# TYPE endoflife_release_cycle_timestamp_seconds gauge
endoflife_release_cycle_timestamp_seconds{product_name="mongo",release_cycle_name="8.0"} 1.7278272e+09
```
