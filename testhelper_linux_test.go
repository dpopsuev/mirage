//go:build linux

package mirage

import "testing"

// requireFuse skips the test if fuse-overlayfs is not available.
func requireFuse(t *testing.T) {
	t.Helper()
	workspace := t.TempDir()
	s, err := Create(Spec{Workspace: workspace, Backend: Overlay})
	if err != nil {
		t.Skipf("fuse-overlayfs not available: %v", err)
	}
	s.Destroy() //nolint:errcheck
}
