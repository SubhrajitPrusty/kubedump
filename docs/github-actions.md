# GitHub Actions — daily manifest refresh

Run `kubedump refresh` on a schedule and open a PR whenever manifests change.

```yaml
name: kubedump refresh

on:
  schedule:
    - cron: "0 2 * * *"
  workflow_dispatch:

permissions:
  id-token: write
  contents: write
  pull-requests: write

jobs:
  refresh:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Configure AWS credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{ secrets.AWS_DEPLOY_ROLE_ARN }}
          aws-region: ap-south-1

      - name: Install Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: false

      - name: Build kubedump
        run: |
          git clone https://github.com/SubhrajitPrusty/kubedump /tmp/kubedump
          cd /tmp/kubedump && go build -o /usr/local/bin/kubedump .

      - uses: azure/setup-kubectl@v4
      - uses: azure/setup-helm@v4

      - name: Install kubectl-neat
        run: |
          NEAT_VERSION=$(curl -sSf https://api.github.com/repos/itaysk/kubectl-neat/releases/latest \
            | grep '"tag_name"' | sed 's/.*"v\([^"]*\)".*/\1/')
          curl -sSfL \
            "https://github.com/itaysk/kubectl-neat/releases/download/v${NEAT_VERSION}/kubectl-neat_linux_amd64.tar.gz" \
            | tar -xz -C /usr/local/bin kubectl-neat

      - name: Update kubeconfig
        run: |
          for cluster in my-api-cluster my-ws-cluster; do
            aws eks update-kubeconfig --region ap-south-1 --name "$cluster"
          done

      - name: Run kubedump refresh
        run: kubedump refresh

      - name: Commit and open PR
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          git add -A
          git diff --cached --quiet && exit 0
          git config user.name  "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git checkout -b chore/kubedump-refresh
          git commit -m "chore: kubedump refresh $(date -u '+%Y-%m-%d %H:%M UTC')"
          git push --force-with-lease origin chore/kubedump-refresh
          gh pr create \
            --title "chore: kubedump refresh" \
            --body "Automated manifest refresh from live clusters." \
            --base main --head chore/kubedump-refresh || true
```

The IAM role attached to `AWS_DEPLOY_ROLE_ARN` needs `eks:DescribeCluster` to call `update-kubeconfig`, plus whatever RBAC the target cluster grants it.
