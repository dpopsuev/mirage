package testkit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/mirage"
)

// AssertClean fails if the Space has any uncommitted changes.
func AssertClean(t *testing.T, s mirage.Space) {
	t.Helper()
	changes, err := s.Diff()
	if err != nil {
		t.Fatalf("Diff() failed: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected clean Space, got %d changes: %v", len(changes), changes)
	}
}

// AssertDirty fails if the Space has no uncommitted changes.
func AssertDirty(t *testing.T, s mirage.Space) {
	t.Helper()
	changes, err := s.Diff()
	if err != nil {
		t.Fatalf("Diff() failed: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected dirty Space, got 0 changes")
	}
}

// AssertDiffCount fails if the Space's Diff does not return exactly n changes.
func AssertDiffCount(t *testing.T, s mirage.Space, n int) {
	t.Helper()
	changes, err := s.Diff()
	if err != nil {
		t.Fatalf("Diff() failed: %v", err)
	}
	if len(changes) != n {
		t.Fatalf("expected %d changes, got %d: %v", n, len(changes), changes)
	}
}

// AssertFileInWorkDir fails if the file does not exist in the Space's WorkDir.
func AssertFileInWorkDir(t *testing.T, s mirage.Space, relPath string) {
	t.Helper()
	full := filepath.Join(s.WorkDir(), relPath)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		t.Fatalf("expected %s in WorkDir, not found", relPath)
	}
}

// AssertFileNotInWorkDir fails if the file exists in the Space's WorkDir.
func AssertFileNotInWorkDir(t *testing.T, s mirage.Space, relPath string) {
	t.Helper()
	full := filepath.Join(s.WorkDir(), relPath)
	if _, err := os.Stat(full); err == nil {
		t.Fatalf("expected %s not in WorkDir, but found it", relPath)
	}
}

// AssertCommitted fails if the file does not exist in the real workspace.
func AssertCommitted(t *testing.T, workspace, relPath string) {
	t.Helper()
	full := filepath.Join(workspace, relPath)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		t.Fatalf("expected %s committed to workspace, not found", relPath)
	}
}

// AssertNotCommitted fails if the file exists in the real workspace.
func AssertNotCommitted(t *testing.T, workspace, relPath string) {
	t.Helper()
	full := filepath.Join(workspace, relPath)
	if _, err := os.Stat(full); err == nil {
		t.Fatalf("expected %s not committed to workspace, but found it", relPath)
	}
}

// AssertFileContent fails if the file's content doesn't match expected.
func AssertFileContent(t *testing.T, s mirage.Space, relPath, expected string) {
	t.Helper()
	full := filepath.Join(s.WorkDir(), relPath)
	data, err := os.ReadFile(full) //nolint:gosec // test helper
	if err != nil {
		t.Fatalf("cannot read %s: %v", relPath, err)
	}
	if string(data) != expected {
		t.Fatalf("file %s content = %q, want %q", relPath, string(data), expected)
	}
}

// AssertDiffContains fails if the given path is not in Diff results.
func AssertDiffContains(t *testing.T, s mirage.Space, relPath string) {
	t.Helper()
	changes, err := s.Diff()
	if err != nil {
		t.Fatalf("Diff() failed: %v", err)
	}
	for _, c := range changes {
		if c.Path == relPath {
			return
		}
	}
	t.Fatalf("expected %s in Diff, not found in %v", relPath, changes)
}

// AssertDiffNotContains fails if the given path IS in Diff results.
func AssertDiffNotContains(t *testing.T, s mirage.Space, relPath string) {
	t.Helper()
	changes, err := s.Diff()
	if err != nil {
		t.Fatalf("Diff() failed: %v", err)
	}
	for _, c := range changes {
		if c.Path == relPath {
			t.Fatalf("expected %s not in Diff, but found it", relPath)
		}
	}
}
