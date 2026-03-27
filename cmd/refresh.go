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
	refreshNamespace   string
	refreshIncludeHelm bool
	refreshIgnoreKinds string
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Re-fetch every existing YAML file in the directory tree from the live cluster",
	Long: `Walks the existing directory structure, reads each YAML file to determine
kind/name/namespace, then re-fetches and overwrites it from the live cluster.
HelmRelease directories (values.yaml) are always refreshed.
Resources managed by Helm are skipped by default; use --include-helm to also refresh them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig(baseDir)
		if err != nil {
			return err
		}

		ignoreKinds := mergeIgnoreKinds(cfg.IgnoreKinds, refreshIgnoreKinds)
		fmt.Printf("Refreshing %d clusters\n", len(cfg.Clusters))
		fmt.Printf("Ignoring %v kinds\n", ignoreKinds)

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

			if err := runner.Refresh(clusterPath, context, refreshNamespace, ignoreKinds, refreshIncludeHelm, dryRun); err != nil {
				fmt.Fprintf(os.Stderr, "error on cluster %s: %v\n", clusterDir, err)
			}
		}
		return nil
	},
}

func init() {
	refreshCmd.Flags().StringVar(&refreshNamespace, "namespace", "", "Limit to a specific namespace")
	refreshCmd.Flags().BoolVar(&refreshIncludeHelm, "include-helm", false, "Include resources managed by Helm (skipped by default; HelmRelease values.yaml always refreshed)")
	refreshCmd.Flags().StringVar(&refreshIgnoreKinds, "ignore-kinds", "", "Comma-separated resource kinds to skip (merged with ignore_kinds from kubedump.yaml)")
	rootCmd.AddCommand(refreshCmd)
}
