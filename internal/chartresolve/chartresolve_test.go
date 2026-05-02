package chartresolve

import (
	"os"
	"path/filepath"
	"testing"
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
	res, cleanup, err := Resolve(tmp)
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
	_, cleanup, err := Resolve("")
	defer cleanup()
	if err == nil {
		t.Fatalf("expected error for empty ref")
	}
}

func TestResolve_nonexistentPath_passthrough(t *testing.T) {
	// A path-looking ref that doesn't exist should pass through unchanged so that
	// kubescape produces the not-found error against the user's literal input.
	res, cleanup, err := Resolve("./this-does-not-exist")
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
