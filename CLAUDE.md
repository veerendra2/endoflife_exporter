# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Prometheus exporter that fetches product version End-of-Life (EOL) information from the endoflife.date API and exposes it as Prometheus metrics. Data is fetched on-demand when Prometheus scrapes the `/metrics` endpoint.

## Development Commands

### Prerequisites
- Go 1.26+
- [Task](https://taskfile.dev/) for build automation
- `oapi-codegen` (installed via `task install`)

### Common Commands

```bash
# Install dependencies and tools
task install

# Run the application locally
task run                    # Uses sample.config.yml, console logging

# Testing
task test                   # Run all tests
go test ./internal/config   # Run tests for specific package
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

The app reads `sample.config.yml` by default when using `task run`. Override with environment variables:

```bash
CONFIG_FILE=my-config.yml LOG_LEVEL=debug task run
```

Access metrics at http://localhost:8080/metrics

## Architecture

### Request Flow

1. Prometheus scrapes `/metrics` endpoint
2. `collector.Exporter.Collect()` is invoked
3. For each product in config:
   - If `all_releases: true` → fetch all release cycles via `GetProductDetails()`
   - Otherwise → fetch specific releases via `GetRelease()` for each listed release
4. API responses converted to `ReleaseDetails` structs
5. Four Prometheus metrics emitted per release cycle

### Key Components

**`main.go`**
- Entry point: sets up HTTP server, Prometheus registry, graceful shutdown
- Uses `kong` for CLI parsing, `slog` for structured logging
- Registers `collector.Exporter` as Prometheus collector

**`internal/collector/collector.go`**
- Implements `prometheus.Collector` interface
- Orchestrates API calls per configured product
- Emits four metric types (product_info, latest_version_timestamp, release_cycle_timestamp, eol_from_timestamp)

**`internal/config/config.go`**
- Loads YAML configuration
- Defaults to `["latest"]` release if neither `all_releases` nor `releases` specified
- Warns if both `all_releases` and `releases` are set (ignores `releases`)

**`pkg/endoflife/endoflife.go`**
- HTTP client for endoflife.date API
- Two endpoints: `/products/{name}` (all releases), `/products/{name}/releases/{cycle}` (single release)
- Converts API-generated types to internal `ReleaseDetails`
- Default EOL date: 2050-01-01 (Unix: 2524608000) when API returns null

**`pkg/endoflife/types.go`**
- Auto-generated from `pkg/endoflife/openapi.yaml` via `oapi-codegen`
- **DO NOT EDIT MANUALLY** — regenerate with `task gen`

### Configuration Model

```yaml
products:
  - name: mongo               # Required: product slug from endoflife.date
    releases:                 # Optional: specific release cycles (default: ["latest"])
      - "8.0"
      - "7.0"
  - name: redis               # Defaults to latest release
  - name: ubuntu
    all_releases: true        # Fetch all release cycles (ignores 'releases' field)
```

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

**Known upstream issue:** The API spec incorrectly defines `isEoes` as `string` in some versions, but the API returns `boolean`. This has been corrected in recent versions, but verify line 622-631 in openapi.yaml shows `type: boolean` with `nullable: true`.

## Testing Strategy

- Unit tests use Ginkgo/Gomega framework
- Config parsing tests in `internal/config/config_test.go`
- Run specific test suites: `go test ./internal/config -v`

## Metrics Exposed

All metrics use Unix timestamps (seconds since epoch).

1. **`endoflife_product_info`** (gauge=1): Metadata labels (is_eol, is_lts, is_maintained, latest_version, product_name, release_cycle_name)
2. **`endoflife_latest_version_timestamp_seconds`**: When the latest patch version was released
3. **`endoflife_release_cycle_timestamp_seconds`**: When the release cycle first launched
4. **`endoflife_eol_from_timestamp_seconds`**: When support ends (2050-01-01 if no EOL date)

## Common Tasks

**Add a new metric:**
1. Define `prometheus.NewDesc` in `internal/collector/collector.go` (var block at top)
2. Add to `Exporter.Describe()` method
3. Emit in `Exporter.Collect()` loop using `prometheus.MustNewConstMetric`

**Change API client behavior:**
- Modify `pkg/endoflife/endoflife.go` (e.g., timeouts, error handling)
- If API schema changes, update `openapi.yaml` and run `task gen`

**Update configuration schema:**
1. Modify structs in `internal/config/config.go`
2. Update `sample.config.yml` example
3. Add validation logic in `LoadConfig()` if needed
