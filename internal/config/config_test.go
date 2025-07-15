package config

import (
	"fmt"
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
	})
})
