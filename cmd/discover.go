package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/subhrajitprusty/kubedump/internal/config"
	"github.com/subhrajitprusty/kubedump/internal/runner"
)

var (
	discoverContext     string
	discoverCluster     string
	discoverNamespace   string
	discoverKinds       string
	discoverIgnoreKinds string
	includeHelm         bool
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover all resources from clusters and write to the directory structure",
	Long: `Queries each cluster for all resources of the specified kinds and saves them.
Helm release values are always captured when helm is available.
With --skip-helm, resources owned by Helm are omitted (only values.yaml is kept).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfigMap(baseDir)
		if err != nil {
			return err
		}

		ignoreKinds := mergeIgnoreKinds(cfg.IgnoreKinds, discoverIgnoreKinds)

		// Use kubedump.yaml when no explicit context is given
		if len(cfg.Clusters) > 0 && discoverContext == "" {
			for clusterDir, context := range cfg.Clusters {
				if err := runner.Discover(baseDir, clusterDir, context, discoverNamespace, discoverKinds, ignoreKinds, !includeHelm, dryRun); err != nil {
					fmt.Fprintf(os.Stderr, "error on cluster %s: %v\n", clusterDir, err)
				}
			}
			return nil
		}

		// Fall back to explicit --context or current context
		if discoverContext == "" {
			discoverContext, err = runner.CurrentContext()
			if err != nil {
				return err
			}
			fmt.Printf("No --context given, using current context: %s\n", discoverContext)
		}

		clusterDir := discoverCluster
		if clusterDir == "" {
			// Use the last segment of the context as a reasonable directory name
			parts := strings.Split(discoverContext, "/")
			clusterDir = parts[len(parts)-1]
		}

		return runner.Discover(baseDir, clusterDir, discoverContext, discoverNamespace, discoverKinds, ignoreKinds, !includeHelm, dryRun)
	},
}

func init() {
	discoverCmd.Flags().StringVar(&discoverContext, "context", "", "kubectl context to use")
	discoverCmd.Flags().StringVar(&discoverCluster, "cluster", "", "Override cluster directory name (default: last segment of context)")
	discoverCmd.Flags().StringVar(&discoverNamespace, "namespace", "", "Limit to a specific namespace")
	discoverCmd.Flags().StringVar(&discoverKinds, "kinds", runner.DefaultKinds, "Comma-separated resource kinds to fetch")
	discoverCmd.Flags().StringVar(&discoverIgnoreKinds, "ignore-kinds", "", "Comma-separated resource kinds to skip (merged with ignore_kinds from kubedump.yaml)")
	discoverCmd.Flags().BoolVar(&includeHelm, "include-helm", false, "Include resources managed by Helm (skipped by default)")
	rootCmd.AddCommand(discoverCmd)
}
