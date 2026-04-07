package mirage

import (
	"testing"
)

// contractSuite runs the Space contract against any backend.
// Every backend must pass these tests to prove substitutability (LSP).
// For real I/O backends (overlay, container), see integration_test.go.
func contractSuite(t *testing.T, backend Backend) {
	t.Helper()

	t.Run("CreateAndWorkDir", func(t *testing.T) {
		t.Parallel()
		workspace := t.TempDir()
		space, err := Create(Spec{Workspace: workspace, Backend: backend})
		if err != nil {
			t.Fatal(err)
		}
		defer space.Destroy() //nolint:errcheck

		wd := space.WorkDir()
		if wd == "" {
			t.Fatal("WorkDir() returned empty string")
		}
	})

	t.Run("DiffEmpty", func(t *testing.T) {
		t.Parallel()
		workspace := t.TempDir()
		space, err := Create(Spec{Workspace: workspace, Backend: backend})
		if err != nil {
			t.Fatal(err)
		}
		defer space.Destroy() //nolint:errcheck

		changes, err := space.Diff()
		if err != nil {
			t.Fatal(err)
		}
		if len(changes) != 0 {
			t.Fatalf("Diff() on fresh space should be empty, got %d changes", len(changes))
		}
	})

	t.Run("ResetClearsState", func(t *testing.T) {
		t.Parallel()
		workspace := t.TempDir()
		space, err := Create(Spec{Workspace: workspace, Backend: backend})
		if err != nil {
			t.Fatal(err)
		}
		defer space.Destroy() //nolint:errcheck

		if err := space.Reset(); err != nil {
			t.Fatal(err)
		}

		changes, err := space.Diff()
		if err != nil {
			t.Fatal(err)
		}
		if len(changes) != 0 {
			t.Fatalf("Diff() after Reset should be empty, got %d changes", len(changes))
		}
	})

	t.Run("DestroyIdempotent", func(t *testing.T) {
		t.Parallel()
		workspace := t.TempDir()
		space, err := Create(Spec{Workspace: workspace, Backend: backend})
		if err != nil {
			t.Fatal(err)
		}

		if err := space.Destroy(); err != nil {
			t.Fatal(err)
		}
		// Second destroy should not error.
		if err := space.Destroy(); err != nil {
			t.Fatalf("second Destroy() should be idempotent, got %v", err)
		}
	})
}

// TestStub_Contract verifies the stub backend passes the Space contract.
func TestStub_Contract(t *testing.T) {
	contractSuite(t, Stub)
}

// TestStub_CallRecording verifies that StubSpace records all method calls.
func TestStub_CallRecording(t *testing.T) {
	t.Parallel()
	stub := createStub(Spec{Workspace: "/fake"})

	stub.Diff()                                 //nolint:errcheck
	stub.Diff()                                 //nolint:errcheck
	stub.Commit([]string{"a.txt"})              //nolint:errcheck
	stub.Commit([]string{"b.txt", "c.txt"})     //nolint:errcheck
	stub.Reset()                                //nolint:errcheck
	stub.Destroy()                              //nolint:errcheck

	if stub.DiffCalls != 2 {
		t.Errorf("DiffCalls = %d, want 2", stub.DiffCalls)
	}
	if stub.CommitCalls != 2 {
		t.Errorf("CommitCalls = %d, want 2", stub.CommitCalls)
	}
	if stub.ResetCalls != 1 {
		t.Errorf("ResetCalls = %d, want 1", stub.ResetCalls)
	}
	if stub.DestroyCalls != 1 {
		t.Errorf("DestroyCalls = %d, want 1", stub.DestroyCalls)
	}

	// CommittedLog records what was passed to each Commit call.
	if len(stub.CommittedLog) != 2 {
		t.Fatalf("CommittedLog = %d entries, want 2", len(stub.CommittedLog))
	}
	if len(stub.CommittedLog[0]) != 1 || stub.CommittedLog[0][0] != "a.txt" {
		t.Errorf("CommittedLog[0] = %v, want [a.txt]", stub.CommittedLog[0])
	}
	if len(stub.CommittedLog[1]) != 2 {
		t.Errorf("CommittedLog[1] = %v, want [b.txt c.txt]", stub.CommittedLog[1])
	}
}

// TestStub_ConfigurableChanges verifies that test code can set Changes
// and Diff() returns them. Reset clears them.
func TestStub_ConfigurableChanges(t *testing.T) {
	t.Parallel()
	stub := createStub(Spec{Workspace: "/fake"})

	stub.Changes = []Change{
		{Path: "foo.go", Kind: Modified, Size: 100},
		{Path: "bar.go", Kind: Created, Size: 200},
	}

	changes, err := stub.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("Diff() returned %d changes, want 2", len(changes))
	}

	// Reset clears configurable changes.
	stub.Reset() //nolint:errcheck
	changes, _ = stub.Diff()
	if len(changes) != 0 {
		t.Fatalf("Diff() after Reset should be empty, got %d", len(changes))
	}
}

// TestStub_WorkDirIsWorkspace verifies that the stub's WorkDir is the
// workspace path itself (no temp dirs, no I/O).
func TestStub_WorkDirIsWorkspace(t *testing.T) {
	t.Parallel()
	stub := createStub(Spec{Workspace: "/my/project"})

	if stub.WorkDir() != "/my/project" {
		t.Errorf("WorkDir() = %q, want /my/project", stub.WorkDir())
	}
}

// TestValidation verifies Spec validation.
func TestValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    Spec
		wantErr error
	}{
		{"empty workspace", Spec{Backend: Stub}, ErrWorkspaceRequired},
		{"empty backend", Spec{Workspace: "/tmp"}, ErrBackendRequired},
		{"unknown backend", Spec{Workspace: "/tmp", Backend: "magic"}, ErrUnknownBackend},
		{"valid stub", Spec{Workspace: "/tmp", Backend: Stub}, nil},
		{"valid overlay", Spec{Workspace: "/tmp", Backend: Overlay}, nil},
		{"valid container", Spec{Workspace: "/tmp", Backend: Container}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
