package config

import (
	"fmt"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
)

type Product struct {
	Name        string   `yaml:"name"`
	AllReleases bool     `yaml:"all_releases,omitempty"`
	Releases    []string `yaml:"releases"`
}

type Config struct {
	Products []Product `yaml:"products"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	if config.Products == nil {
		return nil, fmt.Errorf("no products defined in the configuration")
	}

	for i, product := range config.Products {
		// Warn if both all_releases and releases are specified
		if product.AllReleases && len(product.Releases) > 0 {
			slog.Warn("Ignoring 'releases' field when 'all_releases' is true", "product", product.Name)
		}

		// Set default to ["latest"] only if all_releases is false and releases is empty
		if !product.AllReleases && product.Releases == nil {
			config.Products[i].Releases = []string{"latest"}
		}
	}

	return config, nil
}
