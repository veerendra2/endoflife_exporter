package probe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/veerendra2/endoflife_exporter/pkg/endoflife"
)

const probeTimeout = 10 * time.Second

// A release cycle name is an endoflife.date API path segment, so it must be a
// plain slug. This also blocks path traversal via the query string.
var slugRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var (
	EndOfLifeProductInfoDesc = prometheus.NewDesc(
		"endoflife_product_info",
		"Product release cycle information with EOL status, LTS flag, and maintenance state.",
		[]string{
			"is_eol",
			"is_lts",
			"is_maintained",
			"latest_version",
			"product_name",
			"release_cycle_name",
		}, nil,
	)
	EndOfLifeLatestVersionTimestampSecondsDesc = prometheus.NewDesc(
		"endoflife_latest_version_timestamp_seconds",
		"Release date of the latest version in the release cycle in Unix timestamp.",
		[]string{
			"product_name",
			"release_cycle_name",
			"latest_version",
		}, nil,
	)
	EndOfLifeReleaseCycleTimestampSecondsDesc = prometheus.NewDesc(
		"endoflife_release_cycle_timestamp_seconds",
		"Initial release date of the release cycle in Unix timestamp.",
		[]string{
			"product_name",
			"release_cycle_name",
		}, nil,
	)
	EndOfLifeEolFromTimestampSecondsDesc = prometheus.NewDesc(
		"endoflife_eol_from_timestamp_seconds",
		"End-of-life date when the release cycle support ends in Unix timestamp.",
		[]string{
			"product_name",
			"release_cycle_name",
		}, nil,
	)
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

// parseReleases turns the releases query parameter into a fetch plan.
// all=true means fetch every cycle; otherwise releases holds the cycle names.
// shortcut: "all" is a reserved value, rename the param if a cycle is ever named that.
func parseReleases(param string) (releases []string, all bool, err error) {
	param = strings.TrimSpace(param)
	if param == "" {
		return []string{"latest"}, false, nil
	}

	parts := strings.Split(param, ",")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false, fmt.Errorf("empty release cycle in %q", param)
		}
		parts[i] = part
	}

	if len(parts) > 1 {
		for _, part := range parts {
			if part == "all" {
				return nil, false, fmt.Errorf(`"all" cannot be combined with release cycles: %q`, param)
			}
		}
	}
	if parts[0] == "all" {
		return nil, true, nil
	}

	// Dedupe while keeping the requested order: duplicate cycles would emit
	// identical series and make Prometheus fail the whole scrape.
	seen := make(map[string]struct{}, len(parts))
	unique := make([]string, 0, len(parts))
	for _, part := range parts {
		if !slugRe.MatchString(part) {
			return nil, false, fmt.Errorf("invalid release cycle name %q", part)
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		unique = append(unique, part)
	}

	return unique, false, nil
}

// productCollector emits the data metrics for one product.
type productCollector struct {
	product  string
	releases []endoflife.ReleaseDetails
}

func (c *productCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- EndOfLifeProductInfoDesc
	ch <- EndOfLifeLatestVersionTimestampSecondsDesc
	ch <- EndOfLifeReleaseCycleTimestampSecondsDesc
	ch <- EndOfLifeEolFromTimestampSecondsDesc
}

func (c *productCollector) Collect(ch chan<- prometheus.Metric) {
	for _, relInfo := range c.releases {
		ch <- prometheus.MustNewConstMetric(
			EndOfLifeProductInfoDesc,
			prometheus.GaugeValue,
			1,
			strconv.FormatBool(relInfo.IsEol),
			strconv.FormatBool(relInfo.IsLts),
			strconv.FormatBool(relInfo.IsMaintained),
			relInfo.LatestVersion,
			c.product,
			relInfo.ReleaseCycleName,
		)

		ch <- prometheus.MustNewConstMetric(
			EndOfLifeLatestVersionTimestampSecondsDesc,
			prometheus.GaugeValue,
			float64(relInfo.LatestVersionDate.Unix()),
			c.product,
			relInfo.ReleaseCycleName,
			relInfo.LatestVersion,
		)

		ch <- prometheus.MustNewConstMetric(
			EndOfLifeReleaseCycleTimestampSecondsDesc,
			prometheus.GaugeValue,
			float64(relInfo.ReleaseCycleDate.Unix()),
			c.product,
			relInfo.ReleaseCycleName,
		)

		ch <- prometheus.MustNewConstMetric(
			EndOfLifeEolFromTimestampSecondsDesc,
			prometheus.GaugeValue,
			float64(relInfo.EOLFrom.Unix()),
			c.product,
			relInfo.ReleaseCycleName,
		)
	}
}

// probeCollector emits the per-probe outcome metrics.
type probeCollector struct {
	product  string
	success  bool
	duration float64
}

func (c *probeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- probeSuccessDesc
	ch <- probeDurationDesc
}

func (c *probeCollector) Collect(ch chan<- prometheus.Metric) {
	success := 0.0
	if c.success {
		success = 1.0
	}
	ch <- prometheus.MustNewConstMetric(probeSuccessDesc, prometheus.GaugeValue, success, c.product)
	ch <- prometheus.MustNewConstMetric(probeDurationDesc, prometheus.GaugeValue, c.duration, c.product)
}

// probe scrapes one product and returns a registry holding its metrics.
func probe(ctx context.Context, client endoflife.Client, product string, releases []string, all bool) *prometheus.Registry {
	start := time.Now()
	details, err := fetch(ctx, client, product, releases, all)

	registry := prometheus.NewRegistry()
	if len(details) > 0 {
		registry.MustRegister(&productCollector{product: product, releases: details})
	}
	registry.MustRegister(&probeCollector{product: product, success: err == nil, duration: time.Since(start).Seconds()})
	return registry
}

// fetch returns every release it could read, plus an error when any request failed.
func fetch(ctx context.Context, client endoflife.Client, product string, releases []string, all bool) ([]endoflife.ReleaseDetails, error) {
	if all {
		details, err := client.GetProductDetails(ctx, product)
		if err != nil {
			slog.Error("Failed to get all release cycles", "product_name", product, "error", err)
			return nil, err
		}
		return details, nil
	}

	var (
		details []endoflife.ReleaseDetails
		errs    []error
	)
	for _, releaseName := range releases {
		relInfo, err := client.GetRelease(ctx, product, releaseName)
		if err != nil {
			slog.Error("Failed to get release cycle", "product_name", product, "release_name", releaseName, "error", err)
			errs = append(errs, err)
			continue
		}
		details = append(details, relInfo)
	}
	return details, errors.Join(errs...)
}

// Handler serves GET /probe?target=<product>&releases=<cycles|all>.
// A failed scrape is not an HTTP error: the response is still 200 and
// endoflife_probe_success reports the outcome (blackbox convention).
func Handler(client endoflife.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := strings.TrimSpace(r.URL.Query().Get("target"))
		if target == "" {
			http.Error(w, "missing target parameter", http.StatusBadRequest)
			return
		}
		if !slugRe.MatchString(target) {
			http.Error(w, "invalid target: must be a product slug from endoflife.date", http.StatusBadRequest)
			return
		}

		releases, all, err := parseReleases(r.URL.Query().Get("releases"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
		defer cancel()

		promhttp.HandlerFor(probe(ctx, client, target, releases, all), promhttp.HandlerOpts{}).ServeHTTP(w, r)
	})
}
