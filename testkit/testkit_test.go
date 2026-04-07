package testkit_test

import (
	"testing"

	"github.com/dpopsuev/mirage"
	"github.com/dpopsuev/mirage/testkit"
)

func newStub(workspace string) *mirage.StubSpace {
	s, _ := mirage.Create(mirage.Spec{Workspace: workspace, Backend: mirage.Stub})
	return s.(*mirage.StubSpace)
}

func TestGuard_CleansUpOnPanic(t *testing.T) {
	t.Parallel()
	stub := newStub("/fake")
	g := testkit.NewGuard(t)
	g.Track(stub)

	// Cleanup registered via t.Cleanup — will run after this test returns.
	// We verify by checking DestroyCalls after.
	if stub.DestroyCalls != 0 {
		t.Fatal("expected 0 DestroyCalls before cleanup")
	}
}

func TestGuard_ReverseOrder(t *testing.T) {
	t.Parallel()
	stubs := make([]*mirage.StubSpace, 3)
	g := &testkit.Guard{}
	for i := range stubs {
		stubs[i] = newStub("/fake")
		g.Track(stubs[i])
	}

	g.Cleanup()

	for i, s := range stubs {
		if s.DestroyCalls != 1 {
			t.Errorf("stub[%d] DestroyCalls = %d, want 1", i, s.DestroyCalls)
		}
	}
}

func TestProbe_FileExists(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	stub := newStub(workspace)
	probe := testkit.NewProbe(stub)

	if probe.FileExists("nope.txt") {
		t.Fatal("expected false for missing file")
	}
}

func TestProbe_DiffPaths(t *testing.T) {
	t.Parallel()
	stub := newStub("/fake")
	stub.Changes = []mirage.Change{
		{Path: "a.go", Kind: mirage.Created},
		{Path: "b.go", Kind: mirage.Modified},
	}
	probe := testkit.NewProbe(stub)

	paths, err := probe.DiffPaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "a.go" || paths[1] != "b.go" {
		t.Fatalf("DiffPaths() = %v, want [a.go b.go]", paths)
	}
}

func TestProbe_DiffContains(t *testing.T) {
	t.Parallel()
	stub := newStub("/fake")
	stub.Changes = []mirage.Change{
		{Path: "found.go", Kind: mirage.Created},
	}
	probe := testkit.NewProbe(stub)

	found, err := probe.DiffContains("found.go")
	if err != nil || !found {
		t.Fatal("expected DiffContains to find found.go")
	}
	found, err = probe.DiffContains("missing.go")
	if err != nil || found {
		t.Fatal("expected DiffContains to not find missing.go")
	}
}

func TestProbe_IsClean(t *testing.T) {
	t.Parallel()
	stub := newStub("/fake")
	probe := testkit.NewProbe(stub)

	clean, err := probe.IsClean()
	if err != nil || !clean {
		t.Fatal("expected clean on fresh stub")
	}

	stub.Changes = []mirage.Change{{Path: "x.go", Kind: mirage.Created}}
	clean, _ = probe.IsClean()
	if clean {
		t.Fatal("expected dirty after setting Changes")
	}
}

func TestAssertClean(t *testing.T) {
	t.Parallel()
	stub := newStub("/fake")
	testkit.AssertClean(t, stub)
}

func TestAssertDirty(t *testing.T) {
	t.Parallel()
	stub := newStub("/fake")
	stub.Changes = []mirage.Change{{Path: "x.go", Kind: mirage.Created}}
	testkit.AssertDirty(t, stub)
}

func TestAssertDiffCount(t *testing.T) {
	t.Parallel()
	stub := newStub("/fake")
	stub.Changes = []mirage.Change{
		{Path: "a.go", Kind: mirage.Created},
		{Path: "b.go", Kind: mirage.Modified},
	}
	testkit.AssertDiffCount(t, stub, 2)
}

func TestAssertDiffContains(t *testing.T) {
	t.Parallel()
	stub := newStub("/fake")
	stub.Changes = []mirage.Change{{Path: "target.go", Kind: mirage.Created}}
	testkit.AssertDiffContains(t, stub, "target.go")
	testkit.AssertDiffNotContains(t, stub, "other.go")
}
