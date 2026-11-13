package probe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/veerendra2/endoflife_exporter/pkg/endoflife"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProbe(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Probe Suite")
}

var _ = Describe("parseReleases", func() {
	DescribeTable("parses the releases query parameter",
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
})

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

func timePtr(t time.Time) *time.Time { return &t }

func details(cycle string) endoflife.ReleaseDetails {
	return endoflife.ReleaseDetails{
		EOLFrom:           timePtr(time.Unix(2524608000, 0)),
		IsMaintained:      true,
		LatestVersion:     cycle + ".1",
		LatestVersionDate: time.Unix(1700000000, 0),
		ReleaseCycleDate:  time.Unix(1600000000, 0),
		ReleaseCycleName:  cycle,
	}
}

const probeSuccessHeader = `# HELP endoflife_probe_success Whether the probe to endoflife.date succeeded.
# TYPE endoflife_probe_success gauge
`

const productInfoHeader = `# HELP endoflife_product_info Product release cycle information with EOL status, LTS flag, and maintenance state.
# TYPE endoflife_product_info gauge
`

func probeSuccess(product string, value int) string {
	return probeSuccessHeader + `endoflife_probe_success{product_name="` + product + `"} ` + strconv.Itoa(value) + "\n"
}

func productInfo(cycle string) string {
	return `endoflife_product_info{is_eol="false",is_lts="false",is_maintained="true",latest_version="` +
		cycle + `.1",product_name="mongo",release_cycle_name="` + cycle + `"} 1` + "\n"
}

var _ = Describe("probe", func() {
	It("scrapes the latest cycle by default", func() {
		client := fakeClient{getRelease: func(_ context.Context, productName, cycleName string) (endoflife.ReleaseDetails, error) {
			Expect(productName).To(Equal("mongo"))
			Expect(cycleName).To(Equal("latest"))
			return details("latest"), nil
		}}

		reg := probe(context.Background(), client, "mongo", []string{"latest"}, false)

		expected := probeSuccess("mongo", 1) + productInfoHeader + productInfo("latest")
		Expect(testutil.GatherAndCompare(reg, strings.NewReader(expected),
			"endoflife_probe_success", "endoflife_product_info")).To(Succeed())
	})

	It("scrapes every requested cycle", func() {
		client := fakeClient{getRelease: func(_ context.Context, _, cycleName string) (endoflife.ReleaseDetails, error) {
			return details(cycleName), nil
		}}

		reg := probe(context.Background(), client, "mongo", []string{"8.0", "7.0"}, false)

		expected := probeSuccess("mongo", 1) + productInfoHeader + productInfo("8.0") + productInfo("7.0")
		Expect(testutil.GatherAndCompare(reg, strings.NewReader(expected),
			"endoflife_probe_success", "endoflife_product_info")).To(Succeed())
	})

	It("dedupes cycles that resolve to the same release cycle", func() {
		client := fakeClient{getRelease: func(_ context.Context, _, cycleName string) (endoflife.ReleaseDetails, error) {
			if cycleName == "latest" {
				return details("8.0"), nil
			}
			return details(cycleName), nil
		}}

		reg := probe(context.Background(), client, "mongo", []string{"latest", "8.0"}, false)

		expected := probeSuccess("mongo", 1) + productInfoHeader + productInfo("8.0")
		Expect(testutil.GatherAndCompare(reg, strings.NewReader(expected),
			"endoflife_probe_success", "endoflife_product_info")).To(Succeed())
	})

	It("omits the end-of-life date when the API reports none", func() {
		client := fakeClient{getRelease: func(_ context.Context, _, cycleName string) (endoflife.ReleaseDetails, error) {
			d := details(cycleName)
			d.EOLFrom = nil
			return d, nil
		}}

		reg := probe(context.Background(), client, "mongo", []string{"8.0"}, false)

		Expect(testutil.CollectAndCount(reg, "endoflife_eol_from_timestamp_seconds")).To(Equal(0))
		Expect(testutil.CollectAndCount(reg, "endoflife_product_info")).To(Equal(1))
	})

	It("scrapes all cycles when all is requested", func() {
		client := fakeClient{
			getProductDetails: func(_ context.Context, productName string) ([]endoflife.ReleaseDetails, error) {
				Expect(productName).To(Equal("mongo"))
				return []endoflife.ReleaseDetails{details("22.04"), details("24.04")}, nil
			},
			getRelease: func(context.Context, string, string) (endoflife.ReleaseDetails, error) {
				Fail("GetRelease must not be called when all cycles are requested")
				return endoflife.ReleaseDetails{}, nil
			},
		}

		reg := probe(context.Background(), client, "mongo", nil, true)

		expected := probeSuccess("mongo", 1) + productInfoHeader + productInfo("22.04") + productInfo("24.04")
		Expect(testutil.GatherAndCompare(reg, strings.NewReader(expected),
			"endoflife_probe_success", "endoflife_product_info")).To(Succeed())
	})

	It("reports failure when the API fails", func() {
		client := fakeClient{getRelease: func(context.Context, string, string) (endoflife.ReleaseDetails, error) {
			return endoflife.ReleaseDetails{}, errors.New("boom")
		}}

		reg := probe(context.Background(), client, "mongo", []string{"latest"}, false)

		Expect(testutil.GatherAndCompare(reg, strings.NewReader(probeSuccess("mongo", 0)),
			"endoflife_probe_success")).To(Succeed())
		Expect(testutil.CollectAndCount(reg, "endoflife_product_info")).To(Equal(0))
	})

	It("reports failure but keeps the cycles that succeeded", func() {
		client := fakeClient{getRelease: func(_ context.Context, _, cycleName string) (endoflife.ReleaseDetails, error) {
			if cycleName == "7.0" {
				return endoflife.ReleaseDetails{}, errors.New("boom")
			}
			return details(cycleName), nil
		}}

		reg := probe(context.Background(), client, "mongo", []string{"8.0", "7.0"}, false)

		expected := probeSuccess("mongo", 0) + productInfoHeader + productInfo("8.0")
		Expect(testutil.GatherAndCompare(reg, strings.NewReader(expected),
			"endoflife_probe_success", "endoflife_product_info")).To(Succeed())
	})

	It("reports success without data when the product has no cycles", func() {
		client := fakeClient{getProductDetails: func(context.Context, string) ([]endoflife.ReleaseDetails, error) {
			return nil, nil
		}}

		reg := probe(context.Background(), client, "mongo", nil, true)

		Expect(testutil.GatherAndCompare(reg, strings.NewReader(probeSuccess("mongo", 1)),
			"endoflife_probe_success")).To(Succeed())
		Expect(testutil.CollectAndCount(reg, "endoflife_product_info")).To(Equal(0))
	})
})

var _ = Describe("Handler", func() {
	It("rejects a missing target", func() {
		rec := httptest.NewRecorder()
		Handler(fakeClient{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe", nil))

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("rejects a target that is not a product slug", func() {
		client := fakeClient{
			getProductDetails: func(context.Context, string) ([]endoflife.ReleaseDetails, error) {
				Fail("client must not be called for an invalid target")
				return nil, nil
			},
			getRelease: func(context.Context, string, string) (endoflife.ReleaseDetails, error) {
				Fail("client must not be called for an invalid target")
				return endoflife.ReleaseDetails{}, nil
			},
		}

		rec := httptest.NewRecorder()
		Handler(client).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe?target=../x", nil))

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("rejects invalid releases", func() {
		rec := httptest.NewRecorder()
		Handler(fakeClient{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe?target=redis&releases=all,8.0", nil))

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("serves the probe metrics", func() {
		client := fakeClient{getRelease: func(_ context.Context, _, cycleName string) (endoflife.ReleaseDetails, error) {
			return details(cycleName), nil
		}}

		rec := httptest.NewRecorder()
		Handler(client).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe?target=redis", nil))

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(ContainSubstring(`endoflife_probe_success{product_name="redis"} 1`))
	})
})
