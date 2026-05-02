#!/usr/bin/env bash
# install-binary.sh: Helm plugin install/update hook.
#
# At present we only verify that the `kubescape` CLI is reachable and supports
# the Helm value-override flags (--values / --set / --release-name) introduced
# in the Part-A PR. We deliberately do NOT auto-download the kubescape binary
# yet — that's a follow-up once a kubescape release with these flags is tagged.

set -euo pipefail

KUBESCAPE_BIN="${KUBESCAPE_BIN:-kubescape}"

# Make the main script executable. `helm plugin install` does this for the
# `command:` target on most platforms, but doing it here is harmless and
# covers manual installs from source.
chmod +x "${HELM_PLUGIN_DIR}/scripts/helm-kubescape.sh"

if ! command -v "${KUBESCAPE_BIN}" >/dev/null 2>&1; then
  cat >&2 <<EOF
helm-kubescape: warning - '${KUBESCAPE_BIN}' was not found in PATH.

The plugin requires the kubescape CLI to be installed separately. Install it
following https://kubescape.io/docs/install-cli/ before running:

    helm kubescape scan <chart>

(You can also point KUBESCAPE_BIN at a custom path.)
EOF
  exit 0
fi

# Best-effort capability probe: warn if the installed kubescape predates the
# Helm-values flags. We don't fail the install — users may upgrade kubescape
# afterwards.
if ! "${KUBESCAPE_BIN}" scan --help 2>/dev/null | grep -qE -- '--values|--set'; then
  cat >&2 <<EOF
helm-kubescape: warning - the installed kubescape ($(${KUBESCAPE_BIN} version 2>/dev/null | head -n1)) does not appear to support the Helm value-override flags (--values / --set).

Upgrade kubescape to a release that includes the Helm-values-overrides change before using this plugin.
EOF
fi

echo "helm-kubescape installed. Try: helm kubescape help"
