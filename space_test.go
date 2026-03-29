package mirage

import (
	"os"
	"path/filepath"
	"testing"
)

// plainSpace is a test-only Space using plain directories (no fuse mount).
// This lets us test diff/commit logic without fuse-overlayfs installed.
type plainSpace struct {
	overlaySpace
}

func newPlainSpace(t *testing.T) (*plainSpace, string) {
	t.Helper()

	// Create workspace with some files.
	workspace := t.TempDir()
	writeFile(t, workspace, "existing.go", "package main\n")
	writeFile(t, workspace, "sub/nested.go", "package sub\n")

	// Create upper (overlay) dir.
	tempDir := t.TempDir()
	upper := filepath.Join(tempDir, "upper")
	os.MkdirAll(upper, 0o755)

	s := &plainSpace{
		overlaySpace: overlaySpace{
			lower:   workspace,
			upper:   upper,
			merged:  workspace, // plain mode: merged = lower
			tempDir: tempDir,
			mounted: true,
		},
	}
	return s, workspace
}

func TestDiff_NoChanges(t *testing.T) {
	s, _ := newPlainSpace(t)
	changes, err := s.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}

func TestDiff_CreatedFile(t *testing.T) {
	s, _ := newPlainSpace(t)

	// Write a new file to upper (simulating agent write).
	writeFile(t, s.upper, "new.go", "package new\n")

	changes, err := s.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != Created {
		t.Errorf("kind = %s, want created", changes[0].Kind)
	}
	if changes[0].Path != "new.go" {
		t.Errorf("path = %s, want new.go", changes[0].Path)
	}
}

func TestDiff_ModifiedFile(t *testing.T) {
	s, _ := newPlainSpace(t)

	// Write to upper with same name as existing file.
	writeFile(t, s.upper, "existing.go", "package main // modified\n")

	changes, err := s.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != Modified {
		t.Errorf("kind = %s, want modified", changes[0].Kind)
	}
}

func TestCommit_PromotesFiles(t *testing.T) {
	s, workspace := newPlainSpace(t)

	// Agent creates a new file.
	writeFile(t, s.upper, "promoted.go", "package promoted\n")

	if err := s.Commit([]string{"promoted.go"}); err != nil {
		t.Fatal(err)
	}

	// File should now exist in the real workspace.
	data, err := os.ReadFile(filepath.Join(workspace, "promoted.go"))
	if err != nil {
		t.Fatal("committed file not found in workspace")
	}
	if string(data) != "package promoted\n" {
		t.Errorf("content = %q", data)
	}
}

func TestCommit_NestedDirectory(t *testing.T) {
	s, workspace := newPlainSpace(t)

	writeFile(t, s.upper, "deep/nested/file.go", "package deep\n")

	if err := s.Commit([]string{"deep/nested/file.go"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(workspace, "deep/nested/file.go")); err != nil {
		t.Fatal("nested committed file not found")
	}
}

func TestReset_ClearsUpper(t *testing.T) {
	s, _ := newPlainSpace(t)

	writeFile(t, s.upper, "discard.go", "will be discarded\n")

	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}

	changes, err := s.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes after reset, got %d", len(changes))
	}
}

func TestNotMounted_Errors(t *testing.T) {
	s, _ := newPlainSpace(t)
	s.mounted = false

	if _, err := s.Diff(); err != ErrNotMounted {
		t.Errorf("Diff error = %v, want ErrNotMounted", err)
	}
	if err := s.Commit([]string{"x"}); err != ErrNotMounted {
		t.Errorf("Commit error = %v, want ErrNotMounted", err)
	}
	if err := s.Reset(); err != ErrNotMounted {
		t.Errorf("Reset error = %v, want ErrNotMounted", err)
	}
}

func TestChangeKindValues(t *testing.T) {
	if Created != "created" || Modified != "modified" || Deleted != "deleted" {
		t.Error("ChangeKind constants have wrong values")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
