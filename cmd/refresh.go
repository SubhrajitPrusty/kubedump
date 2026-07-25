package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/subhrajitprusty/kubedump/internal/config"
	"github.com/subhrajitprusty/kubedump/internal/runner"
)

var (
	refreshNamespace        string
	refreshContext          string
	refreshIncludeHelm      bool
	refreshIgnoreKinds      string
	refreshIgnoreNamespaces string
)

var refreshCmd = &cobra.Command{
	Use:   "refresh [path...]",
	Short: "Re-fetch existing YAML files in the directory tree from the live cluster",
	Long: `Walks the existing directory structure, reads each YAML file to determine
kind/name/namespace, then re-fetches and overwrites it from the live cluster.
HelmRelease directories (values.yaml) are always refreshed.
Resources managed by Helm are skipped by default; use --include-helm to also refresh them.

Given one or more paths, only those files are refreshed. A named path is always
fetched — ignore_kinds, ignore_namespaces and Helm ownership are reported but not
applied — and a failure to refresh it exits non-zero. Accepted path shapes:

  <cluster>/<namespace>/<Kind>/<name>.yaml
  <cluster>/<namespace>/HelmRelease/<release>[/values.yaml]`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig(baseDir)
		if err != nil {
			return err
		}

		ignoreKinds := mergeIgnoreKinds(cfg.IgnoreKinds, refreshIgnoreKinds)
		ignoreNamespaces := mergeIgnoreNamespaces(cfg.IgnoreNamespaces, refreshIgnoreNamespaces)

		if len(args) > 0 {
			if cmd.Flags().Changed("namespace") {
				return fmt.Errorf("--namespace cannot be combined with explicit paths, which already name a namespace")
			}
			return refreshPaths(args, cfg.Clusters, ignoreKinds, ignoreNamespaces)
		}
		if cmd.Flags().Changed("context") {
			return fmt.Errorf("--context requires explicit paths; a full refresh resolves contexts from kubedump.yaml")
		}

		fmt.Printf("Refreshing %d clusters\n", len(cfg.Clusters))
		fmt.Printf("Ignoring %v kinds\n", ignoreKinds)
		fmt.Printf("Ignoring %v namespaces\n", ignoreNamespaces)

		entries, err := os.ReadDir(baseDir)
		if err != nil {
			return err
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			clusterDir := e.Name()
			clusterPath := filepath.Join(baseDir, clusterDir)

			context, ok := cfg.Clusters[clusterDir]
			if !ok {
				continue // skip directories not in context map
			}

			if err := runner.Refresh(clusterPath, context, refreshNamespace, ignoreKinds, ignoreNamespaces, refreshIncludeHelm, dryRun); err != nil {
				fmt.Fprintf(os.Stderr, "error on cluster %s: %v\n", clusterDir, err)
			}
		}
		return nil
	},
}

// refreshPaths refreshes only the paths named on the command line. Every path is
// attempted; the command fails if any of them did.
func refreshPaths(paths []string, clusters map[string]string, ignoreKinds, ignoreNamespaces []string) error {
	fmt.Printf("Refreshing %d path(s)\n", len(paths))

	failed := 0
	for _, p := range paths {
		target, err := runner.ResolveTarget(baseDir, p, clusters, refreshContext)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [error] %v\n", err)
			failed++
			continue
		}
		if err := runner.RefreshTarget(target, ignoreKinds, ignoreNamespaces, dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "  [error] %v\n", err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d path(s) failed to refresh", failed, len(paths))
	}
	return nil
}

func init() {
	refreshCmd.Flags().StringVar(&refreshNamespace, "namespace", "", "Limit to a specific namespace")
	refreshCmd.Flags().StringVar(&refreshContext, "context", "", "Override the kubectl context (requires explicit paths)")
	refreshCmd.Flags().BoolVar(&refreshIncludeHelm, "include-helm", false, "Include resources managed by Helm (skipped by default; HelmRelease values.yaml always refreshed)")
	refreshCmd.Flags().StringVar(&refreshIgnoreKinds, "ignore-kinds", "", "Comma-separated resource kinds to skip (merged with ignore_kinds from kubedump.yaml)")
	refreshCmd.Flags().StringVar(&refreshIgnoreNamespaces, "ignore-namespaces", "", "Comma-separated namespaces to skip (merged with ignore_namespaces from kubedump.yaml)")
	rootCmd.AddCommand(refreshCmd)
}
