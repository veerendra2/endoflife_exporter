package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Product struct {
	Name     string   `yaml:"name"`
	Releases []string `yaml:"releases"`
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
		return nil, fmt.Errorf("no products found in the configuration")
	}

	for i, product := range config.Products {
		if product.Releases == nil {
			config.Products[i].Releases = []string{"latest"}
		}
	}

	return config, nil
}
