# Multi-Target Exporter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the exporter's `products:` config file with a blackbox-style `GET /probe?target=<product>&releases=<cycles|all>` endpoint so Prometheus owns the product list.

**Architecture:** `main.go` stops loading config and registering a global collector. A new `internal/probe` package parses the request, builds a per-request `prometheus.Registry`, fetches one product from endoflife.date, and serves it via `promhttp.HandlerFor`. `/metrics` keeps only Go/process metrics. `internal/config` and `internal/collector` are deleted.

**Tech Stack:** Go 1.26, `prometheus/client_golang` (incl. `prometheus/testutil`), `net/http/httptest`, Ginkgo/Gomega.

**Spec:** `docs/superpowers/specs/2026-11-12-multi-target-design.md`

## Global Constraints

- No new dependencies. `prometheus/testutil` ships with `client_golang`, already required.
- The four existing metric names, help strings, and label sets are unchanged: `endoflife_product_info` (is_eol, is_lts, is_maintained, latest_version, product_name, release_cycle_name), `endoflife_latest_version_timestamp_seconds` (product_name, release_cycle_name, latest_version), `endoflife_release_cycle_timestamp_seconds` (product_name, release_cycle_name), `endoflife_eol_from_timestamp_seconds` (product_name, release_cycle_name).
- Probe failure is HTTP 200 with `endoflife_probe_success 0`. HTTP 400 only for a bad request (missing target, invalid slug, invalid `releases`).
- Probe timeout is 10s; server `WriteTimeout` becomes 15s.
- Tests use Ginkgo/Gomega, matching `internal/config/config_test.go`'s existing style.
- No config file anywhere: no `--config` flag, no `CONFIG_FILE`, no `sample.config.yml`.
- Commits use conventional prefixes (`feat:`, `refactor:`, `docs:`, `chore:`), matching `git log`.

## Review Focus

Each line gets a test in the task that owns the code.

1. `target=../../admin` or `target=mongo/8.0` — must be 400 and never reach the API URL path (Task 2).
2. `releases=8.0,8.0` — duplicate cycles would emit identical series and make Prometheus fail the whole scrape; must dedupe (Task 1).
3. `releases=all,8.0` and `releases=8.0,,7.0` — ambiguous input; must be 400 rather than a silently partial scrape (Task 1).
4. A product the API reports with zero cycles under `releases=all` — `probe_success 1` with no data metrics, not a false failure (Task 2).
5. One of several requested cycles fails — `probe_success 0` while the successful cycles are still emitted, so the target is visibly degraded without losing data (Task 2).

---

### Task 0: Branch

- [ ] **Step 1: Create the feature branch**

```bash
git checkout -b feat/multi-target
```

---

### Task 1: `parseReleases`

**Files:**
- Create: `internal/probe/probe.go`
- Test: `internal/probe/probe_test.go`

**Interfaces:**
- Produces: `parseReleases(param string) (releases []string, all bool, err error)` — package-private, used by `Handler` in Task 2.

- [ ] **Step 1: Write the failing test**

`internal/probe/probe_test.go` — standard Ginkgo suite boilerplate (`TestProbe` + `RunSpecs(t, "Probe Suite")`), then a `DescribeTable` over `parseReleases`:

```go
DescribeTable("parseReleases",
	func(param string, wantReleases []string, wantAll bool, wantErr bool) {
		releases, all, err := parseReleases(param)
		if wantErr {
			Expect(err).To(HaveOccurred())
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(all).To(Equal(wantAll))
		Expect(releases).To(Equal(wantReleases))
	},
	Entry("empty defaults to latest", "", []string{"latest"}, false, false),
	Entry("single cycle", "8.0", []string{"8.0"}, false, false),
	Entry("several cycles", "8.0,7.0", []string{"8.0", "7.0"}, false, false),
	Entry("trims spaces", " 8.0 , 7.0 ", []string{"8.0", "7.0"}, false, false),
	Entry("dedupes", "8.0,8.0,7.0", []string{"8.0", "7.0"}, false, false),
	Entry("all", "all", nil, true, false),
	Entry("rejects empty element", "8.0,,7.0", nil, false, true),
	Entry("rejects all mixed with cycles", "all,8.0", nil, false, true),
	Entry("rejects path traversal", "8.0,../x", nil, false, true),
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/probe/ -v`
Expected: FAIL — `undefined: parseReleases`.

- [ ] **Step 3: Implement `parseReleases` in `internal/probe/probe.go`**

Package `probe`, imports `fmt`, `regexp`, `slices`, `strings`.

```go
// A release cycle name is an endoflife.date API path segment, so it must be a
// plain slug. This also blocks path traversal via the query string.
var slugRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// parseReleases turns the releases query parameter into a fetch plan.
// all=true means fetch every cycle; otherwise releases holds the cycle names.
// shortcut: "all" is a reserved value, rename the param if a cycle is ever named that.
func parseReleases(param string) (releases []string, all bool, err error)
```

