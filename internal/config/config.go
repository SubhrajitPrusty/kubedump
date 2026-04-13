package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultIgnoreNamespaces are the Kubernetes system namespaces skipped unless overridden.
var DefaultIgnoreNamespaces = []string{
	"kube-system",
	"kube-node-lease",
	"kube-public",
	"kube-flannel",
}

// Config holds all settings loaded from the kubedump.yaml file.
type Config struct {
	// Clusters maps local directory name -> kubectl context name.
	Clusters map[string]string `yaml:"clusters"`
	// IgnoreKinds lists resource kinds to skip during discover/refresh.
	IgnoreKinds []string `yaml:"ignore_kinds"`
	// IgnoreNamespaces lists namespaces to skip during discover/refresh.
	// Defaults to kube-system, kube-node-lease, kube-public, kube-flannel.
	// Set to an empty list in kubedump.yaml to disable the defaults.
	IgnoreNamespaces []string `yaml:"ignore_namespaces"`
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
		return &Config{
			Clusters:         map[string]string{},
			IgnoreNamespaces: append([]string{}, DefaultIgnoreNamespaces...),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Clusters: make(map[string]string),
	}

	// Use a raw map to detect whether ignore_namespaces was explicitly set.
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Clusters == nil {
		cfg.Clusters = make(map[string]string)
	}
	// Apply defaults only when the key is absent from the file entirely.
	if _, ok := raw["ignore_namespaces"]; !ok {
		cfg.IgnoreNamespaces = append([]string{}, DefaultIgnoreNamespaces...)
	}
	return cfg, nil
}
