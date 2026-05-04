package chartresolve

import (
	"fmt"

	"helm.sh/helm/v3/pkg/chartutil"
)

// expandTgz extracts a Helm chart .tgz/.tar.gz at src into dir, producing
// dir/<chart-name>/. Thin wrapper around chartutil.ExpandFile so the resolver
// doesn't reimplement archive/tar handling.
func expandTgz(src, dir string) error {
	if err := chartutil.ExpandFile(dir, src); err != nil {
		return fmt.Errorf("expanding %s: %w", src, err)
	}
	return nil
}