Body rules: trim the whole param; empty → `[]string{"latest"}, false, nil`. Split on `,`; trim each; an empty element is an error; `all` must be the only element (`strings.EqualFold` not needed — exact match); every other element must match `slugRe`; sort and `slices.Compact` the result to drop duplicates; return.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/probe/ -v`
Expected: PASS, 9 entries.

- [ ] **Step 5: Commit**

```bash
git add internal/probe/
git commit -m "feat: add probe release parameter parsing"
```

---

### Task 2: `/probe` handler and per-request collector

**Files:**
- Modify: `internal/probe/probe.go`
- Modify: `internal/probe/probe_test.go`
- Modify: `pkg/endoflife/endoflife.go` (interface only)

**Interfaces:**
- Consumes: `parseReleases` (Task 1); `endoflife.Client` with `GetProductDetails(ctx, productName) ([]ReleaseDetails, error)` and `GetRelease(ctx, productName, cycleName) (ReleaseDetails, error)`.
- Produces: `probe(ctx context.Context, client endoflife.Client, product string, releases []string, all bool) *prometheus.Registry` and `Handler(client endoflife.Client) http.Handler` — used by `main.go` in Task 3.

- [ ] **Step 1: Make the client fakeable**

In `pkg/endoflife/endoflife.go`, delete the `doRequest` line from the `Client` interface. It stays a method on `*client`; only the interface changes. Nothing else in the repo calls it through the interface (verify: `rg 'doRequest' --type go` shows only the method definition and `client` internals).

- [ ] **Step 2: Write the failing tests**

Add to `internal/probe/probe_test.go`:

```go
type fakeClient struct {
	getProductDetails func(ctx context.Context, productName string) ([]endoflife.ReleaseDetails, error)
	getRelease        func(ctx context.Context, productName, cycleName string) (endoflife.ReleaseDetails, error)
}

func (f fakeClient) GetProductDetails(ctx context.Context, productName string) ([]endoflife.ReleaseDetails, error) {
	return f.getProductDetails(ctx, productName)
}

func (f fakeClient) GetRelease(ctx context.Context, productName, cycleName string) (endoflife.ReleaseDetails, error) {
	return f.getRelease(ctx, productName, cycleName)
}

func details(cycle string) endoflife.ReleaseDetails {
	return endoflife.ReleaseDetails{
		EOLFrom:           time.Unix(2524608000, 0),
		IsMaintained:      true,
		LatestVersion:     cycle + ".1",
		LatestVersionDate: time.Unix(1700000000, 0),
		ReleaseCycleDate:  time.Unix(1600000000, 0),
		ReleaseCycleName:  cycle,
	}
}
```

Six `probe` specs (call `probe(context.Background(), fake, "mongo", releases, all)` then `testutil.GatherAndCompare(registry, strings.NewReader(expected), "endoflife_probe_success")` etc.):

1. **latest by default** — fake `getRelease` asserts `cycleName == "latest"` and `productName == "mongo"`, returns `details("latest")`. Expected, filtering on `endoflife_probe_success` and `endoflife_product_info`:
```text
# HELP endoflife_probe_success Whether the probe to endoflife.date succeeded.
# TYPE endoflife_probe_success gauge
endoflife_probe_success{product_name="mongo"} 1
# HELP endoflife_product_info Product release cycle information with EOL status, LTS flag, and maintenance state.
# TYPE endoflife_product_info gauge
endoflife_product_info{is_eol="false",is_lts="false",is_maintained="true",latest_version="latest.1",product_name="mongo",release_cycle_name="latest"} 1
```
2. **requested cycles** — `["8.0","7.0"]`; fake returns `details(cycle)`; expect two `endoflife_product_info` lines (`8.0`, `7.0`) and `probe_success 1`.
3. **all cycles** — `all=true`; fake `getProductDetails` returns `details("22.04")`, `details("24.04")`; `getRelease` panics if called; expect two `product_info` lines and `probe_success 1`.
4. **API failure** — `getRelease` returns `errors.New("boom")`; expect `endoflife_probe_success{product_name="mongo"} 0` and no `endoflife_product_info` family (pass only `endoflife_probe_success` to `GatherAndCompare`).
5. **partial failure** — `getRelease` returns `details("8.0"), nil` for `"8.0"` and an error for `"7.0"`; expect `probe_success 0` plus the `8.0` `product_info` line.
6. **no cycles reported** — `all=true`, `getProductDetails` returns empty slice, nil error; expect `probe_success 1` and no data metrics.

Four `Handler` specs using `httptest.NewRecorder` + `httptest.NewRequest("GET", "/probe?...", nil)`:
7. missing `target` → 400.
8. `target=../x` → 400, and the fake must not be called.
9. `releases=all,8.0` → 400.
10. `target=redis` → 200 and body contains `endoflife_probe_success{product_name="redis"} 1`.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/probe/ -v`
Expected: FAIL — `undefined: probe`, `undefined: Handler`.

