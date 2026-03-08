package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// helmAvailable returns true if the helm binary is on PATH.
func helmAvailable() bool {
	_, err := exec.LookPath("helm")
	return err == nil
}

// DumpHelmReleases lists Helm releases in a namespace and saves their values.
// Saves to: <baseDir>/<clusterDir>/<ns>/HelmRelease/<release>/values.yaml
// Silently skips if helm is not installed.
func DumpHelmReleases(baseDir, clusterDir, ns, context string, dryRun bool) error {
	if !helmAvailable() {
		return nil
	}

	out, err := exec.Command(
		"helm", "list",
		"-n", ns,
		"--kube-context", context,
		"-q",
	).Output()
	if err != nil {
		return nil // namespace may have no helm releases
	}

	for _, release := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		release = strings.TrimSpace(release)
		if release == "" {
			continue
		}

		outFile := filepath.Join(baseDir, clusterDir, ns, "HelmRelease", release, "values.yaml")

		if dryRun {
			fmt.Printf("[dry-run] helm get values %s -n %s --kube-context %s > %s\n",
				release, ns, context, outFile)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(outFile), 0o755); err != nil {
			return fmt.Errorf("mkdir for helm release %s: %w", release, err)
		}

		values, err := exec.Command(
			"helm", "get", "values", release,
			"-n", ns,
			"--kube-context", context,
		).Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [error] helm get values %s: %v\n", release, err)
			continue
		}

		if err := os.WriteFile(outFile, values, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outFile, err)
		}
		fmt.Printf("  [helm] %s\n", outFile)
	}
	return nil
}

// RefreshHelmRelease re-fetches values for a single Helm release.
func RefreshHelmRelease(release, ns, context, outFile string, dryRun bool) error {
	if !helmAvailable() {
		return nil
	}

	if dryRun {
		fmt.Printf("[dry-run] helm get values %s -n %s --kube-context %s > %s\n",
			release, ns, context, outFile)
		return nil
	}

	values, err := exec.Command(
		"helm", "get", "values", release,
		"-n", ns,
		"--kube-context", context,
	).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  [error] helm get values %s: %v\n", release, err)
		return nil
	}

	if err := os.WriteFile(outFile, values, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outFile, err)
	}
	fmt.Printf("  [helm] %s\n", outFile)
	return nil
}
