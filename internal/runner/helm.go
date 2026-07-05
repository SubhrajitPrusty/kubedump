package runner

import (
	"bytes"
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

	cmd := exec.Command(
		"helm", "list",
		"-n", ns,
		"--kube-context", context,
		"-q",
	)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "  [warn] helm list -n %s: %s\n", ns, strings.TrimSpace(string(ee.Stderr)))
			return nil
		}
		return fmt.Errorf("helm list -n %s: %w", ns, err)
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

		helmCmd := exec.Command(
			"helm", "get", "values", release,
			"-n", ns,
			"--kube-context", context,
			"-o", "yaml",
		)
		var helmStderr bytes.Buffer
		helmCmd.Stderr = &helmStderr
		values, err := helmCmd.Output()
		if err != nil {
			msg := strings.TrimSpace(helmStderr.String())
			errMsg := err.Error()
			if msg == "" {
				msg = errMsg
			} else if msg != errMsg {
				msg = fmt.Sprintf("%s (%s)", msg, errMsg)
			}
			fmt.Fprintf(os.Stderr, "  [error] helm get values %s: %s\n", release, msg)
			continue
		}

		if merged, err := mergeCommentsFromFile(outFile, values); err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] preserve comments %s: %v\n", outFile, err)
		} else {
			values = merged
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

	helmCmd := exec.Command(
		"helm", "get", "values", release,
		"-n", ns,
		"--kube-context", context,
		"-o", "yaml",
	)
	var stderr bytes.Buffer
	helmCmd.Stderr = &stderr
	values, err := helmCmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		errMsg := err.Error()
		if msg == "" {
			msg = errMsg
		} else if msg != errMsg {
			msg = fmt.Sprintf("%s (%s)", msg, errMsg)
		}
		fmt.Fprintf(os.Stderr, "  [error] helm get values %s: %s\n", release, msg)
		return nil
	}

	if merged, err := mergeCommentsFromFile(outFile, values); err != nil {
		fmt.Fprintf(os.Stderr, "  [warn] preserve comments %s: %v\n", outFile, err)
	} else {
		values = merged
	}

	if err := os.WriteFile(outFile, values, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outFile, err)
	}
	fmt.Printf("  [helm] %s\n", outFile)
	return nil
}
