package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Services []Service `yaml:"services"`
	// TODO: add global defaults?
}

type Service struct {
	Name string `yaml:"name"`
	APIs []API  `yaml:"apis"`
}

type API struct {
	Path          string `yaml:"path"`
	Algorithm     string `yaml:"algorithm"` // sliding_window or token_bucket
	KeyStrategy   string `yaml:"key_strategy"` // ip or header:X-Name
	Limit         int    `yaml:"limit"`
	WindowSeconds int    `yaml:"window_seconds"`
	Burst         int    `yaml:"burst"` // only for token_bucket
}

func Load(path string) (*Config, error) {
	fmt.Printf("[config] loading from %s\n", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	fmt.Printf("[config] loaded %d services\n", len(cfg.Services))
	// TODO: validate config values?
	
	return &cfg, nil
}