- [ ] **Step 4: Implement the collector, `probe`, and `Handler`**

Move the four `prometheus.NewDesc` vars from `internal/collector/collector.go` into `internal/probe/probe.go` verbatim (names, help strings, label order unchanged). Add:

```go
var (
	probeSuccessDesc = prometheus.NewDesc(
		"endoflife_probe_success",
		"Whether the probe to endoflife.date succeeded.",
		[]string{"product_name"}, nil,
	)
	probeDurationDesc = prometheus.NewDesc(
		"endoflife_probe_duration_seconds",
		"Duration of the probe in seconds.",
		[]string{"product_name"}, nil,
	)
)

// productCollector emits the data metrics for one product.
type productCollector struct {
	product  string
	releases []endoflife.ReleaseDetails
}

// probeCollector emits the per-probe outcome metrics.
type probeCollector struct {
	product  string
	success  bool
	duration float64
}

// probe scrapes one product and returns a registry holding its metrics.
func probe(ctx context.Context, client endoflife.Client, product string, releases []string, all bool) *prometheus.Registry

// Handler serves GET /probe?target=<product>&releases=<cycles|all>.
func Handler(client endoflife.Client) http.Handler
```

`probe`: `prometheus.NewRegistry()`; `start := time.Now()`; fetch (`GetProductDetails` when `all`, else one `GetRelease` per cycle, logging each failure with `slog.Error` and keeping the successful results); `success := len(errs) == 0`; register `productCollector` only when at least one release was fetched; register `probeCollector`; return the registry.

`productCollector.Collect` is today's `internal/collector/collector.go` loop body, with `product.Name` replaced by `c.product`. `Describe` sends the four descs.

`Handler`: read `target`; 400 when empty or `!slugRe.MatchString(target)`; `parseReleases(r.URL.Query().Get("releases"))`, 400 on error; `ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)`; `promhttp.HandlerFor(probe(ctx, client, target, releases, all), promhttp.HandlerOpts{}).ServeHTTP(w, r)`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/probe/ -v`
Expected: PASS, 19 entries/specs total.

- [ ] **Step 6: Commit**

```bash
git add internal/probe/ pkg/endoflife/endoflife.go
git commit -m "feat: add multi-target /probe endpoint"
```

---

### Task 3: Wire `main.go`, delete config and collector

**Files:**
- Modify: `main.go`
- Delete: `internal/collector/collector.go`, `internal/config/config.go`, `internal/config/config_test.go`, `sample.config.yml`
- Modify: `Taskfile.yml:24-28` (the `run` task's `env` block)

**Interfaces:**
- Consumes: `probe.Handler(endoflife.Client) http.Handler` (Task 2), `endoflife.NewClient() (Client, error)`.

- [ ] **Step 1: Rewrite `main.go`**

Remove the `Config string` field from the `cli` struct and the `config`/`collector` imports. After the `slog` setup:

```go
eolClient, err := endoflife.NewClient()
if err != nil {
	slog.Error("Failed to create endoflife.date client", "error", err)
	os.Exit(1)
}
```

Routes: `/` body becomes `Probing is available at /probe?target=&lt;product&gt;, metrics at <a href="/metrics">/metrics</a>`; add `http.Handle("/probe", probe.Handler(eolClient))`; keep `http.Handle("/metrics", promhttp.Handler())`. Drop `prometheus.MustRegister` and the `prometheus` import. Set `WriteTimeout: 15 * time.Second` with the comment `// Must exceed the probe's 10s timeout, or the result never reaches Prometheus.`

- [ ] **Step 2: Delete the old config and collector**

```bash
git rm -r internal/config internal/collector sample.config.yml
```

- [ ] **Step 3: Drop `CONFIG_FILE` from `task run`**

In `Taskfile.yml`, remove the `CONFIG_FILE: ./sample.config.yml` line from the `run` task's `env` block.

- [ ] **Step 4: Tidy and build**

```bash
go mod tidy
go build ./...
go vet ./...
```
Expected: all succeed. `gopkg.in/yaml.v3` disappears from `go.mod` (its only user was `internal/config`).

- [ ] **Step 5: Run the whole test suite**

Run: `go test ./...`
Expected: PASS. `internal/config` no longer appears in the package list.

- [ ] **Step 6: Smoke-test against the live API**

