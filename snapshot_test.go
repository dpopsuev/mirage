package mirage_test

import (
	"testing"

	"github.com/dpopsuev/mirage"
)

func TestStubSpace_Snapshot_Create(t *testing.T) {
	s := &mirage.StubSpace{}
	s.Changes = []mirage.Change{
		{Path: "main.go", Kind: mirage.Created, Size: 100},
	}

	if err := s.Snapshot("cp1"); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if s.SnapshotCalls != 1 {
		t.Fatalf("SnapshotCalls = %d", s.SnapshotCalls)
	}
}

func TestStubSpace_Restore_ToSnapshot(t *testing.T) {
	s := &mirage.StubSpace{}
	s.Changes = []mirage.Change{
		{Path: "main.go", Kind: mirage.Created},
	}
	s.Snapshot("before") //nolint:errcheck // test

	// Simulate more changes
	s.Changes = append(s.Changes, mirage.Change{Path: "bad.go", Kind: mirage.Created})

	if err := s.Restore("before"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Changes should be back to snapshot state
	if len(s.Changes) != 1 {
		t.Fatalf("Changes = %d, want 1 (restored to snapshot)", len(s.Changes))
	}
	if s.Changes[0].Path != "main.go" {
		t.Fatalf("Changes[0] = %q, want main.go", s.Changes[0].Path)
	}
}

func TestStubSpace_Restore_NotFound(t *testing.T) {
	s := &mirage.StubSpace{}
	err := s.Restore("nonexistent")
	if err == nil {
		t.Fatal("Restore should fail for unknown snapshot")
	}
}

func TestStubSpace_Snapshots_ListNames(t *testing.T) {
	s := &mirage.StubSpace{}
	s.Snapshot("alpha") //nolint:errcheck // test
	s.Snapshot("beta")  //nolint:errcheck // test

	names := s.Snapshots()
	if len(names) != 2 {
		t.Fatalf("Snapshots = %d, want 2", len(names))
	}
}

func TestStubSpace_Snapshot_PreservesIndependently(t *testing.T) {
	s := &mirage.StubSpace{}
	s.Changes = []mirage.Change{{Path: "a.go", Kind: mirage.Created}}
	s.Snapshot("v1") //nolint:errcheck // test

	s.Changes = []mirage.Change{{Path: "b.go", Kind: mirage.Created}}
	s.Snapshot("v2") //nolint:errcheck // test

	// Restore v1
	s.Restore("v1") //nolint:errcheck // test
	if len(s.Changes) != 1 || s.Changes[0].Path != "a.go" {
		t.Fatalf("after restore v1: %v", s.Changes)
	}

	// Restore v2
	s.Restore("v2") //nolint:errcheck // test
	if len(s.Changes) != 1 || s.Changes[0].Path != "b.go" {
		t.Fatalf("after restore v2: %v", s.Changes)
	}
}
