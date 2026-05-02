# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial Helm plugin scaffold (`helm kubescape scan <chart>`).
- Flag translation for `-f` / `--values`, `--set`, `--set-string`, `--set-file`,
  `-n` / `--namespace`, `--release-name`, `--release-namespace` — forwarded to
  `kubescape scan`.
- Smoke-test suite covering 8 flag-translation cases.
- GitHub Actions workflows: `pr-created.yaml` (shellcheck + smoke + e2e against
  kubescape master) and `release.yaml` (tag-driven source tarball).
- Standard issue / PR templates and Dependabot config for GitHub Actions.

### Notes
- Requires a `kubescape` CLI release that supports the Helm value-override flags
  (`--values`, `--set`, `--set-string`, `--set-file`, `--release-name`,
  `--release-namespace`). Tracked in
  [kubescape/kubescape#1883](https://github.com/kubescape/kubescape/issues/1883).
