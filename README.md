# End-of-Life Exporter

<p align="center">
  <img src="./assets/endoflife.png" width="90"/>
  <img src="./assets/prometheus.png" width="90"/>
  <br>
</p>

A Prometheus exporter that exposes product versions and their End-of-Life (EOL) dates as metrics using the [endoflife.date](https://endoflife.date) API. Information is fetched only when Prometheus scrapes the `/metrics` endpoint.

## Deployment

### Usage

```bash
Usage: endoflife_exporter [flags]

Prometheus exporter for product versions and their End-of-Life.

Flags:
  -h, --help                    Show context-sensitive help.
      --address=":8080"         The address where the server should listen on ($ADDRESS).
      --config="config.yml"     Configuration file path ($CONFIG_FILE)
      --log.format="console"    Set the output format of the logs. Must be "console" or "json" ($LOG_FORMAT).
      --log.level=INFO          Set the log level. Must be "DEBUG", "INFO", "WARN" or "ERROR" ($LOG_LEVEL).
      --log.add-source          Whether to add source file and line number to log records ($LOG_ADD_SOURCE).
      --version                 Print version information and exit
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

## Configuration

Configure products and their release cycles as shown below.

> 💡 Always verify product names on [endoflife.date](https://endoflife.date/). If you don't specify a release, it defaults to the `latest` available one.

```yaml
---
products:
  - name: mongo
    releases:
      - "8.0"
      - "7.0"
  - name: redis
  - name: ubuntu
    all_releases: true
```

## Prometheus Configuration

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

## Metrics

See [metrics](https://github.com/veerendra2/endoflife_exporter/wiki/Metrics)

## Grafana Dashboard

- [Download Grafana Dashboard Json](./assets/endoflife-grafana-dashboard.json)

![Dashboard Screenshot](./dashboard/dashboard-screenshot.png)

## Development

Install [task](https://taskfile.dev/docs/installation)

```bash
go mod tidy

# Install dependencies
task gen

# Run tests
task test

# List all tasks
task --list
task: Available tasks for this project:
* all:                   Run comprehensive checks: format, lint, security and test
* build:                 Build the application binary for the current platform
* build-docker:          Build Docker image
* build-platforms:       Build the application binaries for multiple platforms and architectures
* fmt:                   Formats all Go source files
* gen:                   Generates Go types from OAPI spec      (aliases: generate)
* install:               Install required tools and dependencies
* lint:                  Run static analysis and code linting using golangci-lint
* run:                   Runs the main application
* security:              Run security vulnerability scan
* test:                  Runs all tests in the project      (aliases: tests)
* vet:                   Examines Go source code and reports suspicious constructs
```

Update endoflife OpenAPI Spec

- Check https://endoflife.date/docs/api/v1/ for changes.
- Update pkg/endoflife/openapi.yaml if needed and run `task gen` again.
- The upstream OpenAPI spec has a type mismatch: `isEoes` is defined as `string` but the API returns `boolean`
  - **Workaround**: Before running `task gen`, manually change the type at [line 564 in openapi.yaml](./pkg/endoflife/openapi.yaml#L564):
    ```yaml
    isEoes:
      type: boolean # Changed from 'string'
      nullable: true
    ```
