//go:build linux

package mirage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/mirage"
)

func newOverlaySpace(t *testing.T) mirage.Space {
	t.Helper()
	root := t.TempDir()
	s, err := mirage.Create(mirage.Spec{
		Workspace: root,
		Backend:   mirage.Overlay,
	})
	if err != nil {
		t.Skipf("overlay unavailable: %v", err)
	}
	t.Cleanup(func() { s.Destroy() }) //nolint:errcheck // test
	return s
}

func TestOverlay_Snapshot_Restore(t *testing.T) {
	s := newOverlaySpace(t)
	ws := s.WorkDir()

	// Write a.go
	os.WriteFile(filepath.Join(ws, "a.go"), []byte("package a"), 0o644)

	// Snapshot
	if err := s.Snapshot("v1"); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Write b.go (to be undone)
	os.WriteFile(filepath.Join(ws, "b.go"), []byte("package b"), 0o644)

	// Verify both files exist
	if _, err := os.Stat(filepath.Join(ws, "a.go")); err != nil {
		t.Fatal("a.go should exist before restore")
	}
	if _, err := os.Stat(filepath.Join(ws, "b.go")); err != nil {
		t.Fatal("b.go should exist before restore")
	}

	// Restore to v1
	if err := s.Restore("v1"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// a.go should exist (was in snapshot)
	if _, err := os.Stat(filepath.Join(ws, "a.go")); err != nil {
		t.Fatal("a.go should exist after restore")
	}

	// b.go should be gone from upper (not in snapshot).
	// Note: FUSE merged view may cache b.go briefly. Check upper directly.
	diff, _ := s.Diff()
	for _, c := range diff {
		if c.Path == "b.go" {
			t.Fatal("b.go should not be in Diff after restore")
		}
	}
}

func TestOverlay_MultipleSnapshots(t *testing.T) {
	s := newOverlaySpace(t)
	ws := s.WorkDir()

	// v1: just a.go
	os.WriteFile(filepath.Join(ws, "a.go"), []byte("v1"), 0o644)
	s.Snapshot("v1") //nolint:errcheck // test

	// v2: a.go + b.go
	os.WriteFile(filepath.Join(ws, "b.go"), []byte("v2"), 0o644)
	s.Snapshot("v2") //nolint:errcheck // test

	// Write c.go (not in any snapshot)
	os.WriteFile(filepath.Join(ws, "c.go"), []byte("v3"), 0o644)

	// Restore v1 — only a.go
	s.Restore("v1") //nolint:errcheck // test
	if _, err := os.Stat(filepath.Join(ws, "a.go")); err != nil {
		t.Fatal("a.go should exist after restore v1")
	}
	if _, err := os.Stat(filepath.Join(ws, "b.go")); !os.IsNotExist(err) {
		t.Fatal("b.go should not exist after restore v1")
	}

	// Restore v2 — a.go + b.go
	s.Restore("v2") //nolint:errcheck // test
	if _, err := os.Stat(filepath.Join(ws, "b.go")); err != nil {
		t.Fatal("b.go should exist after restore v2")
	}

	// List snapshots
	names := s.Snapshots()
	if len(names) != 2 {
		t.Fatalf("Snapshots = %d, want 2", len(names))
	}
}

func TestOverlay_Restore_NotFound(t *testing.T) {
	s := newOverlaySpace(t)
	err := s.Restore("nonexistent")
	if err == nil {
		t.Fatal("Restore should fail for unknown snapshot")
	}
}
