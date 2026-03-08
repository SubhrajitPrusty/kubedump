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

// resourceMeta is a minimal struct for parsing kind/name/namespace from a YAML file.
type resourceMeta struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
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

// Discover fetches all resources from a cluster and writes them to baseDir.
func Discover(baseDir, clusterDir, context, nsFilter, kinds string, skipHelm, dryRun bool) error {
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

		if err := DumpHelmReleases(baseDir, clusterDir, ns, context, dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] helm releases in %s: %v\n", ns, err)
		}

		for _, kind := range kindList {
			kind = strings.TrimSpace(kind)
			if kind == "" {
				continue
			}

			resources, err := GetResources(context, ns, kind)
			if err != nil || len(resources) == 0 {
				continue
			}

			for _, res := range resources {
				if skipHelm && res.ManagedBy == "Helm" {
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
func Refresh(clusterPath, context, nsFilter string, skipHelm, dryRun bool) error {
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
			kindPath := filepath.Join(nsPath, kindDir)

			// HelmRelease dirs: refresh via helm get values
			if kindDir == "HelmRelease" {
				if skipHelm {
					fmt.Printf("  [skip-helm] HelmRelease in %s\n", ns)
					continue
				}
				releaseEntries, _ := os.ReadDir(kindPath)
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
			entries, _ := os.ReadDir(kindPath)
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

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !strings.Contains(string(data), "managed-by: Helm") {
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
