package runner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultKinds = "Deployment,StatefulSet,DaemonSet,CronJob,Service,Ingress,ConfigMap,HorizontalPodAutoscaler,ServiceAccount,PodDisruptionBudget"

// resourceMeta is a minimal struct for parsing kind/name/namespace/labels from a YAML file.
type resourceMeta struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string            `yaml:"name"`
		Namespace string            `yaml:"namespace"`
		Labels    map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
}

func parseResourceMeta(path string) (*resourceMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m resourceMeta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// isIgnored reports whether kind matches any entry in ignoreKinds (case-insensitive).
func isIgnored(kind string, ignoreKinds []string) bool {
	lower := strings.ToLower(kind)
	for _, ig := range ignoreKinds {
		if strings.ToLower(ig) == lower {
			return true
		}
	}
	return false
}

// Discover fetches all resources from a cluster and writes them to baseDir.
func Discover(baseDir, clusterDir, context, nsFilter, kinds string, ignoreKinds []string, includeHelm, dryRun bool) error {
	fmt.Printf("Cluster: %s  (context: %s)\n", clusterDir, context)

	var namespaces []string
	if nsFilter != "" {
		namespaces = []string{nsFilter}
	} else {
		var err error
		namespaces, err = GetNamespaces(context)
		if err != nil {
			return err
		}
	}

	kindList := strings.Split(kinds, ",")

	for _, ns := range namespaces {
		fmt.Printf("  Namespace: %s\n", ns)

		// Always snapshot Helm release values — this is the preferred artifact for Helm-managed apps.
		if err := DumpHelmReleases(baseDir, clusterDir, ns, context, dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] helm releases in %s: %v\n", ns, err)
		}

		for _, kind := range kindList {
			kind = strings.TrimSpace(kind)
			if kind == "" {
				continue
			}
			if isIgnored(kind, ignoreKinds) {
				continue
			}

			resources, err := GetResources(context, ns, kind)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  [error] list %s in %s: %v\n", kind, ns, err)
				continue
			}
			if len(resources) == 0 {
				continue
			}

			for _, res := range resources {
				if !includeHelm && res.ManagedBy == "Helm" {
					fmt.Printf("  [skip-helm] %s/%s\n", kind, res.Name)
					continue
				}
				outFile := filepath.Join(baseDir, clusterDir, ns, kind, res.Name+".yaml")
				if err := FetchAndSave(context, kind, res.Name, ns, outFile, dryRun); err != nil {
					fmt.Fprintf(os.Stderr, "  [error] %v\n", err)
				}
			}
		}
	}
	return nil
}

// Refresh re-fetches every existing YAML file under clusterPath from the live cluster.
func Refresh(clusterPath, context, nsFilter string, ignoreKinds []string, includeHelm, dryRun bool) error {
	clusterDir := filepath.Base(clusterPath)
	fmt.Printf("Cluster: %s  (context: %s)\n", clusterDir, context)

	nsEntries, err := os.ReadDir(clusterPath)
	if err != nil {
		return err
	}

	for _, nsEntry := range nsEntries {
		if !nsEntry.IsDir() {
			continue
		}
		ns := nsEntry.Name()
		if nsFilter != "" && ns != nsFilter {
			continue
		}
		fmt.Printf("  Namespace: %s\n", ns)
		nsPath := filepath.Join(clusterPath, ns)

		kindEntries, err := os.ReadDir(nsPath)
		if err != nil {
			continue
		}

		for _, kindEntry := range kindEntries {
			if !kindEntry.IsDir() {
				continue
			}
			kindDir := kindEntry.Name()
			if isIgnored(kindDir, ignoreKinds) {
				fmt.Printf("  [ignore] %s in %s\n", kindDir, ns)
				continue
			}
			kindPath := filepath.Join(nsPath, kindDir)

			// HelmRelease dirs: always refresh via helm get values
			if kindDir == "HelmRelease" {
				releaseEntries, err := os.ReadDir(kindPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  [error] read HelmRelease dir %s: %v\n", kindPath, err)
					continue
				}
				for _, re := range releaseEntries {
					if !re.IsDir() {
						continue
					}
					outFile := filepath.Join(kindPath, re.Name(), "values.yaml")
					if err := RefreshHelmRelease(re.Name(), ns, context, outFile, dryRun); err != nil {
						fmt.Fprintf(os.Stderr, "  [error] %v\n", err)
					}
				}
				continue
			}

			// Regular resource files
			entries, err := os.ReadDir(kindPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  [error] read kind dir %s: %v\n", kindPath, err)
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
					continue
				}
				yamlFile := filepath.Join(kindPath, name)
				meta, err := parseResourceMeta(yamlFile)
				if err != nil || meta.Kind == "" || meta.Metadata.Name == "" {
					fmt.Printf("  [skip] %s (could not parse kind/name)\n", yamlFile)
					continue
				}
				if !includeHelm && meta.Metadata.Labels["app.kubernetes.io/managed-by"] == "Helm" {
					fmt.Printf("  [skip-helm] %s/%s\n", meta.Kind, meta.Metadata.Name)
					continue
				}
				if err := FetchAndSave(context, meta.Kind, meta.Metadata.Name, meta.Metadata.Namespace, yamlFile, dryRun); err != nil {
					fmt.Fprintf(os.Stderr, "  [error] %v\n", err)
				}
			}
		}
	}
	return nil
}

// PruneHelm deletes YAML files under baseDir whose content has managed-by=Helm,
// then removes any directories that are left empty.
func PruneHelm(baseDir string, dryRun bool) error {
	fmt.Printf("Scanning for Helm-managed resource files under %s ...\n", baseDir)
	deleted := 0

	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		// Keep HelmRelease values files
		if strings.Contains(path, string(filepath.Separator)+"HelmRelease"+string(filepath.Separator)) {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}

		meta, err := parseResourceMeta(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] parse %s: %v\n", path, err)
			return nil
		}
		if meta.Metadata.Labels["app.kubernetes.io/managed-by"] != "Helm" {
			return nil
		}

		if dryRun {
			fmt.Printf("[dry-run] would delete %s\n", path)
			return nil
		}

		if err := os.Remove(path); err != nil {
			fmt.Fprintf(os.Stderr, "  [error] remove %s: %v\n", path, err)
			return nil
		}
		fmt.Printf("  [pruned] %s\n", path)
		deleted++
		return nil
	})
	if err != nil {
		return err
	}

	if !dryRun {
		removeEmptyDirs(baseDir)
		fmt.Printf("Done. Deleted %d file(s) and cleaned up empty directories.\n", deleted)
	}
	return nil
}

// removeEmptyDirs removes empty directories bottom-up within baseDir.
func removeEmptyDirs(baseDir string) {
	var dirs []string
	filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, _ error) error { //nolint:errcheck
		if d != nil && d.IsDir() && path != baseDir {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err == nil && len(entries) == 0 {
			os.Remove(dirs[i]) //nolint:errcheck
			fmt.Printf("  [rmdir] %s\n", dirs[i])
		}
	}
}
