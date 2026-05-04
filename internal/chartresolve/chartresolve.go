// Package chartresolve turns a user-supplied chart reference into a local
// directory path that `kubescape scan` can consume.
//
// Why this exists: `kubescape scan` renders Helm charts from a local directory
// (it walks the filesystem looking for Chart.yaml). It does not know how to pull
// from an OCI registry, an HTTP URL, a `repo/chart` reference, or even unpack a
// .tgz. Helm itself supports all of these natively via its SDK; the plugin is
// the natural place to bridge the two.
//
// Detection rules (checked in order):
//   - empty string                    -> error (caller should require a chart arg)
//   - existing directory              -> return as-is, no temp dir
//   - existing file ending in .tgz    -> unpack to temp dir
//   - oci:// prefix                   -> Helm SDK pull + untar
//   - http:// or https:// prefix      -> Helm SDK pull + untar
//   - "repo/chart" form (one slash,
//     not a path)                     -> Helm SDK pull + untar (uses configured repos)
//   - otherwise                       -> return as-is (let kubescape produce the
//                                        not-found error against the user's literal input)
//
// The caller is responsible for cleaning up any temp dir; Resolve returns a
// cleanup func that is always safe to call (no-op for the local-dir case).
package chartresolve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
)

// Result describes a resolved chart.
type Result struct {
	// LocalPath is a directory on disk containing Chart.yaml at its root.
	LocalPath string
	// IsTemp is true if LocalPath points into a temp dir created by Resolve.
	IsTemp bool
}

// Options controls Resolve's behavior. Zero value is fine: defaults match
// `helm pull` with no extra flags. Version is the chart version to request when
// pulling a remote ref; ignored for local dirs and .tgz inputs.
type Options struct {
	Version string
}

// Resolve turns a chart reference into a local directory. The returned cleanup
// func is always non-nil and always safe to call — for local-directory inputs
// it's a no-op; for pulled or unpacked charts it removes the temp dir.
func Resolve(ref string, opts Options) (Result, func(), error) {
	noop := func() {}
	if ref == "" {
		return Result{}, noop, errors.New("chart reference is empty")
	}

	// Existing local directory: trivial passthrough.
	if info, err := os.Stat(ref); err == nil && info.IsDir() {
		return Result{LocalPath: ref, IsTemp: false}, noop, nil
	}

	// Existing local .tgz file: unpack to temp dir.
	if info, err := os.Stat(ref); err == nil && !info.IsDir() && hasTgzSuffix(ref) {
		dir, cleanup, err := untarToTempDir(ref)
		if err != nil {
			return Result{}, noop, fmt.Errorf("unpacking %s: %w", ref, err)
		}
		return Result{LocalPath: dir, IsTemp: true}, cleanup, nil
	}

	// Remote refs Helm can pull: oci://, http(s)://, or repo/chart.
	if isRemoteRef(ref) {
		return pullAndUntar(ref, opts)
	}

	// Local path that doesn't exist (or repo/chart we don't recognize): pass through.
	// Letting kubescape produce the error keeps error messaging aligned with what the
	// user typed instead of wrapping it in plugin-speak.
	return Result{LocalPath: ref, IsTemp: false}, noop, nil
}

func hasTgzSuffix(p string) bool {
	lp := strings.ToLower(p)
	return strings.HasSuffix(lp, ".tgz") || strings.HasSuffix(lp, ".tar.gz")
}

// isRemoteRef reports whether ref should be handed to Helm's pull machinery.
// We treat as remote: oci://, http(s)://, and the "repo/chart" shorthand.
// "repo/chart" detection is heuristic — exactly one slash, no path separators on
// the operating system that would make it a relative path, and the ref isn't an
// existing filesystem entry. This matches how `helm install` distinguishes its
// arg.
func isRemoteRef(ref string) bool {
	switch {
	case strings.HasPrefix(ref, "oci://"):
		return true
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return true
	}
	// repo/chart heuristic. Reject anything that looks like a relative path.
	if strings.HasPrefix(ref, ".") || strings.HasPrefix(ref, "/") {
		return false
	}
	if strings.Contains(ref, string(os.PathSeparator)) && os.PathSeparator != '/' {
		// On Windows a backslash means it's a path, not a repo/chart ref.
		return false
	}
	if strings.Count(ref, "/") != 1 {
		return false
	}
	if _, err := os.Stat(ref); err == nil {
		// Exists on disk — treat as a path.
		return false
	}
	return true
}

// newPullAction configures an action.Pull mirroring Helm's CLI defaults
// (cmd/helm/root.go newDefaultRegistryClient + the pull action wiring), with
// our own DestDir/UntarDir override and chart-version pinning. Extracted so
// tests can assert the registry client is wired (otherwise oci:// pulls fail
// at runtime with a nil-client deref).
func newPullAction(opts Options, destDir string) (*action.Pull, *action.Configuration, error) {
	settings := cli.New()
	regClient, err := registry.NewClient(
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(os.Stderr),
		registry.ClientOptCredentialsFile(settings.RegistryConfig),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating registry client: %w", err)
	}
	cfg := &action.Configuration{RegistryClient: regClient}
	pull := action.NewPullWithOpts(action.WithConfig(cfg))
	pull.Settings = settings
	pull.DestDir = destDir
	pull.Untar = true
	pull.UntarDir = destDir
	pull.Version = opts.Version
	return pull, cfg, nil
}

// pullAndUntar uses Helm's pull action to fetch a chart and untar it into a temp dir.
// We rely on Helm's CLI environment defaults (cli.New()) so users who already configured
// `helm repo add` / `HELM_REPOSITORY_CONFIG` etc. don't have to reconfigure anything.
func pullAndUntar(ref string, opts Options) (Result, func(), error) {
	noop := func() {}

	tempDir, err := os.MkdirTemp("", "helm-kubescape-")
	if err != nil {
		return Result{}, noop, fmt.Errorf("creating temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	pull, _, err := newPullAction(opts, tempDir)
	if err != nil {
		cleanup()
		return Result{}, noop, err
	}

	// repo/chart references need the repo URL resolved. Pull's RepoURL+Ref split is
	// driven by the ChartPathOptions; for "repo/chart" form, action.Pull handles
	// repo lookup against the configured repo cache automatically when we just
	// pass the ref.
	if _, err := pull.Run(ref); err != nil {
		cleanup()
		return Result{}, noop, fmt.Errorf("pulling chart %q: %w", ref, err)
	}

	// pull.Run with Untar=true writes the chart into UntarDir/<chart-name>/.
	// Find the single subdirectory containing Chart.yaml.
	dir, err := findChartDir(tempDir)
	if err != nil {
		cleanup()
		return Result{}, noop, err
	}

	return Result{LocalPath: dir, IsTemp: true}, cleanup, nil
}

// findChartDir locates the directory containing Chart.yaml under root.
// `helm pull --untar` produces root/<chart-name>/Chart.yaml.
func findChartDir(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("reading temp dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(candidate, "Chart.yaml")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no Chart.yaml found under %s after pull", root)
}

// untarToTempDir extracts a .tgz chart into a fresh temp dir and returns the
// directory containing Chart.yaml.
func untarToTempDir(tgzPath string) (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "helm-kubescape-")
	if err != nil {
		return "", func() {}, fmt.Errorf("creating temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	// Use Helm's own chartutil.ExpandFile so we don't reimplement archive/tar
	// handling (and so we get the same validation Helm itself applies).
	if err := expandTgz(tgzPath, tempDir); err != nil {
		cleanup()
		return "", func() {}, err
	}
	dir, err := findChartDir(tempDir)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return dir, cleanup, nil
}
