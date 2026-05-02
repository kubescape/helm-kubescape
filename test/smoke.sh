#!/usr/bin/env bash
# Smoke test for the helm-kubescape plugin.
#
# Invokes the plugin script directly (without going through `helm`) and checks
# that flag translation and forwarding produce the expected kubescape command
# line. We don't actually run kubescape here; we shadow it with a fake binary
# that records its argv and exits 0.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="${ROOT}/scripts/helm-kubescape.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

# Fake kubescape: writes its argv to a file and exits cleanly.
cat >"${TMP}/kubescape" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" > "${ARGS_FILE}"
exit 0
EOF
chmod +x "${TMP}/kubescape"

export ARGS_FILE="${TMP}/argv"
export KUBESCAPE_BIN="${TMP}/kubescape"

assert_args() {
  local label="$1"; shift
  local expected="$*"
  local actual
  actual="$(tr '\n' ' ' < "${ARGS_FILE}" | sed 's/ *$//')"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "FAIL: ${label}"
    echo "  expected: ${expected}"
    echo "  actual:   ${actual}"
    exit 1
  fi
  echo "ok: ${label}"
}

# Case 1: -f translates to --values; --set passes through.
"${SCRIPT}" scan ./mychart -f vals.yaml --set image.tag=v1
assert_args "case1: -f -> --values, --set passthrough" \
  "scan ./mychart --values vals.yaml --set image.tag=v1"

# Case 2: --values=path inline form.
"${SCRIPT}" scan ./mychart --values=prod.yaml
assert_args "case2: --values=PATH inline" \
  "scan ./mychart --values prod.yaml"

# Case 3: -n maps to --release-namespace.
"${SCRIPT}" scan ./mychart -n prod --release-name myrel
assert_args "case3: -n -> --release-namespace, --release-name passthrough" \
  "scan ./mychart --release-namespace prod --release-name myrel"

# Case 4: kubescape-native flags pass through unchanged.
"${SCRIPT}" scan ./mychart --format json --output out.json --severity-threshold high
assert_args "case4: kubescape-native flags pass through" \
  "scan ./mychart --format json --output out.json --severity-threshold high"

# Case 5: --set-string and --set-file pass through verbatim.
"${SCRIPT}" scan ./mychart --set-string foo=bar --set-file ca=ca.pem
assert_args "case5: --set-string / --set-file passthrough" \
  "scan ./mychart --set-string foo=bar --set-file ca=ca.pem"

# Case 6: repeatable -f / --set are preserved.
"${SCRIPT}" scan ./mychart -f a.yaml -f b.yaml --set k1=v1 --set k2=v2
assert_args "case6: repeatable -f and --set" \
  "scan ./mychart --values a.yaml --values b.yaml --set k1=v1 --set k2=v2"

# Case 6b: -f with a comma-separated list (Helm splits this into multiple files).
# The plugin must pass the comma-bearing arg through unchanged so kubescape's
# StringSliceVar binding for --values produces the same two-file result.
"${SCRIPT}" scan ./mychart -f a.yaml,b.yaml
assert_args "case6b: -f a.yaml,b.yaml passes commas through to --values" \
  "scan ./mychart --values a.yaml,b.yaml"

# Case 6c: --set values containing commas inside braces must NOT be split by
# the plugin. Kubescape's StringArrayVar binding for --set preserves them
# verbatim for Helm's strvals parser.
"${SCRIPT}" scan ./mychart --set "tolerations={a,b}"
assert_args "case6c: --set tolerations={a,b} preserved verbatim" \
  "scan ./mychart --set tolerations={a,b}"

# Case 7: unknown subcommand exits non-zero.
if "${SCRIPT}" frobnicate ./mychart 2>/dev/null; then
  echo "FAIL: unknown subcommand should exit non-zero"
  exit 1
fi
echo "ok: case7: unknown subcommand rejected"

# Case 8: help / version don't shell out to kubescape.
"${SCRIPT}" help >/dev/null
echo "ok: case8: help works"

echo "All smoke tests passed."
