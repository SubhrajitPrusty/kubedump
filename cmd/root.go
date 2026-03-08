package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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

Cluster contexts are resolved via a .context-map file in the base directory.`,
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
