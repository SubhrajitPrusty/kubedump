package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CurrentContext returns the active kubectl context.
func CurrentContext() (string, error) {
	out, err := exec.Command("kubectl", "config", "current-context").Output()
	if err != nil {
		return "", fmt.Errorf("kubectl config current-context: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GetNamespaces returns all namespace names reachable by the given context.
func GetNamespaces(context string) ([]string, error) {
	out, err := exec.Command(
		"kubectl", "get", "namespaces",
		"--context="+context,
		"-o", "jsonpath={.items[*].metadata.name}",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("get namespaces (context=%s): %w", context, err)
	}
	return strings.Fields(string(out)), nil
}

// ResourceInfo holds the name and Helm managed-by label for a resource.
type ResourceInfo struct {
	Name      string
	ManagedBy string
}

// GetResources returns all resources of the given kind in a namespace.
// Returns an empty slice (no error) when the resource type does not exist in the cluster.
func GetResources(context, ns, kind string) ([]ResourceInfo, error) {
	const jsonpath = `{range .items[*]}{.metadata.name}{"\t"}{.metadata.labels.app\.kubernetes\.io/managed-by}{"\n"}{end}`
	cmd := exec.Command(
		"kubectl", "get", kind,
		"-n", ns,
		"--context="+context,
		"-o", "jsonpath="+jsonpath,
	)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(ee.Stderr))
			// These messages indicate the CRD/API group simply isn't registered — not a real error.
			if strings.Contains(stderr, "no matches for kind") ||
				strings.Contains(stderr, "the server doesn't have a resource type") {
				return nil, nil
			}
			fmt.Fprintf(os.Stderr, "  [warn] kubectl get %s -n %s: %s\n", kind, ns, stderr)
			return nil, nil
		}
		return nil, fmt.Errorf("kubectl get %s -n %s: %w", kind, ns, err)
	}

	var resources []ResourceInfo
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		ri := ResourceInfo{Name: parts[0]}
		if len(parts) == 2 {
			ri.ManagedBy = strings.TrimSpace(parts[1])
		}
		resources = append(resources, ri)
	}
	return resources, nil
}

// FetchAndSave fetches a single resource and writes it cleaned via kubectl-neat.
// Non-fatal errors (resource not found, neat failure) are printed and skipped.
func FetchAndSave(context, kind, name, ns, outFile string, dryRun bool) error {
	args := []string{"get", kind, name, "--context=" + context, "-o", "yaml"}
	if ns != "" {
		args = append(args, "-n", ns)
	}

	if dryRun {
		fmt.Printf("[dry-run] kubectl %s | kubectl neat > %s\n", strings.Join(args, " "), outFile)
		return nil
	}

	cmd := exec.Command("kubectl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		errMsg := err.Error()
		if msg == "" {
			msg = errMsg
		} else if msg != errMsg {
			msg = fmt.Sprintf("%s (%s)", msg, errMsg)
		}
		fmt.Fprintf(os.Stderr, "  [error] kubectl get %s/%s: %s\n", kind, name, msg)
		return nil
	}

	neatCmd := exec.Command("kubectl", "neat")
	neatCmd.Stdin = bytes.NewReader(raw)
	cleaned, err := neatCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  [error] kubectl neat %s/%s: %v\n", kind, name, err)
		return nil
	}

	if len(bytes.TrimSpace(cleaned)) == 0 {
		fmt.Printf("  [warn] empty output for %s/%s, skipping\n", kind, name)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(outFile), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(outFile), err)
	}
	if err := os.WriteFile(outFile, cleaned, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outFile, err)
	}

	fmt.Printf("  [ok] %s\n", outFile)
	return nil
}
