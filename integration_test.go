//go:build integration

package mirage

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOverlay_Contract runs the 5-verb contract against real fuse-overlayfs.
func TestOverlay_Contract(t *testing.T) {
	requireFuse(t)
	contractSuite(t, Overlay)
}

// TestOverlay_NestedSpaces verifies that overlay Spaces can stack — a child
// Space uses a parent Space's WorkDir as its workspace. Changes bubble up
// through Commit at each level. Requires real fuse-overlayfs.
func TestOverlay_NestedSpaces(t *testing.T) {
	requireFuse(t)
	t.Parallel()
	workspace := t.TempDir()

	// Seed the workspace with a file so read-through is testable.
	if err := os.WriteFile(filepath.Join(workspace, "original.txt"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Level 0: Agent A gets a Space over the real workspace.
	spaceA, err := Create(Spec{Workspace: workspace, Backend: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	defer spaceA.Destroy() //nolint:errcheck

	// A sees the original file via read-through.
	if _, err := os.Stat(filepath.Join(spaceA.WorkDir(), "original.txt")); err != nil {
		t.Fatal("A should see original.txt via read-through")
	}

	// Agent A writes a file.
	if err := os.WriteFile(filepath.Join(spaceA.WorkDir(), "a.txt"), []byte("from A"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Level 1: Agent A summons Agent B. B's workspace = A's WorkDir.
	spaceB, err := Create(Spec{Workspace: spaceA.WorkDir(), Backend: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	defer spaceB.Destroy() //nolint:errcheck

	// B should see original.txt AND A's file via read-through.
	for _, name := range []string{"original.txt", "a.txt"} {
		if _, err := os.Stat(filepath.Join(spaceB.WorkDir(), name)); err != nil {
			t.Fatalf("Agent B should see %s via read-through", name)
		}
	}

	// Agent B writes its own file.
	if err := os.WriteFile(filepath.Join(spaceB.WorkDir(), "b.txt"), []byte("from B"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Level 2: B summons C. C's workspace = B's WorkDir.
	spaceC, err := Create(Spec{Workspace: spaceB.WorkDir(), Backend: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	defer spaceC.Destroy() //nolint:errcheck

	// C should see original, A's, and B's files.
	for _, name := range []string{"original.txt", "a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(spaceC.WorkDir(), name)); err != nil {
			t.Fatalf("Agent C should see %s via read-through", name)
		}
	}

	// Agent C writes its own file.
	if err := os.WriteFile(filepath.Join(spaceC.WorkDir(), "c.txt"), []byte("from C"), 0o644); err != nil {
		t.Fatal(err)
	}

	// C's diff should show only c.txt (its own work).
	changesC, err := spaceC.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changesC) != 1 || changesC[0].Path != "c.txt" {
		t.Fatalf("C's Diff should show only c.txt, got %v", changesC)
	}

	// Commit C → B's merged view.
	if err := spaceC.Commit([]string{"c.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := spaceC.Destroy(); err != nil {
		t.Fatal(err)
	}

	// B should now see c.txt.
	if _, err := os.Stat(filepath.Join(spaceB.WorkDir(), "c.txt")); err != nil {
		t.Fatal("c.txt should be visible in B's WorkDir after C commits")
	}

	// Commit B → A's merged view.
	if err := spaceB.Commit([]string{"b.txt", "c.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := spaceB.Destroy(); err != nil {
		t.Fatal(err)
	}

	// A should now see b.txt and c.txt.
	for _, name := range []string{"b.txt", "c.txt"} {
		if _, err := os.Stat(filepath.Join(spaceA.WorkDir(), name)); err != nil {
			t.Fatalf("%s should be visible in A's WorkDir after B commits", name)
		}
	}

	// Commit A → real workspace.
	if err := spaceA.Commit([]string{"a.txt", "b.txt", "c.txt"}); err != nil {
		t.Fatal(err)
	}

	// All files should exist in the real workspace.
	for _, name := range []string{"original.txt", "a.txt", "b.txt", "c.txt"} {
		if _, err := os.Stat(filepath.Join(workspace, name)); err != nil {
			t.Fatalf("%s should exist in real workspace after full commit chain", name)
		}
	}
}

// TestOverlay_SiblingSpaces verifies that sibling Spaces over the same workspace
// are isolated — concurrent agents don't see each other's uncommitted work.
func TestOverlay_SiblingSpaces(t *testing.T) {
	requireFuse(t)
	t.Parallel()
	workspace := t.TempDir()

	spaceA, err := Create(Spec{Workspace: workspace, Backend: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	defer spaceA.Destroy() //nolint:errcheck

	spaceB, err := Create(Spec{Workspace: workspace, Backend: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	defer spaceB.Destroy() //nolint:errcheck

	// A writes a file.
	if err := os.WriteFile(filepath.Join(spaceA.WorkDir(), "a.txt"), []byte("from A"), 0o644); err != nil {
		t.Fatal(err)
	}

	// B writes a different file.
	if err := os.WriteFile(filepath.Join(spaceB.WorkDir(), "b.txt"), []byte("from B"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A should NOT see B's file, and vice versa.
	if _, err := os.Stat(filepath.Join(spaceA.WorkDir(), "b.txt")); err == nil {
		t.Fatal("A should not see B's uncommitted file")
	}
	if _, err := os.Stat(filepath.Join(spaceB.WorkDir(), "a.txt")); err == nil {
		t.Fatal("B should not see A's uncommitted file")
	}

	// Commit both.
	if err := spaceA.Commit([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := spaceB.Commit([]string{"b.txt"}); err != nil {
		t.Fatal(err)
	}

	// Both files in the real workspace.
	for _, name := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(workspace, name)); err != nil {
			t.Fatalf("%s should exist in workspace after commit", name)
		}
	}
}

// TestOverlay_ResetKeepsSpace verifies that Reset wipes changes but the Space
// stays usable — the "keep warm" pattern for reusing Spaces across agents.
func TestOverlay_ResetKeepsSpace(t *testing.T) {
	requireFuse(t)
	t.Parallel()
	workspace := t.TempDir()

	space, err := Create(Spec{Workspace: workspace, Backend: Overlay})
	if err != nil {
		t.Fatal(err)
	}
	defer space.Destroy() //nolint:errcheck

	// Agent 1 writes a file.
	if err := os.WriteFile(filepath.Join(space.WorkDir(), "agent1.txt"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, _ := space.Diff()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	// Reset — turnover for next agent.
	if err := space.Reset(); err != nil {
		t.Fatal(err)
	}

	// Space is still usable, diff is clean.
	changes, _ = space.Diff()
	if len(changes) != 0 {
		t.Fatalf("expected 0 changes after reset, got %d", len(changes))
	}

	// Agent 2 can work in the same Space.
	if err := os.WriteFile(filepath.Join(space.WorkDir(), "agent2.txt"), []byte("more work"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, _ = space.Diff()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change from agent2, got %d", len(changes))
	}
}

// TestContainer_Contract runs the 5-verb contract against the container backend.
// Requires real fuse-overlayfs (container backend composes overlay internally).
func TestContainer_Contract(t *testing.T) {
	requireFuse(t)
	contractSuite(t, Container)
}

// TestContainer_ResetKeepsResources verifies that Reset only resets the filesystem.
// Cgroup and network stay intact — the "warm Space" pattern.
func TestContainer_ResetKeepsResources(t *testing.T) {
	requireFuse(t)
	t.Parallel()
	workspace := t.TempDir()

	space, err := Create(Spec{Workspace: workspace, Backend: Container})
	if err != nil {
		t.Fatal(err)
	}
	defer space.Destroy() //nolint:errcheck

	// Write, reset, write again — Space stays usable.
	if err := os.WriteFile(filepath.Join(space.WorkDir(), "before.txt"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := space.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(space.WorkDir(), "after.txt"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes, err := space.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != "after.txt" {
		t.Fatalf("expected only after.txt, got %v", changes)
	}
}

// TestOverlay_RWPaths verifies that Diff only shows changes under RWPaths.
func TestOverlay_RWPaths(t *testing.T) {
	requireFuse(t)
	t.Parallel()
	workspace := t.TempDir()

	space, err := Create(Spec{
		Workspace: workspace,
		Backend:   Overlay,
		RWPaths:   []string{"src", "docs"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer space.Destroy() //nolint:errcheck

	// Write files in and out of RWPaths.
	for _, f := range []struct{ path, content string }{
		{"src/main.go", "package main"},
		{"docs/README.md", "# readme"},
		{"vendor/lib.go", "package lib"}, // outside RWPaths
		{"config.yaml", "key: val"},      // outside RWPaths
	} {
		full := filepath.Join(space.WorkDir(), f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Diff should only show src/ and docs/ files.
	changes, err := space.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes (src + docs), got %d: %v", len(changes), changes)
	}
	paths := map[string]bool{}
	for _, c := range changes {
		paths[c.Path] = true
	}
	if !paths["src/main.go"] {
		t.Error("expected src/main.go in Diff")
	}
	if !paths["docs/README.md"] {
		t.Error("expected docs/README.md in Diff")
	}
}

// TestOverlay_RWPaths_CommitBlocked verifies that Commit rejects paths outside RWPaths.
func TestOverlay_RWPaths_CommitBlocked(t *testing.T) {
	requireFuse(t)
	t.Parallel()
	workspace := t.TempDir()

	space, err := Create(Spec{
		Workspace: workspace,
		Backend:   Overlay,
		RWPaths:   []string{"src"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer space.Destroy() //nolint:errcheck

	// Write a file outside RWPaths.
	full := filepath.Join(space.WorkDir(), "outside.txt")
	if err := os.WriteFile(full, []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Commit should fail for paths outside RWPaths.
	err = space.Commit([]string{"outside.txt"})
	if err == nil {
		t.Fatal("expected error committing outside RWPaths")
	}
}

// TestOverlay_RWPaths_Empty verifies that empty RWPaths means all writable (default).
func TestOverlay_RWPaths_Empty(t *testing.T) {
	requireFuse(t)
	t.Parallel()
	workspace := t.TempDir()

	space, err := Create(Spec{
		Workspace: workspace,
		Backend:   Overlay,
		// No RWPaths — default: all writable
	})
	if err != nil {
		t.Fatal(err)
	}
	defer space.Destroy() //nolint:errcheck

	if err := os.WriteFile(filepath.Join(space.WorkDir(), "anywhere.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes, err := space.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
}

// requireFuse is defined in testhelper_linux_test.go (shared across build tags).
