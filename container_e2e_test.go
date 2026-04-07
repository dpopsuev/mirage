//go:build e2e

package mirage

import (
	"os"
	"path/filepath"
	"testing"
)

// TestContainer_E2E_FullLifecycle tests the complete container backend lifecycle:
// create → write → diff → commit → reset → write → destroy.
func TestContainer_E2E_FullLifecycle(t *testing.T) {
	requireFuse(t)
	workspace := t.TempDir()

	// Seed workspace.
	os.WriteFile(filepath.Join(workspace, "existing.txt"), []byte("original"), 0o644) //nolint:errcheck

	space, err := Create(Spec{
		Workspace: workspace,
		Backend:   Container,
		Resources: &ResourceLimits{
			Memory:    "256MB",
			CPUWeight: 1000,
			PIDMax:    100,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Phase 1: Agent writes files.
	for _, f := range []struct{ name, content string }{
		{"src/main.go", "package main"},
		{"src/lib.go", "package main"},
		{"docs/README.md", "# project"},
	} {
		full := filepath.Join(space.WorkDir(), f.name)
		os.MkdirAll(filepath.Dir(full), 0o755) //nolint:errcheck
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Diff should show 3 new files.
	changes, err := space.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("expected 3 changes, got %d: %v", len(changes), changes)
	}

	// Commit only src/.
	if err := space.Commit([]string{"src/main.go", "src/lib.go"}); err != nil {
		t.Fatal(err)
	}

	// Verify committed files exist in real workspace.
	for _, f := range []string{"src/main.go", "src/lib.go"} {
		if _, err := os.Stat(filepath.Join(workspace, f)); err != nil {
			t.Fatalf("%s should exist in workspace after commit: %v", f, err)
		}
	}
	// docs/README.md should NOT be in workspace (not committed).
	if _, err := os.Stat(filepath.Join(workspace, "docs/README.md")); err == nil {
		t.Fatal("docs/README.md should not be in workspace (not committed)")
	}

	// Phase 2: Reset and re-use (warm turnover).
	if err := space.Reset(); err != nil {
		t.Fatal(err)
	}

	changes, err = space.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected 0 changes after reset, got %d", len(changes))
	}

	// Agent 2 writes in the same Space.
	if err := os.WriteFile(filepath.Join(space.WorkDir(), "agent2.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, _ = space.Diff()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change from agent2, got %d", len(changes))
	}

	// Phase 3: Destroy.
	if err := space.Destroy(); err != nil {
		t.Fatal(err)
	}

	// Double destroy is idempotent.
	if err := space.Destroy(); err != nil {
		t.Fatalf("second destroy should be idempotent: %v", err)
	}
}

// TestContainer_E2E_RWPaths tests container backend with RWPaths filtering.
func TestContainer_E2E_RWPaths(t *testing.T) {
	requireFuse(t)
	workspace := t.TempDir()

	space, err := Create(Spec{
		Workspace: workspace,
		Backend:   Container,
		RWPaths:   []string{"src"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer space.Destroy() //nolint:errcheck

	// Write inside and outside RWPaths.
	for _, f := range []struct{ name, content string }{
		{"src/app.go", "package main"},
		{"vendor/lib.go", "package lib"},
	} {
		full := filepath.Join(space.WorkDir(), f.name)
		os.MkdirAll(filepath.Dir(full), 0o755) //nolint:errcheck
		os.WriteFile(full, []byte(f.content), 0o644) //nolint:errcheck
	}

	// Diff should only show src/.
	changes, _ := space.Diff()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change (src only), got %d: %v", len(changes), changes)
	}
	if changes[0].Path != "src/app.go" {
		t.Errorf("expected src/app.go, got %s", changes[0].Path)
	}

	// Commit src should work.
	if err := space.Commit([]string{"src/app.go"}); err != nil {
		t.Fatal(err)
	}

	// Commit vendor should fail.
	if err := space.Commit([]string{"vendor/lib.go"}); err == nil {
		t.Fatal("expected error committing outside RWPaths")
	}
}

// TestContainer_E2E_NestedSpaces tests nested container Spaces.
func TestContainer_E2E_NestedSpaces(t *testing.T) {
	requireFuse(t)
	workspace := t.TempDir()

	// Parent Space.
	parent, err := Create(Spec{Workspace: workspace, Backend: Container})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Destroy() //nolint:errcheck

	os.WriteFile(filepath.Join(parent.WorkDir(), "parent.txt"), []byte("from parent"), 0o644) //nolint:errcheck

	// Child Space over parent's WorkDir (overlay backend to avoid lock conflict).
	child, err := Create(Spec{Workspace: parent.WorkDir(), Backend: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Destroy() //nolint:errcheck

	// Child should see parent's file.
	if _, err := os.Stat(filepath.Join(child.WorkDir(), "parent.txt")); err != nil {
		t.Fatal("child should see parent.txt via read-through")
	}

	// Child writes, commits to parent.
	os.WriteFile(filepath.Join(child.WorkDir(), "child.txt"), []byte("from child"), 0o644) //nolint:errcheck
	child.Commit([]string{"child.txt"}) //nolint:errcheck
	child.Destroy() //nolint:errcheck

	// Parent should see child's committed file.
	if _, err := os.Stat(filepath.Join(parent.WorkDir(), "child.txt")); err != nil {
		t.Fatal("parent should see child.txt after child commits")
	}
}
