#!/usr/bin/env bash
# helm-kubescape: Helm plugin that scans a Helm chart with Kubescape.
#
# Usage:
#   helm kubescape scan <chart> [helm-style flags] [kubescape flags]
#   helm kubescape version
#   helm kubescape help
#
# Helm-style flags accepted (mirrors `helm install`):
#   -f, --values FILE          (repeatable)  -> kubescape --values FILE
#       --set    KEY=VAL       (repeatable)  -> kubescape --set    KEY=VAL
#       --set-string KEY=VAL   (repeatable)  -> kubescape --set-string KEY=VAL
#       --set-file   KEY=PATH  (repeatable)  -> kubescape --set-file   KEY=PATH
#   -n, --namespace NS                       -> kubescape --release-namespace NS
#       --release-name NAME                  -> kubescape --release-name NAME
#       --release-namespace NS               -> kubescape --release-namespace NS
#
# Any unrecognized flag is forwarded verbatim to `kubescape scan`, so you can
# pass through Kubescape options (e.g. --format json --output out.json,
# --severity-threshold high, --compliance-threshold 80) directly.

set -euo pipefail

PLUGIN_NAME="helm-kubescape"
KUBESCAPE_BIN="${KUBESCAPE_BIN:-kubescape}"

die() {
  echo "${PLUGIN_NAME}: error: $*" >&2
  exit 1
}

print_help() {
  cat <<EOF
helm kubescape - scan a Helm chart with Kubescape

Usage:
  helm kubescape scan <chart> [helm flags] [kubescape flags]
  helm kubescape version
  helm kubescape help

Helm-style value overrides (forwarded to kubescape):
  -f, --values FILE          path to a YAML values file (repeatable)
      --set KEY=VAL          set a value on the command line (repeatable)
      --set-string KEY=VAL   set a STRING value (repeatable)
      --set-file KEY=PATH    set a value from a file (repeatable)
  -n, --namespace NS         release namespace (.Release.Namespace)
      --release-name NAME    release name (.Release.Name)
      --release-namespace NS release namespace (.Release.Namespace)

Any other flags are forwarded verbatim to 'kubescape scan'. Examples:

  # CI gate: fail on any high-severity finding
  helm kubescape scan ./mychart \\
      -f values-prod.yaml --set image.tag=v2 \\
      --release-name prod --namespace prod \\
      --severity-threshold high

  # Save JSON results
  helm kubescape scan ./mychart --set image.pullPolicy=Never \\
      --format json --output scan.json

Environment:
  KUBESCAPE_BIN   path to the kubescape binary (default: 'kubescape' from PATH)

Requirements:
  kubescape >= the release that ships --values/--set support
  (https://github.com/kubescape/kubescape/pull/?? -- the Helm-values-overrides PR)
EOF
}

print_version() {
  local plugin_version
  plugin_version="$(awk -F'"' '/^version:/ {print $2; exit}' "${HELM_PLUGIN_DIR:-.}/plugin.yaml" 2>/dev/null || echo "unknown")"
  echo "${PLUGIN_NAME} version: ${plugin_version}"
  if command -v "${KUBESCAPE_BIN}" >/dev/null 2>&1; then
    "${KUBESCAPE_BIN}" version 2>/dev/null || true
  else
    echo "kubescape: not found in PATH (set KUBESCAPE_BIN to override)"
  fi
}

# Translate a Helm-style flag stream into kubescape flags, populating the
# global KARGS array. We only rewrite the flags that have different names;
# everything else is forwarded verbatim. Repeatable flags are preserved as
# separate argv occurrences so kubescape's flag parser sees each value as
# a distinct entry. Note that kubescape mirrors upstream Helm's binding
# choices: --values is StringSliceVar (so `-f a.yaml,b.yaml` is two files
# upstream, and we pass the comma-bearing arg through unchanged so
# kubescape splits it the same way), while --set / --set-string / --set-file
# are StringArrayVar (so commas inside `--set tolerations={a,b}` survive).
#
# We use a global array (rather than printing newline-separated values and
# using mapfile) for portability: macOS still ships bash 3.2, which lacks
# `mapfile`. Globals + bash-3.2-friendly array syntax work everywhere.
KARGS=()
translate_args() {
  KARGS=()
  while [ $# -gt 0 ]; do
    case "$1" in
      -f|--values)
        [ $# -ge 2 ] || die "flag '$1' requires a value"
        KARGS=("${KARGS[@]}" "--values" "$2")
        shift 2
        ;;
      --values=*)
        KARGS=("${KARGS[@]}" "--values" "${1#--values=}")
        shift
        ;;
      -f=*)
        KARGS=("${KARGS[@]}" "--values" "${1#-f=}")
        shift
        ;;
      -n|--namespace)
        [ $# -ge 2 ] || die "flag '$1' requires a value"
        KARGS=("${KARGS[@]}" "--release-namespace" "$2")
        shift 2
        ;;
      --namespace=*)
        KARGS=("${KARGS[@]}" "--release-namespace" "${1#--namespace=}")
        shift
        ;;
      -n=*)
        KARGS=("${KARGS[@]}" "--release-namespace" "${1#-n=}")
        shift
        ;;
      # Pass-through for flags whose names already match kubescape, including
      # --set / --set-string / --set-file / --release-name / --release-namespace
      # and any kubescape-native flags (--format, --output, --severity-threshold, ...).
      *)
        KARGS=("${KARGS[@]}" "$1")
        shift
        ;;
    esac
  done
}

main() {
  if [ $# -eq 0 ]; then
    print_help
    exit 0
  fi

  local sub="$1"
  shift || true

  case "$sub" in
    -h|--help|help)
      print_help
      exit 0
      ;;
    version|--version|-v)
      print_version
      exit 0
      ;;
    scan)
      ;;
    *)
      die "unknown subcommand '$sub' (try 'helm kubescape help')"
      ;;
  esac

  command -v "${KUBESCAPE_BIN}" >/dev/null 2>&1 \
    || die "'${KUBESCAPE_BIN}' not found in PATH; install from https://kubescape.io/docs/install-cli/"

  translate_args "$@"

  # ${KARGS[@]} expansion is safe with `set -u` only when the array is non-empty
  # under bash 3.2; guard the empty case explicitly.
  if [ ${#KARGS[@]} -eq 0 ]; then
    exec "${KUBESCAPE_BIN}" scan
  fi
  exec "${KUBESCAPE_BIN}" scan "${KARGS[@]}"
}

main "$@"
