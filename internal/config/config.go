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

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	fmt.Printf("[config] loaded %d services\n", len(cfg.Services))
	
	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Services) == 0 {
		return fmt.Errorf("no services defined")
	}

	for _, svc := range c.Services {
		if svc.Name == "" {
			return fmt.Errorf("service name cannot be empty")
		}
		if len(svc.APIs) == 0 {
			return fmt.Errorf("service %s has no APIs defined", svc.Name)
		}

		for _, api := range svc.APIs {
			if api.Path == "" {
				return fmt.Errorf("service %s has API with empty path", svc.Name)
			}
			if api.Algorithm != "sliding_window" && api.Algorithm != "token_bucket" {
				return fmt.Errorf("service %s api %s has invalid algorithm: %s", svc.Name, api.Path, api.Algorithm)
			}
			if api.Limit <= 0 {
				return fmt.Errorf("service %s api %s has invalid limit: %d", svc.Name, api.Path, api.Limit)
			}
			if api.WindowSeconds <= 0 {
				return fmt.Errorf("service %s api %s has invalid window_seconds: %d", svc.Name, api.Path, api.WindowSeconds)
			}
			if api.KeyStrategy == "" {
				return fmt.Errorf("service %s api %s has empty key_strategy", svc.Name, api.Path)
			}
		}
	}

	return nil
}
