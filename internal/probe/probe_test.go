package probe

import (
	"testing"

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
