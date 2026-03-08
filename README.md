# kubedump

A CLI tool that snapshots live Kubernetes cluster resources into a structured local directory tree. Each resource is cleaned through [`kubectl-neat`](https://github.com/itaysk/kubectl-neat) to strip runtime-only fields (managed fields, last-applied annotations, cluster IPs, resource versions), producing minimal, readable manifests suitable for Git.

## Why

`kubectl get -o yaml` output is cluttered with fields that make diffs noisy and version control impractical. `kubedump` solves this by discovering all resources of interest, cleaning each one through `kubectl-neat`, and saving them in a consistent directory layout — one file per resource.

Helm releases are tracked as `values.yaml` rather than noisy rendered manifests.

## Output layout

```
<cluster>/
  <namespace>/
    <Kind>/
      <resource-name>.yaml       # kubectl-neat cleaned manifest
    HelmRelease/
      <release-name>/
        values.yaml              # helm get values output
```

## Prerequisites

- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) on PATH
- [`kubectl-neat`](https://github.com/itaysk/kubectl-neat) on PATH
- [`helm`](https://helm.sh/docs/intro/install/) on PATH (optional — Helm tracking is skipped gracefully if absent)

## Installation

```bash
git clone https://github.com/SubhrajitPrusty/kubedump.git
cd kubedump
go build -o kubedump .
# Move the binary somewhere on your PATH, e.g.:
mv kubedump /usr/local/bin/
```

## Configuration

Create a `.context-map` file in your working directory mapping local directory names to kubectl context names:

```
prod-in-cluster=arn:aws:eks:ap-south-1:123456789:cluster/prod-in-cluster
dev-in-cluster=arn:aws:eks:ap-south-1:123456789:cluster/dev-in-cluster
```

When a `.context-map` is present, all commands operate across every listed cluster automatically. Without it, commands fall back to the current kubectl context.

## Usage

### discover

Fetch all resources from every cluster (or a specific one) and write them to the directory tree.

```bash
# All clusters from .context-map
kubedump discover

# Skip resources owned by Helm (values.yaml is still captured)
kubedump discover --skip-helm

# Single namespace / explicit context
kubedump discover --namespace default --context my-ctx --cluster my-cluster

# Custom resource kinds
kubedump discover --kinds Deployment,Service,ConfigMap
```

Default kinds fetched: `Deployment`, `StatefulSet`, `DaemonSet`, `CronJob`, `Service`, `Ingress`, `ConfigMap`, `HorizontalPodAutoscaler`, `ServiceAccount`, `PodDisruptionBudget`.

### refresh

Re-fetch every YAML file that already exists in the directory tree from the live cluster. Useful for keeping snapshots up to date without a full rediscover.

```bash
kubedump refresh

# Limit to a specific namespace
kubedump refresh --namespace default
```

### prune-helm

Delete dumped YAML files whose content shows `managed-by: Helm`. Files inside `HelmRelease/` directories are preserved. Empty directories are cleaned up afterwards.

```bash
# Preview what would be deleted
kubedump prune-helm --dry-run

# Actually delete
kubedump prune-helm
```

## Global flags

| Flag | Description |
|------|-------------|
| `--base-dir <dir>` | Base directory for the dump (default: current directory) |
| `--dry-run` | Print actions without writing or deleting anything |

## Development

```bash
go mod tidy
go build -o kubedump .
```
