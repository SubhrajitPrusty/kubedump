package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Target identifies a single dumped file to refresh, resolved from its location
// within the dump tree.
type Target struct {
	ClusterDir    string // directory name under the base dir, e.g. "api-cluster"
	Context       string // kubectl context for ClusterDir
	Namespace     string // namespace segment of the path
	KindDir       string // <Kind> directory segment, e.g. "Deployment" or "HelmRelease"
	Release       string // Helm release name (IsHelmRelease only)
	Path          string // absolute path of the file to write
	IsHelmRelease bool
}

// ResolveTarget maps a path inside the dump tree to the resource it holds.
//
// Resolution is purely lexical: the path is not required to exist, so a
// HelmRelease values.yaml that was never dumped can still be fetched. Accepted
// shapes, relative to baseDir:
//
//	<cluster>/<namespace>/<Kind>/<name>.yaml           a single resource
//	<cluster>/<namespace>/HelmRelease/<release>        release dir, values.yaml implied
//	<cluster>/<namespace>/HelmRelease/<release>/values.yaml
//
// The kind and name of a regular resource come from the file contents rather
// than the path, so a <Kind> directory whose casing differs from the API kind
// still resolves. contextOverride, when non-empty, wins over the clusters map.
func ResolveTarget(baseDir, argPath string, clusters map[string]string, contextOverride string) (*Target, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(argPath)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot resolve against base dir %s: %w", argPath, baseDir, err)
	}
	sep := string(filepath.Separator)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+sep) {
		return nil, fmt.Errorf("%s is outside base dir %s (pass --base-dir)", argPath, baseDir)
	}

	segs := strings.Split(rel, sep)
	shapeErr := fmt.Errorf("%s: expected <cluster>/<namespace>/<Kind>/<name>.yaml or "+
		"<cluster>/<namespace>/%s/<release>[/%s] relative to base dir %s, got %q",
		argPath, helmReleaseDir, helmValuesFile, baseDir, rel)
	if len(segs) < 4 {
		return nil, shapeErr
	}

	t := &Target{
		ClusterDir: segs[0],
		Namespace:  segs[1],
		KindDir:    segs[2],
		Path:       absPath,
	}
	switch {
	case t.KindDir == helmReleaseDir && len(segs) == 4:
		// Release directory — the values file is implied.
		t.IsHelmRelease = true
		t.Release = segs[3]
		t.Path = filepath.Join(absPath, helmValuesFile)
	case t.KindDir == helmReleaseDir && len(segs) == 5 && segs[4] == helmValuesFile:
		t.IsHelmRelease = true
		t.Release = segs[3]
	case len(segs) == 4 && isYAMLFile(segs[3]):
		// Regular resource file.
	default:
		return nil, shapeErr
	}

	switch {
	case contextOverride != "":
		t.Context = contextOverride
	case clusters[t.ClusterDir] != "":
		t.Context = clusters[t.ClusterDir]
	default:
		return nil, fmt.Errorf("cluster directory %q is not in kubedump.yaml (%s); pass --context to override",
			t.ClusterDir, knownClusters(clusters))
	}
	return t, nil
}

// knownClusters renders the configured cluster directory names for error messages.
func knownClusters(clusters map[string]string) string {
	if len(clusters) == 0 {
		return "no clusters configured"
	}
	names := make([]string, 0, len(clusters))
	for name := range clusters {
		names = append(names, name)
	}
	sort.Strings(names)
	return "known: " + strings.Join(names, ", ")
}

// RefreshTarget re-fetches the single file identified by t.
//
// Unlike the full sweep, every failure is returned rather than logged: a file
// the caller named explicitly and that could not be refreshed is an error. For
// the same reason no filtering is applied — ignoreKinds, ignoreNamespaces and
// Helm ownership are reported as notices, not honoured, since naming a path is
// a stronger signal of intent than a bulk-sweep filter.
func RefreshTarget(t *Target, ignoreKinds, ignoreNamespaces []string, dryRun bool) error {
	if isIgnored(t.KindDir, ignoreKinds) {
		fmt.Printf("  [explicit] kind %s is in ignore_kinds, refreshing anyway\n", t.KindDir)
	}
	if isNamespaceIgnored(t.Namespace, ignoreNamespaces) {
		fmt.Printf("  [explicit] namespace %s is in ignore_namespaces, refreshing anyway\n", t.Namespace)
	}

	if t.IsHelmRelease {
		if !helmAvailable() {
			return fmt.Errorf("helm not found on PATH, cannot refresh %s", t.Path)
		}
		return RefreshHelmRelease(t.Release, t.Namespace, t.Context, t.Path, dryRun)
	}

	meta, err := parseResourceMeta(t.Path)
	if err != nil {
		// Kind and name come from the file, so there is nothing to fetch without it.
		if os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist; run discover to create it", t.Path)
		}
		return fmt.Errorf("parse %s: %w", t.Path, err)
	}
	if meta.Kind == "" || meta.Metadata.Name == "" {
		return fmt.Errorf("%s: could not determine kind/name from file contents", t.Path)
	}
	// Opaque release history rather than declarative config — never useful to re-fetch.
	if meta.Type == helmReleaseSecretType {
		return fmt.Errorf("%s: Secret of type %s is a Helm release blob, refusing to refresh", t.Path, helmReleaseSecretType)
	}

	ns := meta.Metadata.Namespace
	if ns == "" {
		ns = t.Namespace
	} else if ns != t.Namespace {
		fmt.Fprintf(os.Stderr, "  [warn] %s: namespace in file (%s) differs from directory (%s), using %s\n",
			t.Path, ns, t.Namespace, ns)
	}
	if meta.Metadata.Labels[managedByLabel] == "Helm" {
		fmt.Printf("  [explicit] %s/%s is Helm-managed, refreshing anyway\n", meta.Kind, meta.Metadata.Name)
	}

	return FetchAndSave(t.Context, meta.Kind, meta.Metadata.Name, ns, t.Path, dryRun)
}
