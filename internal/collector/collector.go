package collector

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veerendra2/endoflife_exporter/internal/config"
	"github.com/veerendra2/endoflife_exporter/pkg/endoflife"
)

var (
	EndOfLifeProductInfoDesc = prometheus.NewDesc(
		"endoflife_product_info",
		"Information about the End-of-Life (EOL) status and details of a product.",
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
		"Unix timestamp of the latest version release date for a product's release cycle.",
		[]string{
			"product_name",
			"release_cycle_name",
		}, nil,
	)
	EndOfLifeReleaseCycleTimestampSecondsDesc = prometheus.NewDesc(
		"endoflife_release_cycle_timestamp_seconds",
		"Unix timestamp of the release cycle's official release date.",
		[]string{
			"product_name",
			"release_cycle_name",
		}, nil,
	)
	EndOfLifeEolFromTimestampSecondsDesc = prometheus.NewDesc(
		"endoflife_eol_from_timestamp_seconds",
		"Unix timestamp when a product's release cycle reaches its End-of-Life (EOL) or maintenance end.",
		[]string{
			"product_name",
			"release_cycle_name",
		}, nil,
	)
)

type Exporter struct {
	config    *config.Config
	eolClient endoflife.Client
}

func NewExporter(cfg config.Config) (*Exporter, error) {
	ec, err := endoflife.NewClient()
	if err != nil {
		return nil, err
	}
	return &Exporter{
		config:    &cfg,
		eolClient: ec,
	}, nil
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- EndOfLifeProductInfoDesc
	ch <- EndOfLifeLatestVersionTimestampSecondsDesc
	ch <- EndOfLifeReleaseCycleTimestampSecondsDesc
	ch <- EndOfLifeEolFromTimestampSecondsDesc
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, product := range e.config.Products {
		var releases []endoflife.ReleaseDetails
		var err error

		if product.AllReleases {
			// Fetch all release cycles for the product
			releases, err = e.eolClient.GetProductDetails(ctx, product.Name)
			if err != nil {
				slog.Error("Failed to get all release cycles", "product_name", product.Name, "err", err)
				continue
			}
		} else {
			// Fetch specific releases
			for _, releaseName := range product.Releases {
				relInfo, err := e.eolClient.GetRelease(ctx, product.Name, releaseName)
				if err != nil {
					slog.Error("Failed to get release cycle", "product_name", product.Name, "release_name", releaseName, "err", err)
					continue
				}
				releases = append(releases, relInfo)
			}
		}

		// Process and export metrics for all releases
		for _, relInfo := range releases {
			ch <- prometheus.MustNewConstMetric(
				EndOfLifeProductInfoDesc,
				prometheus.GaugeValue,
				1,
				strconv.FormatBool(relInfo.IsEol),
				strconv.FormatBool(relInfo.IsLts),
				strconv.FormatBool(relInfo.IsMaintained),
				relInfo.LatestVersion,
				product.Name,
				relInfo.ReleaseCycleName,
			)

			ch <- prometheus.MustNewConstMetric(
				EndOfLifeLatestVersionTimestampSecondsDesc,
				prometheus.GaugeValue,
				float64(relInfo.LatestVersionDate.Unix()),
				product.Name,
				relInfo.ReleaseCycleName,
			)

			ch <- prometheus.MustNewConstMetric(
				EndOfLifeReleaseCycleTimestampSecondsDesc,
				prometheus.GaugeValue,
				float64(relInfo.ReleaseCycleDate.Unix()),
				product.Name,
				relInfo.ReleaseCycleName,
			)

			ch <- prometheus.MustNewConstMetric(
				EndOfLifeEolFromTimestampSecondsDesc,
				prometheus.GaugeValue,
				float64(relInfo.EOLFrom.Unix()),
				product.Name,
				relInfo.ReleaseCycleName,
			)
		}
	}
}
