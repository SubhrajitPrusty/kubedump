package cmd

import (
	"github.com/spf13/cobra"
	"github.com/subhrajitprusty/kubedump/internal/runner"
)

var pruneCmd = &cobra.Command{
	Use:   "prune-helm",
	Short: "Delete dumped YAML files whose content shows managed-by=Helm",
	Long: `Walks all YAML files under the base directory and deletes any that contain
"managed-by: Helm" in their content (i.e. resources owned by a Helm release).
Files inside HelmRelease/ directories are preserved.
Empty directories are removed after pruning.

Run with --dry-run first to preview what would be deleted.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runner.PruneHelm(baseDir, dryRun)
	},
}

func init() {
	rootCmd.AddCommand(pruneCmd)
}
