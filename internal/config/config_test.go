package config

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("Config Suite", func() {
	Context("When loading config", func() {
		var tempDir string

		BeforeEach(func() {
			By("Create a temp directory for config")
			tempDir = GinkgoT().TempDir()
		})

		It("should load config without any errors", func() {
			configContent := `---
products:
  - name: mongo
    releases:
      - "8.0"
      - "7.0"`
			filePath := filepath.Join(tempDir, "test_valid_config.yaml")
			Expect(os.WriteFile(filePath, []byte(configContent), 0644)).To(Succeed())

			cfg, err := LoadConfig(filePath)

			Expect(err).To(BeNil())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.Products).To(HaveLen(1))
			Expect(cfg.Products[0].Name).To(Equal("mongo"))
			Expect(cfg.Products[0].AllReleases).To(BeFalse())
			Expect(cfg.Products[0].Releases).To(HaveLen(2))
		})

		It("should fail when the 'products' list is empty", func() {
			configContent := `---
products:`

			filepath := filepath.Join(tempDir, "invalid_config.yaml")
			Expect(os.WriteFile(filepath, []byte(configContent), 0644)).To(Succeed())

			cfg, err := LoadConfig(filepath)
			Expect(err).NotTo(BeNil())
			Expect(cfg).To(BeNil())
		})

		It("should add 'latest' to products release cycle when it is empty", func() {
			configContent := `---
products:
  - name: mongo
    releases:
      - "8.0"
      - "7.0"
  - name: redis`

			filepath := filepath.Join(tempDir, "no_release_cycle_config.yaml")
			Expect(os.WriteFile(filepath, []byte(configContent), 0644)).To(Succeed())

			cfg, err := LoadConfig(filepath)

			Expect(err).To(BeNil())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.Products[0].Name).To(Equal("mongo"))
			Expect(cfg.Products[1].Name).To(Equal("redis"))

			Expect(cfg.Products[0].Releases).To(HaveLen(2))
			Expect(cfg.Products[1].Releases).To(HaveLen(1))

			Expect(cfg.Products[1].Releases[0]).To(Equal("latest"))
		})

		It("should load config with all_releases set to true", func() {
			configContent := `---
products:
  - name: ubuntu
    all_releases: true
  - name: mongo
    releases:
      - "8.0"`

			filepath := filepath.Join(tempDir, "all_releases_config.yaml")
			Expect(os.WriteFile(filepath, []byte(configContent), 0644)).To(Succeed())

			cfg, err := LoadConfig(filepath)

			Expect(err).To(BeNil())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.Products).To(HaveLen(2))

			Expect(cfg.Products[0].Name).To(Equal("ubuntu"))
			Expect(cfg.Products[0].AllReleases).To(BeTrue())
			Expect(cfg.Products[0].Releases).To(BeNil())

			Expect(cfg.Products[1].Name).To(Equal("mongo"))
			Expect(cfg.Products[1].AllReleases).To(BeFalse())
			Expect(cfg.Products[1].Releases).To(HaveLen(1))
		})

		It("should not add 'latest' when all_releases is true and releases is empty", func() {
			configContent := `---
products:
  - name: ubuntu
    all_releases: true`

			filepath := filepath.Join(tempDir, "all_releases_no_releases.yaml")
			Expect(os.WriteFile(filepath, []byte(configContent), 0644)).To(Succeed())

			cfg, err := LoadConfig(filepath)

			Expect(err).To(BeNil())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.Products).To(HaveLen(1))
			Expect(cfg.Products[0].Name).To(Equal("ubuntu"))
			Expect(cfg.Products[0].AllReleases).To(BeTrue())
			Expect(cfg.Products[0].Releases).To(BeNil())
		})

		It("should ignore 'releases' when all_releases is true", func() {
			configContent := `---
products:
  - name: ubuntu
    all_releases: true
    releases:
      - "22.04"
      - "20.04"`

			filepath := filepath.Join(tempDir, "all_releases_with_releases.yaml")
			Expect(os.WriteFile(filepath, []byte(configContent), 0644)).To(Succeed())

			cfg, err := LoadConfig(filepath)

			Expect(err).To(BeNil())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.Products).To(HaveLen(1))
			Expect(cfg.Products[0].Name).To(Equal("ubuntu"))
			Expect(cfg.Products[0].AllReleases).To(BeTrue())
			// Releases will still be present in the struct, but should be ignored by the collector
			Expect(cfg.Products[0].Releases).To(HaveLen(2))
		})
	})
})
