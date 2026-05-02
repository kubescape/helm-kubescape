# helm-kubescape

A Helm plugin that scans a Helm chart with [Kubescape](https://kubescape.io/) for security misconfigurations, vulnerabilities, and compliance — applying user-supplied Helm value overrides (`-f` / `--set` / `--set-string` / `--set-file`) and release identity (`--release-name` / `--release-namespace`) before the scan.

The plugin is a thin wrapper around `kubescape scan`. Per-template source mapping in findings is preserved by Kubescape's own renderer (no `helm template | kubescape scan -` style flattening).

> **Status:** experimental — depends on the Helm-values-overrides change in `kubescape/kubescape` ([#1883](https://github.com/kubescape/kubescape/issues/1883) prerequisite). Build kubescape from `master` (or any release that includes the change) before installing this plugin.

## Install

```bash
helm plugin install https://github.com/kubescape/helm-kubescape
```

The plugin shells out to a locally installed `kubescape` CLI. Install it from <https://kubescape.io/docs/install-cli/> if you don't already have it. Override the binary path with `KUBESCAPE_BIN=/path/to/kubescape` if needed.

## Usage

```
helm kubescape scan <chart> [helm flags] [kubescape flags]
helm kubescape version
helm kubescape help
```

### Helm-style flags forwarded to Kubescape

| Helm flag | Forwarded as | Notes |
|---|---|---|
| `-f`, `--values FILE` | `--values FILE` | repeatable |
| `--set KEY=VAL` | `--set KEY=VAL` | repeatable |
| `--set-string KEY=VAL` | `--set-string KEY=VAL` | repeatable |
| `--set-file KEY=PATH` | `--set-file KEY=PATH` | repeatable |
| `-n`, `--namespace NS` | `--release-namespace NS` | sets `.Release.Namespace` |
| `--release-name NAME` | `--release-name NAME` | sets `.Release.Name` |
| `--release-namespace NS` | `--release-namespace NS` | sets `.Release.Namespace` |

Any other flag is forwarded verbatim to `kubescape scan`, so you can mix in Kubescape-native options:

```bash
# CI gate: fail if any high-severity finding is reported
helm kubescape scan ./mychart \
    -f values-prod.yaml --set image.tag=v2 \
    --release-name prod -n prod \
    --severity-threshold high

# Save results as JSON
helm kubescape scan ./mychart --set image.pullPolicy=Never \
    --format json --output scan.json
```

## How it works

1. The plugin entry script (`scripts/helm-kubescape.sh`) parses the user's argv.
2. It rewrites only the flags whose names differ between Helm and Kubescape (`-f` → `--values`, `-n` → `--release-namespace`); everything else is passed through unchanged.
3. It execs the local `kubescape` binary with `scan` as the first arg followed by the translated argv.
4. Exit code is `kubescape`'s exit code, so the plugin works as a `helm install --pre-install` gate or in CI pipelines. Invalid Helm value overrides (bad `--set`, missing `-f` file, unreadable `--set-file` path, etc.) cause kubescape to exit `1` rather than silently scanning chart defaults — the plugin propagates that exit code unchanged.

### Flag-binding parity with Helm

The plugin does not split or rewrite values; it only renames flags whose names differ between Helm and Kubescape. Comma handling matches Helm exactly because Kubescape mirrors Helm's flag-binding choices:

- `--values` is comma-split (`-f a.yaml,b.yaml` → two files), the same as `helm install -f a.yaml,b.yaml`.
- `--set` / `--set-string` / `--set-file` are taken verbatim (`--set tolerations={a,b}` is a single value with a brace), the same as `helm install --set tolerations={a,b}`.

## Development

```bash
# Run the flag-translation smoke tests (no kubescape required)
make test
# or:
bash test/smoke.sh

# Lint shell scripts
make lint    # requires shellcheck

# Install this checkout as a local plugin and try it
make install
helm kubescape help
```

## License

Apache-2.0 (matches the parent Kubescape project).
