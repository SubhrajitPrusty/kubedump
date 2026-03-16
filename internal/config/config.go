package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds all settings loaded from the kubedump.yaml file.
type Config struct {
	// Clusters maps local directory name -> kubectl context name.
	Clusters map[string]string `yaml:"clusters"`
	// IgnoreKinds lists resource kinds to skip during discover/refresh.
	IgnoreKinds []string `yaml:"ignore_kinds"`
}

// LoadConfig reads the kubedump.yaml file and returns the parsed Config.
//
// Example kubedump.yaml:
//
//	clusters:
//	  api-cluster: arn:aws:eks:ap-south-1:123456789:cluster/api-cluster
//	  ws-cluster: arn:aws:eks:ap-south-1:123456789:cluster/ws-cluster
//	ignore_kinds:
//	  - ConfigMap
//	  - Secret
func LoadConfig(baseDir string) (*Config, error) {
	path := filepath.Join(baseDir, "kubedump.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Clusters: map[string]string{}}, nil
	}
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Clusters: make(map[string]string),
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Clusters == nil {
		cfg.Clusters = make(map[string]string)
	}
	return cfg, nil
}
