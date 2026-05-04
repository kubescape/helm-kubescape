# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `helm kubescape scan <chart>` Helm plugin (Go binary). Forwards Helm-style
  flag overrides to `kubescape scan`:
  - `-f` / `--values FILE` (repeatable; comma-list splits like `helm install`)
  - `--set KEY=VAL` (repeatable; commas inside braces preserved)
  - `--set-string KEY=VAL` (repeatable)
  - `--set-file KEY=PATH` (repeatable)
  - `-n` / `--namespace NS` → `--release-namespace NS`
  - `--release-name NAME`, `--release-namespace NS`
  Any unrecognized flag is forwarded verbatim to `kubescape scan`, so
  Kubescape-native flags (`--format`, `--output`, `--severity-threshold`,
  `--compliance-threshold`, …) work without the plugin enumerating them.
- Remote chart resolution via Helm's SDK: `oci://`, `https://`, `repo/chart`,
  and local `.tgz` references are pulled and unpacked into a temp dir before
  scanning. Local directories pass through unchanged. Temp dirs are cleaned up
  after each run.
- `helm kubescape version` and `helm kubescape help` subcommands.
- Unit tests for flag translation and chart-ref detection.
- GitHub Actions workflows: `pr-created.yaml` (`go vet` + Linux/macOS/Windows
  unit-test matrix + e2e against `kubescape` master) and `release.yaml`
  (tag-driven source tarball, gated on `plugin.yaml`'s `version:` matching the
  tag).
- Standard issue / PR templates and Dependabot config for GitHub Actions.

### Notes
- Requires a `kubescape` CLI release that supports the Helm value-override
  flags (`--values`, `--set`, `--set-string`, `--set-file`, `--release-name`,
  `--release-namespace`). Tracked in
  [kubescape/kubescape#1883](https://github.com/kubescape/kubescape/issues/1883).
