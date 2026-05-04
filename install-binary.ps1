# install-binary.ps1: Helm plugin install/update hook (Windows).
#
# Mirror of install-binary.sh: builds the helm-kubescape Go binary into
# $HELM_PLUGIN_DIR\bin\helm-kubescape.exe. Falls back to a friendly error if no
# Go toolchain is available (prebuilt-binary download is a TODO once the plugin
# tags a release).
#
# Capability-probes the kubescape CLI and warns (without failing) if the
# installed kubescape predates the Helm-values flags.

$ErrorActionPreference = 'Stop'

if ($env:HELM_PLUGIN_DIR) {
    $PluginDir = $env:HELM_PLUGIN_DIR
} else {
    $PluginDir = Split-Path -Parent $MyInvocation.MyCommand.Path
}
$BinDir = Join-Path $PluginDir 'bin'
$BinPath = Join-Path $BinDir 'helm-kubescape.exe'

if ($env:KUBESCAPE_BIN) { $KubescapeBin = $env:KUBESCAPE_BIN } else { $KubescapeBin = 'kubescape' }

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

if (Get-Command go -ErrorAction SilentlyContinue) {
    Write-Host 'helm-kubescape: building binary from source...'
    Push-Location $PluginDir
    try {
        & go build -trimpath -ldflags='-s -w' -o $BinPath ./cmd/helm-kubescape
        if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
    } finally {
        Pop-Location
    }
} else {
    Write-Error @"
helm-kubescape: 'go' is not in PATH and no prebuilt-binary download is wired up yet.

Install Go (https://go.dev/dl/) and re-run:
    helm plugin update kubescape

Or download a prebuilt release manually and place it at:
    $BinPath
"@
    exit 1
}

if (-not (Get-Command $KubescapeBin -ErrorAction SilentlyContinue)) {
    Write-Warning @"
helm-kubescape: '$KubescapeBin' was not found in PATH.

The plugin requires the kubescape CLI to be installed separately. Install it
following https://kubescape.io/docs/install-cli/ before running:

    helm kubescape scan <chart>

(You can also point KUBESCAPE_BIN at a custom path.)
"@
    exit 0
}

# Capability probe: warn if the installed kubescape predates the Helm-values flags.
$helpOut = & $KubescapeBin scan --help 2>$null
if (-not ($helpOut -match '--values' -or $helpOut -match '--set')) {
    $ver = (& $KubescapeBin version 2>$null | Select-Object -First 1)
    Write-Warning @"
helm-kubescape: the installed kubescape ($ver) does not appear to support the Helm value-override flags (--values / --set).

Upgrade kubescape to a release that includes the Helm-values-overrides change before using this plugin.
"@
}

Write-Host "helm-kubescape installed at $BinPath. Try: helm kubescape help"
