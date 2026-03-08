package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/subhrajitprusty/kubedump/internal/config"
	"github.com/subhrajitprusty/kubedump/internal/runner"
)

var refreshNamespace string

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Re-fetch every existing YAML file in the directory tree from the live cluster",
	Long: `Walks the existing directory structure, reads each YAML file to determine
kind/name/namespace, then re-fetches and overwrites it from the live cluster.
HelmRelease directories are refreshed via helm get values.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctxMap, err := config.LoadContextMap(baseDir)
		if err != nil {
			return err
		}

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

			context, ok := ctxMap[clusterDir]
			if !ok {
				context = clusterDir // fall back to dir name as context
			}

			if err := runner.Refresh(clusterPath, context, refreshNamespace, dryRun); err != nil {
				fmt.Fprintf(os.Stderr, "error on cluster %s: %v\n", clusterDir, err)
			}
		}
		return nil
	},
}

func init() {
	refreshCmd.Flags().StringVar(&refreshNamespace, "namespace", "", "Limit to a specific namespace")
	rootCmd.AddCommand(refreshCmd)
}
