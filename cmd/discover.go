package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/subhrajitprusty/kubedump/internal/config"
	"github.com/subhrajitprusty/kubedump/internal/runner"
)

var (
	discoverContext          string
	discoverCluster          string
	discoverNamespace        string
	discoverKinds            string
	discoverIgnoreKinds      string
	discoverIgnoreNamespaces string
	includeHelm              bool
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover all resources from clusters and write to the directory structure",
	Long: `Queries each cluster for all resources of the specified kinds and saves them.
Helm release values (values.yaml) are always saved when helm is available.
Resources owned by Helm are skipped by default; use --include-helm to also dump them as individual files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig(baseDir)
		if err != nil {
			return err
		}

		kinds := resolveKinds(cmd, cfg.Kinds, discoverKinds)
		ignoreKinds := mergeIgnoreKinds(cfg.IgnoreKinds, discoverIgnoreKinds)
		ignoreNamespaces := mergeIgnoreNamespaces(cfg.IgnoreNamespaces, discoverIgnoreNamespaces)

		// Use kubedump.yaml clusters when available
		if len(cfg.Clusters) > 0 {
			if discoverContext != "" {
				fmt.Fprintf(os.Stderr, "[warn] --context=%s ignored: kubedump.yaml defines clusters; use --cluster to select a specific cluster by its key in kubedump.yaml\n", discoverContext)
			}
			if discoverCluster != "" {
				context, ok := cfg.Clusters[discoverCluster]
				if !ok {
					return fmt.Errorf("cluster %q not found in kubedump.yaml", discoverCluster)
				}
				return runner.Discover(baseDir, discoverCluster, context, discoverNamespace, kinds, ignoreKinds, ignoreNamespaces, includeHelm, dryRun)
			}
			var errs []error
			for clusterDir, context := range cfg.Clusters {
				if err := runner.Discover(baseDir, clusterDir, context, discoverNamespace, kinds, ignoreKinds, ignoreNamespaces, includeHelm, dryRun); err != nil {
					errs = append(errs, fmt.Errorf("cluster %q: %w", clusterDir, err))
				}
			}
			return errors.Join(errs...)
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

		return runner.Discover(baseDir, clusterDir, discoverContext, discoverNamespace, kinds, ignoreKinds, ignoreNamespaces, includeHelm, dryRun)
	},
}

func init() {
	discoverCmd.Flags().StringVar(&discoverContext, "context", "", "kubectl context to use")
	discoverCmd.Flags().StringVar(&discoverCluster, "cluster", "", "Select a specific cluster key from kubedump.yaml, or override directory name when not using kubedump.yaml (default: last segment of context)")
	discoverCmd.Flags().StringVar(&discoverNamespace, "namespace", "", "Limit to a specific namespace")
	discoverCmd.Flags().StringVar(&discoverKinds, "kinds", "", "Comma-separated resource kinds to fetch (overrides include_kinds in kubedump.yaml and the built-in defaults)")
	discoverCmd.Flags().StringVar(&discoverIgnoreKinds, "ignore-kinds", "", "Comma-separated resource kinds to skip (merged with ignore_kinds from kubedump.yaml)")
	discoverCmd.Flags().StringVar(&discoverIgnoreNamespaces, "ignore-namespaces", "", "Comma-separated namespaces to skip (merged with ignore_namespaces from kubedump.yaml)")
	discoverCmd.Flags().BoolVar(&includeHelm, "include-helm", false, "Include resources managed by Helm (skipped by default)")
	rootCmd.AddCommand(discoverCmd)
}
