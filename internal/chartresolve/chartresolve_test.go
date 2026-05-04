package chartresolve

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"sigs.k8s.io/yaml"
)

func TestIsRemoteRef(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"./mychart", false},
		{"/abs/path/chart", false},
		{"mychart", false},          // no slash, treated as local-name passthrough
		{"a/b/c", false},             // multiple slashes — not a repo/chart shorthand
		{"oci://ghcr.io/foo/bar", true},
		{"https://example.com/chart-1.0.0.tgz", true},
		{"http://example.com/chart-1.0.0.tgz", true},
		{"bitnami/nginx", true},      // repo/chart shorthand, doesn't exist on disk
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := isRemoteRef(tt.ref)
			if got != tt.want {
				t.Errorf("isRemoteRef(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

// TestIsRemoteRef_existingPathNotRemote ensures that an entry that looks like
// "repo/chart" but exists on disk (e.g. user has a folder literally named "a/b")
// is not treated as a remote ref.
func TestIsRemoteRef_existingPathNotRemote(t *testing.T) {
	tmp := t.TempDir()
	subdir := filepath.Join(tmp, "child")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	// "child/something" with the directory existing — but isRemoteRef only stat's the
	// whole ref; we test the simpler "child" + an empty file case here.
	if err := os.WriteFile(filepath.Join(tmp, "a"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	// "a/b" doesn't exist as a file path here (a is a file, not a dir), and it's not
	// matching any of our heuristics — but the existence check at the bottom should
	// keep us safe in the common case where the user has a literal directory named
	// "a/b" relative to cwd.
	if err := os.MkdirAll(filepath.Join(tmp, "real", "chart"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := isRemoteRef("real/chart"); got {
		t.Errorf("isRemoteRef(real/chart) with directory present = true, want false")
	}
}

func TestResolve_localDir(t *testing.T) {
	tmp := t.TempDir()
	res, cleanup, err := Resolve(tmp, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer cleanup()
	if res.LocalPath != tmp {
		t.Errorf("LocalPath = %q, want %q", res.LocalPath, tmp)
	}
	if res.IsTemp {
		t.Errorf("IsTemp = true for local dir, want false")
	}
}

func TestResolve_empty(t *testing.T) {
	_, cleanup, err := Resolve("", Options{})
	defer cleanup()
	if err == nil {
		t.Fatalf("expected error for empty ref")
	}
}

// TestResolve_tgz exercises the .tgz unpack path end-to-end. We build a minimal
// chart archive (Chart.yaml + values.yaml inside a single directory, the layout
// `helm package` produces) and confirm Resolve points at the unpacked chart dir.
func TestResolve_tgz(t *testing.T) {
	tmp := t.TempDir()
	tgz := filepath.Join(tmp, "mychart-0.1.0.tgz")
	writeChartTgz(t, tgz, "mychart", map[string]string{
		"mychart/Chart.yaml":  "apiVersion: v2\nname: mychart\nversion: 0.1.0\n",
		"mychart/values.yaml": "image: nginx\n",
	})

	res, cleanup, err := Resolve(tgz, Options{})
	if err != nil {
		t.Fatalf("Resolve(%q): %v", tgz, err)
	}
	defer cleanup()
	if !res.IsTemp {
		t.Errorf("IsTemp = false, want true for .tgz unpack")
	}
	if _, err := os.Stat(filepath.Join(res.LocalPath, "Chart.yaml")); err != nil {
		t.Errorf("expected Chart.yaml under %s: %v", res.LocalPath, err)
	}
}

// TestResolve_tgz_cleanup confirms the temp dir is removed when cleanup() runs.
func TestResolve_tgz_cleanup(t *testing.T) {
	tmp := t.TempDir()
	tgz := filepath.Join(tmp, "c.tgz")
	writeChartTgz(t, tgz, "c", map[string]string{
		"c/Chart.yaml": "apiVersion: v2\nname: c\nversion: 0.1.0\n",
	})

	res, cleanup, err := Resolve(tgz, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	parent := filepath.Dir(res.LocalPath) // temp dir created by untarToTempDir
	cleanup()
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Errorf("cleanup did not remove temp dir %s: %v", parent, err)
	}
}

// writeChartTgz builds a gzipped tar of files (path -> contents) at dst.
func writeChartTgz(t *testing.T, dst, _ string, files map[string]string) {
	t.Helper()
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
}

// TestResolve_httpsURL covers the http(s):// URL path: a packaged .tgz served
// directly. action.Pull treats a full URL as the chart ref and bypasses the
// repo-cache lookup, exercising getter.HTTPGetter end-to-end. http:// is
// equivalent on the wire — picking it for the test avoids self-signed-cert
// plumbing while still going through the same code path.
func TestResolve_httpsURL(t *testing.T) {
	docroot := t.TempDir()
	tgzPath := saveChart(t, docroot, "mychart", "0.1.0")
	srv := httptest.NewServer(fileServer(docroot))
	t.Cleanup(srv.Close)

	url := fmt.Sprintf("%s/%s", srv.URL, filepath.Base(tgzPath))
	res, cleanup, err := Resolve(url, Options{})
	if err != nil {
		t.Fatalf("Resolve(%q): %v", url, err)
	}
	defer cleanup()
	if !res.IsTemp {
		t.Errorf("IsTemp = false, want true for URL pull")
	}
	if _, err := os.Stat(filepath.Join(res.LocalPath, "Chart.yaml")); err != nil {
		t.Errorf("expected Chart.yaml under %s: %v", res.LocalPath, err)
	}
}

// TestResolve_repoChart covers the "repo/chart" shorthand path. We stand up a
// minimal HTTP repo (.tgz + index.yaml) and point HELM_REPOSITORY_CONFIG at a
// repositories.yaml that registers it as "test", mirroring how an end-user
// with `helm repo add` configured would invoke the plugin.
func TestResolve_repoChart(t *testing.T) {
	docroot := t.TempDir()
	saveChart(t, docroot, "mychart", "0.2.0")
	srv := httptest.NewServer(fileServer(docroot))
	t.Cleanup(srv.Close)

	writeIndex(t, docroot, srv.URL, "mychart", "0.2.0")
	cacheDir := t.TempDir()
	repoConfig := writeReposYAML(t, t.TempDir(), "test", srv.URL)
	// Helm reads the cached index at <CACHE>/<repo-name>-index.yaml; without it,
	// action.Pull errors with "no cached repo found".
	if data, err := os.ReadFile(filepath.Join(docroot, "index.yaml")); err != nil {
		t.Fatal(err)
	} else if err := os.WriteFile(filepath.Join(cacheDir, "test-index.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HELM_REPOSITORY_CONFIG", repoConfig)
	t.Setenv("HELM_REPOSITORY_CACHE", cacheDir)

	res, cleanup, err := Resolve("test/mychart", Options{Version: "0.2.0"})
	if err != nil {
		t.Fatalf("Resolve(test/mychart): %v", err)
	}
	defer cleanup()
	if !res.IsTemp {
		t.Errorf("IsTemp = false, want true for repo/chart pull")
	}
	if _, err := os.Stat(filepath.Join(res.LocalPath, "Chart.yaml")); err != nil {
		t.Errorf("expected Chart.yaml under %s: %v", res.LocalPath, err)
	}
}

// TestNewPullAction_registryClientWired guards matthyx's blocker: action.Pull
// without a registry client deref-panics on oci:// pulls. We don't run a full
// OCI roundtrip here — that requires an htpasswd-authenticated distribution
// registry which transitively breaks our go.mod (helm's repotest pulls in
// `distribution/distribution/v3` whose deps don't resolve cleanly). Verifying
// that Configuration carries a non-nil RegistryClient catches the regression
// matthyx flagged; a full OCI roundtrip belongs in a containerized CI step.
func TestNewPullAction_registryClientWired(t *testing.T) {
	tmp := t.TempDir()
	pull, cfg, err := newPullAction(Options{Version: "1.2.3"}, tmp)
	if err != nil {
		t.Fatalf("newPullAction: %v", err)
	}
	if cfg.RegistryClient == nil {
		t.Fatal("Configuration.RegistryClient is nil; oci:// pulls would panic")
	}
	if pull.Version != "1.2.3" {
		t.Errorf("pull.Version = %q, want 1.2.3", pull.Version)
	}
	if pull.UntarDir != tmp {
		t.Errorf("pull.UntarDir = %q, want %q", pull.UntarDir, tmp)
	}
	if !pull.Untar {
		t.Error("pull.Untar = false, want true")
	}
}

// saveChart packages a minimal chart into dir as <name>-<version>.tgz and
// returns its absolute path.
func saveChart(t *testing.T, dir, name, version string) string {
	t.Helper()
	c := &chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion: chart.APIVersionV2,
			Name:       name,
			Version:    version,
		},
	}
	tgz, err := chartutil.Save(c, dir)
	if err != nil {
		t.Fatalf("chartutil.Save: %v", err)
	}
	return tgz
}

// fileServer returns an http.Handler serving files from root. Plain wrapper so
// the tests don't repeat the http.FileServer/http.Dir incantation.
func fileServer(root string) http.Handler {
	return http.FileServer(http.Dir(root))
}

// writeIndex writes a Helm v1 repository index.yaml into docroot pointing at
// the .tgz served from baseURL. Helm validates the digest after download, so
// we compute the real sha256 here.
func writeIndex(t *testing.T, docroot, baseURL, chartName, version string) {
	t.Helper()
	tgz := filepath.Join(docroot, fmt.Sprintf("%s-%s.tgz", chartName, version))
	data, err := os.ReadFile(tgz)
	if err != nil {
		t.Fatalf("read tgz: %v", err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])

	index := map[string]any{
		"apiVersion": "v1",
		"entries": map[string]any{
			chartName: []map[string]any{{
				"apiVersion": "v2",
				"name":       chartName,
				"version":    version,
				"urls":       []string{fmt.Sprintf("%s/%s-%s.tgz", baseURL, chartName, version)},
				"digest":     digest,
			}},
		},
	}
	out, err := yaml.Marshal(index)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docroot, "index.yaml"), out, 0o644); err != nil {
		t.Fatalf("write index.yaml: %v", err)
	}
}

// writeReposYAML writes a repositories.yaml registering a single repo by name
// and returns its path. Mirrors the file `helm repo add` produces.
func writeReposYAML(t *testing.T, dir, repoName, repoURL string) string {
	t.Helper()
	body := fmt.Sprintf("apiVersion: \"\"\ngenerated: \"0001-01-01T00:00:00Z\"\nrepositories:\n  - name: %s\n    url: %s\n", repoName, repoURL)
	p := filepath.Join(dir, "repositories.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write repositories.yaml: %v", err)
	}
	return p
}

func TestResolve_nonexistentPath_passthrough(t *testing.T) {
	// A path-looking ref that doesn't exist should pass through unchanged so that
	// kubescape produces the not-found error against the user's literal input.
	res, cleanup, err := Resolve("./this-does-not-exist", Options{})
	defer cleanup()
	if err != nil {
		t.Fatalf("Resolve passthrough returned error: %v", err)
	}
	if res.LocalPath != "./this-does-not-exist" {
		t.Errorf("LocalPath = %q, want passthrough", res.LocalPath)
	}
	if res.IsTemp {
		t.Errorf("IsTemp = true, want false for passthrough")
	}
}
