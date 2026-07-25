# kubedump

A CLI tool that snapshots live Kubernetes cluster resources into a structured local directory tree. Each resource is cleaned through [`kubectl-neat`](https://github.com/itaysk/kubectl-neat) to strip runtime-only fields (managed fields, last-applied annotations, cluster IPs, resource versions), producing minimal, readable manifests suitable for Git.

## Why

`kubectl get -o yaml` output is cluttered with fields that make diffs noisy and version control impractical. `kubedump` solves this by discovering all resources of interest, cleaning each one through `kubectl-neat`, and saving them in a consistent directory layout — one file per resource.

Helm releases are tracked as `values.yaml` rather than noisy rendered manifests.

## Output layout

```text
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

Create a `kubedump.yaml` file in your working directory mapping local directory names to kubectl context names.

```yaml
clusters:
  api-cluster: arn:aws:eks:ap-south-1:123456789:cluster/api-cluster
  ws-cluster: arn:aws:eks:ap-south-1:123456789:cluster/ws-cluster
include_kinds:
  - Deployment
  - StatefulSet
  - Secret
ignore_kinds:
  - ConfigMap
ignore_namespaces:
  - kube-system      # these four are the built-in defaults; listing them
  - kube-node-lease  # here overrides the defaults entirely, so add any
  - kube-public      # extras you need alongside them
  - kube-flannel
  - my-extra-ns
```

`include_kinds`, `ignore_kinds`, and `ignore_namespaces` are all optional.

`include_kinds` sets the exact list of resource kinds to fetch, replacing the built-in defaults — use it to avoid passing `--kinds` on every run. Precedence is `--kinds` > `include_kinds` > built-in defaults.

When `ignore_namespaces` is absent, `kube-system`, `kube-node-lease`, `kube-public`, and `kube-flannel` are skipped by default. Set `ignore_namespaces: []` to disable the defaults.

### AWS EKS

Populate kubeconfig before running kubedump:

```bash
for cluster in my-api-cluster my-ws-cluster; do
  aws eks update-kubeconfig --region ap-south-1 --name "$cluster"
done
```

The ARN written by `update-kubeconfig` is what goes in the `clusters` map above.

## Usage

### discover

Fetch all resources from every cluster (or a specific one) and write them to the directory tree.

```bash
# All clusters from kubedump.yaml
kubedump discover

# Include resources owned by Helm
kubedump discover --include-helm

# Single namespace / explicit context
kubedump discover --namespace default --context my-ctx --cluster my-cluster

# Custom resource kinds
kubedump discover --kinds Deployment,Service,ConfigMap
```

Default kinds fetched: `Deployment`, `StatefulSet`, `DaemonSet`, `CronJob`, `Service`, `Ingress`, `ConfigMap`, `HorizontalPodAutoscaler`, `ServiceAccount`, `PodDisruptionBudget`, `Secret`. Override them permanently with `include_kinds` in `kubedump.yaml`.

Secrets of type `helm.sh/release.v1` (Helm's internal release history) are always skipped in both `discover` and `refresh`, since they are opaque release blobs rather than declarative config.

### refresh

Re-fetch every YAML file that already exists in the directory tree from the live cluster. Useful for keeping snapshots up to date without a full rediscover.

```bash
kubedump refresh

# Limit to a specific namespace
kubedump refresh --namespace default

# Also refresh resources owned by Helm (HelmRelease values.yaml is always refreshed)
kubedump refresh --include-helm
```

Comments you hand-add to a committed YAML file are preserved across refreshes.
When a file is re-fetched, comments are re-attached to matching keys (matched by
key name, so field reordering is fine); the fresh cluster data always wins for
values. A comment attached to a key that no longer exists in the live resource
is dropped, since it has nowhere to land.

#### Refreshing specific files

Pass one or more paths to refresh only those, instead of walking the whole tree.

```bash
# A single resource
kubedump refresh api-cluster/default/Deployment/api-server.yaml

# Several at once, or a shell glob
kubedump refresh api-cluster/default/Deployment/*.yaml

# A Helm release — the trailing values.yaml is optional
kubedump refresh api-cluster/kube-system/HelmRelease/aws-load-balancer-controller
```

Paths are interpreted relative to `--base-dir` (absolute paths work too), in one of two shapes:

```text
<cluster>/<namespace>/<Kind>/<name>.yaml
<cluster>/<namespace>/HelmRelease/<release>[/values.yaml]
```

Targeted refresh differs from the full sweep in three ways:

- **Filters are reported, not applied.** `ignore_kinds`, `ignore_namespaces` and Helm ownership never skip a file you named explicitly — naming a path is a stronger signal of intent than a bulk filter — so each is printed as an `[explicit]` notice instead. Secrets of type `helm.sh/release.v1` are the one exception and are still refused.
- **Failures exit non-zero.** The sweep logs an error per resource and carries on; a named path that cannot be refreshed fails the command, which makes it safe to use in scripts. Every path is still attempted before the command exits.
- **Kind and name are read from the file**, not the path, so a regular resource must already exist — use `discover` to create it. A `HelmRelease` path is the exception: the release name is in the path itself, so a missing `values.yaml` gets created.

`--context <ctx>` overrides the context for a path whose cluster directory is not in `kubedump.yaml`. `--namespace` is rejected alongside paths, since a path already names its namespace.

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

## GitHub Actions

See [docs/github-actions.md](docs/github-actions.md) for a complete workflow that runs a daily refresh and opens a PR when manifests change.

## Roadmap

Deliberately deferred, roughly in order of usefulness:

- **Directory arguments for `refresh`** — `kubedump refresh <cluster>/<namespace>` to sweep one subtree rather than one file. Needs `runner.Refresh` generalised to start at any depth in the tree instead of always at a cluster root; `--namespace` covers the common case for now.
- **Create files from a path alone** — a regular resource must already exist to be refreshed, since its kind and name are read from the file contents. Inferring the kind from the `<Kind>` directory segment would let `refresh` fetch a resource that was never discovered, at the cost of trusting directory casing to match the API kind.
- **Fuzzy resource matching** — `kubedump refresh api-server`, resolving a bare name across clusters and namespaces. Shell globs cover most of this, and ambiguous matches would need a disambiguation prompt to be worth it.

## Development

```bash
go mod tidy
go build -o kubedump .
```