```bash
go run . & sleep 3
curl -s 'localhost:8080/probe?target=redis&releases=latest' | head -20
curl -s 'localhost:8080/probe?target=ubuntu&releases=all' | grep -c endoflife_product_info
curl -s -o /dev/null -w '%{http_code}\n' 'localhost:8080/probe?target=../x'
curl -s -o /dev/null -w '%{http_code}\n' 'localhost:8080/probe?target=nope-nope-nope'
curl -s 'localhost:8080/metrics' | grep -c '^endoflife_' || true
kill %1
```
Expected: real metric lines for redis and many for ubuntu; `400` for `../x`; `200` for the unknown product with `endoflife_probe_success{product_name="nope-nope-nope"} 0`; `0` matches for `^endoflife_` on `/metrics`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: drive the exporter from /probe instead of a config file"
```

---

### Task 4: Docs and dev stack

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `compose.yml`
- Modify: `compose.dev.yml`

**Interfaces:**
- Consumes: the endpoint contract from Task 2.

- [ ] **Step 1: Update `README.md`**

- Intro paragraph: mention the Prometheus multi-target pattern and link the guide.
- Add a `> **Breaking change:**` note under the title pointing at the `products:` → `/probe` migration.
- Usage block: remove `--config="config.yml"` from the flags listing.
- Docker Compose example: drop `CONFIG_FILE` and the `./config.yml:/config.yml` volume.
- Replace the `## Configuration` section with `## Probing`: a table of the two query params (`target` required, product slug; `releases` optional, comma-separated cycles or `all`, default `latest`), two `curl` examples, and the 200-vs-400 rule (`endoflife_probe_success 0` on API failure).
- Replace the `## Prometheus Configuration` yaml block with exactly this:

```yaml
scrape_configs:
  - job_name: "endoflife_exporter"        # the exporter itself
    static_configs:
      - targets: ["endoflife_exporter:8080"]

  - job_name: "endoflife_products"        # one scrape per product
    metrics_path: /probe
    scrape_interval: 168h
    scrape_timeout: 30s
    static_configs:
      - targets: ["mongo"]                # product slug from endoflife.date
        labels:
          releases: "8.0,7.0"
      - targets: ["redis"]                # defaults to latest
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

- After it, one line explaining the four relabel rules (target → `__param_target` → `instance`; `releases` label → `__param_releases`; `__address__` → the exporter) and one line that `up` is per product.
- Alerting example: leave unchanged (`product_name` still exists).
- Metrics section: add the two probe metrics to the list before linking the wiki.

- [ ] **Step 2: Update `CLAUDE.md`**

Rewrite the parts that describe the old model: Project Overview (data fetched per `/probe` request), Request Flow (Prometheus → `/probe?target=&releases=` → per-request registry), Key Components (`internal/probe` replaces `internal/collector`; `internal/config` is gone), Configuration Model (delete; point at the README's Prometheus block), Metrics Exposed (add the probe metrics), Common Tasks ("Add a new metric" now edits `internal/probe/probe.go`; "Update configuration schema" is deleted). Keep the OpenAPI spec maintenance section as is.

- [ ] **Step 3: Update `compose.yml`**

Drop the `CONFIG_FILE` environment variable and the `./config.yml:/config.yml` volume from the `endoflife_exporter` service.

- [ ] **Step 4: Update `compose.dev.yml`**

Drop the `CONFIG_FILE` env var and the `./sample.config.yml:/config.yml` volume from the `endoflifeexporter` service. In the embedded `prometheus.yml` config, keep the existing `endoflifeexporter` job and add the `endoflife_products` job from Step 1 with `replacement: endoflifeexporter:8080` and the same three static targets.

- [ ] **Step 5: Verify the dev stack**

```bash
docker compose -f compose.dev.yml up -d
sleep 20
curl -s 'localhost:9090/api/v1/targets?state=active' | grep -o '"health":"[a-z]*"' | sort | uniq -c
docker compose -f compose.dev.yml down
```
Expected: all targets `"health":"up"` (four: prometheus, grafana is not scraped, `endoflifeexporter`, and the three product instances). If Docker is unavailable, say so instead of skipping silently.

- [ ] **Step 6: Commit**

```bash
git add README.md CLAUDE.md compose.yml compose.dev.yml
git commit -m "docs: document the multi-target probe endpoint"
```

---

### Task 5: Full gate

- [ ] **Step 1: Run every check**

Run: `task all`
Expected: fmt, lint, vet, security, gen, test all pass with no diff after `task fmt`.

- [ ] **Step 2: Confirm nothing references the old model**

Run: `rg -n 'products:|CONFIG_FILE|--config|internal/config|internal/collector|sample\.config' --glob '!docs/**' --glob '!go.sum'`
Expected: no matches outside `docs/superpowers/`.

- [ ] **Step 3: Commit any formatting leftovers**

```bash
git add -A
git commit -m "chore: format after multi-target refactor"
```
Only if `task all` produced changes.
