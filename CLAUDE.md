# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Prometheus exporter that fetches product version End-of-Life (EOL) information from the endoflife.date API and exposes it as Prometheus metrics. It follows the Prometheus multi-target exporter pattern: Prometheus names the product in the `/probe` request and data is fetched on demand for that one product.

## Development Commands

### Prerequisites
- Go 1.27+
- [Task](https://taskfile.dev/) for build automation
- `oapi-codegen` (installed via `task install`)

### Common Commands

```bash
# Install dependencies and tools
task install

# Run the application locally
task run                    # Console logging, listens on :8080

# Testing
task test                   # Run all tests
go test ./internal/probe    # Run tests for specific package
go test -v ./...            # Verbose test output

# Code quality
task fmt                    # Format all Go code
task vet                    # Run go vet
task lint                   # Run golangci-lint
task security               # Run govulncheck for vulnerabilities
task all                    # Run fmt, lint, vet, security, gen, test

# Build
task build                  # Build for current platform (output: dist/endoflife_exporter)
task build-platforms        # Build for linux/darwin on amd64/arm64
task build-docker           # Build Docker image

# Code generation
task gen                    # Regenerate Go types from OpenAPI spec
```

### Running Locally

The app takes no configuration file. Override the listen address with environment variables:

```bash
ADDRESS=:9090 LOG_LEVEL=debug task run
```

- Exporter's own metrics: http://localhost:8080/metrics
- Probe a product: http://localhost:8080/probe?target=redis

## Architecture

### Request Flow

1. Prometheus scrapes `/probe?target=<product>&releases=<cycles|all>`
2. `probe.Handler` validates `target` and `releases` (400 on a bad request)
3. `probe.probe()` builds a per-request `prometheus.Registry`
4. `fetch()` calls `GetProductDetails()` for `releases=all`, otherwise `GetRelease()` per cycle; API responses become `ReleaseDetails` structs
5. Four data metrics per release cycle plus `endoflife_probe_success`/`endoflife_probe_duration_seconds` are served by `promhttp.HandlerFor`

A failed API call is not an HTTP error: the response is `200` with `endoflife_probe_success 0`, so Prometheus's `up` tracks the exporter and `probe_success` tracks the product.

### Key Components

**`main.go`**
- Entry point: sets up HTTP server, routes (`/`, `/probe`, `/metrics`), graceful shutdown
- Uses `kong` for CLI parsing, `slog` for structured logging
- Creates the endoflife.date client and passes it to `probe.Handler`

**`internal/probe/probe.go`**
- `Handler()`: HTTP entry point, query param validation, 10s probe timeout
- `parseReleases()`: turns the `releases` param into a fetch plan (`latest` default, `all`, or an explicit list)
- `probe()`/`fetch()`: per-request registry and API calls
- `productCollector`/`probeCollector`: emit the data and probe metrics
- Metric descriptors live in the var block at the top of the file

**`pkg/endoflife/endoflife.go`**
- HTTP client for endoflife.date API
- Two endpoints: `/products/{name}` (all releases), `/products/{name}/releases/{cycle}` (single release)
- Converts API-generated types to internal `ReleaseDetails`
- Default EOL date: 2050-01-01 (Unix: 2524608000) when API returns null
- `Client` is an interface so tests can fake it; keep it free of unexported methods

**`pkg/endoflife/types.go`**
- Auto-generated from `pkg/endoflife/openapi.yaml` via `oapi-codegen`
- **DO NOT EDIT MANUALLY** — regenerate with `task gen`

### Probing Model

There is no config file. Prometheus owns the product list:

| Parameter  | Required | Description |
|------------|----------|-------------|
| `target`   | yes      | Product slug from endoflife.date, validated against `^[A-Za-z0-9][A-Za-z0-9._-]*$` |
| `releases` | no       | Comma-separated cycles, or `all`. Defaults to `latest` |

See the README's Prometheus scrape config for the relabeling that maps `targets` to `__param_target`/`__param_releases`.

## OpenAPI Spec Maintenance

The `pkg/endoflife/openapi.yaml` spec is sourced from https://endoflife.date/docs/api/v1/.

**Upstream OpenAPI version:** The upstream spec uses OpenAPI 3.1, but oapi-codegen only supports OpenAPI 3.0. When updating the spec, it must be converted from 3.1 to 3.0 format.

**Conversion steps:**
1. Download the latest spec from https://endoflife.date/docs/api/v1/
2. Change `openapi: 3.1.x` to `openapi: 3.0.3`
3. Convert all nullable types from 3.1 array syntax to 3.0 `nullable: true`:
   - `type: [string, "null"]` → `type: string` + `nullable: true`
   - `type: [boolean, "null"]` → `type: boolean` + `nullable: true`
   - `anyOf: [{$ref: ...}, {type: "null"}]` → `allOf: [{$ref: ...}]` + `nullable: true`
4. Run `task gen` to regenerate `types.go`
5. If code breaks, check `pkg/endoflife/endoflife.go` for usage of changed field types (especially pointer vs non-pointer changes)

**Known upstream issue:** The API spec incorrectly defines `isEoes` as `string` in some versions, but the API returns `boolean`. This has been corrected in recent versions; verify the `isEoes` block in openapi.yaml still reads `type: boolean` with `nullable: true`.

## Testing Strategy

- Unit tests use Ginkgo/Gomega framework
- Probe handler, parsing, and probe-outcome tests in `internal/probe/probe_test.go` (fake `endoflife.Client`)
- Run specific test suites: `go test ./internal/probe -v`

## Metrics Exposed

All metrics use Unix timestamps (seconds since epoch).

1. **`endoflife_product_info`** (gauge=1): Metadata labels (is_eol, is_lts, is_maintained, latest_version, product_name, release_cycle_name)
2. **`endoflife_latest_version_timestamp_seconds`**: When the latest patch version was released
3. **`endoflife_release_cycle_timestamp_seconds`**: When the release cycle first launched
4. **`endoflife_eol_from_timestamp_seconds`**: When support ends (2050-01-01 if no EOL date)
5. **`endoflife_probe_success`**: 1 when the probe reached endoflife.date, 0 otherwise
6. **`endoflife_probe_duration_seconds`**: How long the probe took

## Common Tasks

**Add a new metric:**
1. Define `prometheus.NewDesc` in the var block in `internal/probe/probe.go`
2. Add it to the matching collector's `Describe()` method
3. Emit it in that collector's `Collect()` method with `prometheus.MustNewConstMetric`

**Change API client behavior:**
- Modify `pkg/endoflife/endoflife.go` (e.g., timeouts, error handling)
- If API schema changes, update `openapi.yaml` and run `task gen`

**Add a query parameter:**
1. Parse and validate it in `probe.Handler` (return 400 on bad input)
2. Thread it through `probe()`/`fetch()`
3. Add a spec in `internal/probe/probe_test.go` and document it in the README's Probing table
