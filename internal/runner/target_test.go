package runner

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveTarget(t *testing.T) {
	base := filepath.Join("/dump", "root")
	clusters := map[string]string{
		"api-cluster": "arn:aws:eks:ap-south-1:1:cluster/api-cluster",
		"ws-cluster":  "arn:aws:eks:ap-south-1:1:cluster/ws-cluster",
	}

	tests := []struct {
		name            string
		path            string
		contextOverride string
		want            Target
	}{
		{
			name: "regular resource",
			path: filepath.Join(base, "api-cluster", "default", "Deployment", "api-server.yaml"),
			want: Target{
				ClusterDir: "api-cluster",
				Context:    clusters["api-cluster"],
				Namespace:  "default",
				KindDir:    "Deployment",
				Path:       filepath.Join(base, "api-cluster", "default", "Deployment", "api-server.yaml"),
			},
		},
		{
			name: "yml extension",
			path: filepath.Join(base, "ws-cluster", "prod", "Service", "ws.yml"),
			want: Target{
				ClusterDir: "ws-cluster",
				Context:    clusters["ws-cluster"],
				Namespace:  "prod",
				KindDir:    "Service",
				Path:       filepath.Join(base, "ws-cluster", "prod", "Service", "ws.yml"),
			},
		},
		{
			name: "helm values file",
			path: filepath.Join(base, "api-cluster", "monitoring", "HelmRelease", "loki", "values.yaml"),
			want: Target{
				ClusterDir:    "api-cluster",
				Context:       clusters["api-cluster"],
				Namespace:     "monitoring",
				KindDir:       "HelmRelease",
				Release:       "loki",
				IsHelmRelease: true,
				Path:          filepath.Join(base, "api-cluster", "monitoring", "HelmRelease", "loki", "values.yaml"),
			},
		},
		{
			name: "helm release dir implies values file",
			path: filepath.Join(base, "api-cluster", "monitoring", "HelmRelease", "loki"),
			want: Target{
				ClusterDir:    "api-cluster",
				Context:       clusters["api-cluster"],
				Namespace:     "monitoring",
				KindDir:       "HelmRelease",
				Release:       "loki",
				IsHelmRelease: true,
				Path:          filepath.Join(base, "api-cluster", "monitoring", "HelmRelease", "loki", "values.yaml"),
			},
		},
		{
			name: "trailing separator is cleaned",
			path: filepath.Join(base, "api-cluster", "monitoring", "HelmRelease", "loki") + "/",
			want: Target{
				ClusterDir:    "api-cluster",
				Context:       clusters["api-cluster"],
				Namespace:     "monitoring",
				KindDir:       "HelmRelease",
				Release:       "loki",
				IsHelmRelease: true,
				Path:          filepath.Join(base, "api-cluster", "monitoring", "HelmRelease", "loki", "values.yaml"),
			},
		},
		{
			name:            "context override wins over unmapped cluster",
			path:            filepath.Join(base, "other-cluster", "default", "Deployment", "app.yaml"),
			contextOverride: "my-ctx",
			want: Target{
				ClusterDir: "other-cluster",
				Context:    "my-ctx",
				Namespace:  "default",
				KindDir:    "Deployment",
				Path:       filepath.Join(base, "other-cluster", "default", "Deployment", "app.yaml"),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveTarget(base, tc.path, clusters, tc.contextOverride)
			if err != nil {
				t.Fatalf("ResolveTarget(%q) returned error: %v", tc.path, err)
			}
			if *got != tc.want {
				t.Errorf("ResolveTarget(%q)\n got %+v\nwant %+v", tc.path, *got, tc.want)
			}
		})
	}
}

func TestResolveTargetErrors(t *testing.T) {
	base := filepath.Join("/dump", "root")
	clusters := map[string]string{"api-cluster": "api-ctx"}

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name:    "outside base dir",
			path:    filepath.Join("/elsewhere", "api-cluster", "default", "Deployment", "app.yaml"),
			wantErr: "outside base dir",
		},
		{
			name:    "base dir itself",
			path:    base,
			wantErr: "outside base dir",
		},
		{
			name:    "too shallow",
			path:    filepath.Join(base, "api-cluster", "default", "Deployment"),
			wantErr: "expected",
		},
		{
			name:    "too deep",
			path:    filepath.Join(base, "api-cluster", "default", "Deployment", "nested", "app.yaml"),
			wantErr: "expected",
		},
		{
			name:    "not a yaml file",
			path:    filepath.Join(base, "api-cluster", "default", "Deployment", "README.md"),
			wantErr: "expected",
		},
		{
			name:    "helm path with wrong file name",
			path:    filepath.Join(base, "api-cluster", "default", "HelmRelease", "loki", "manifest.yaml"),
			wantErr: "expected",
		},
		{
			name:    "unmapped cluster without override",
			path:    filepath.Join(base, "ws-cluster", "default", "Deployment", "app.yaml"),
			wantErr: `"ws-cluster" is not in kubedump.yaml`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveTarget(base, tc.path, clusters, "")
			if err == nil {
				t.Fatalf("ResolveTarget(%q) = %+v, want error", tc.path, *got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("ResolveTarget(%q) error = %q, want it to contain %q", tc.path, err, tc.wantErr)
			}
		})
	}
}

// A relative base dir and a relative path both resolve against the working
// directory, which is how the CLI is normally invoked.
func TestResolveTargetRelativePaths(t *testing.T) {
	got, err := ResolveTarget(".", filepath.Join("api-cluster", "default", "Deployment", "app.yaml"),
		map[string]string{"api-cluster": "api-ctx"}, "")
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if got.ClusterDir != "api-cluster" || got.Namespace != "default" || got.KindDir != "Deployment" {
		t.Errorf("unexpected target: %+v", *got)
	}
	if !filepath.IsAbs(got.Path) {
		t.Errorf("Path = %q, want absolute", got.Path)
	}
}

func TestKnownClusters(t *testing.T) {
	if got := knownClusters(nil); got != "no clusters configured" {
		t.Errorf("knownClusters(nil) = %q", got)
	}
	// Sorted, so the message is stable across runs.
	if got := knownClusters(map[string]string{"ws": "b", "api": "a"}); got != "known: api, ws" {
		t.Errorf("knownClusters() = %q", got)
	}
}
