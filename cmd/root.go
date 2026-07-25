package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/subhrajitprusty/kubedump/internal/runner"
)

var (
	baseDir string
	dryRun  bool
)

var rootCmd = &cobra.Command{
	Use:   "kubedump",
	Short: "Snapshot live Kubernetes resources into a clean, Git-friendly directory tree",
	Long: `kubedump fetches resources from live clusters, cleans them with kubectl-neat,
and saves them under:

  <cluster>/<namespace>/<Kind>/<name>.yaml
  <cluster>/<namespace>/HelmRelease/<release>/values.yaml

Cluster contexts are resolved via a kubedump.yaml file in the base directory.`,
	// Runtime failures shouldn't dump the whole usage block; flag errors still do.
	// Errors are reported by Execute, so cobra must not also print them.
	SilenceUsage:  true,
	SilenceErrors: true,
}

// resolveKinds returns the kinds string to use for discover, in priority order:
// 1. --kinds flag (if explicitly passed), 2. include_kinds from kubedump.yaml, 3. built-in defaults.
func resolveKinds(cmd *cobra.Command, fromConfig []string, fromFlag string) string {
	if cmd.Flags().Changed("kinds") {
		return fromFlag
	}
	if len(fromConfig) > 0 {
		return strings.Join(fromConfig, ",")
	}
	return runner.DefaultKinds
}

// mergeIgnoreKinds combines kinds from the config file with any extra kinds
// provided via the --ignore-kinds flag (comma-separated string).
func mergeIgnoreKinds(fromConfig []string, fromFlag string) []string {
	merged := append([]string{}, fromConfig...)
	for _, k := range strings.Split(fromFlag, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			merged = append(merged, k)
		}
	}
	return merged
}

// mergeIgnoreNamespaces combines namespaces from the config file with any extra
// namespaces provided via the --ignore-namespaces flag (comma-separated string).
func mergeIgnoreNamespaces(fromConfig []string, fromFlag string) []string {
	merged := append([]string{}, fromConfig...)
	for _, ns := range strings.Split(fromFlag, ",") {
		ns = strings.TrimSpace(ns)
		if ns != "" {
			merged = append(merged, ns)
		}
	}
	return merged
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&baseDir, "base-dir", ".", "Base directory for output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Print actions without writing files")
}
