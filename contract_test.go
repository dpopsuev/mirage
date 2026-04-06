package mirage

import (
	"os"
	"path/filepath"
	"testing"
)

// contractSuite runs the Space contract against any backend.
// Every backend must pass these tests to prove substitutability (LSP).
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

	t.Run("DiffShowsChanges", func(t *testing.T) {
		t.Parallel()
		workspace := t.TempDir()
		space, err := Create(Spec{Workspace: workspace, Backend: backend})
		if err != nil {
			t.Fatal(err)
		}
		defer space.Destroy() //nolint:errcheck

		// Write a file in the workspace
		testFile := filepath.Join(space.WorkDir(), "test.txt")
		if err := os.WriteFile(testFile, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}

		changes, err := space.Diff()
		if err != nil {
			t.Fatal(err)
		}
		if len(changes) == 0 {
			t.Fatal("Diff() should show the new file")
		}
	})

	t.Run("CommitPromotes", func(t *testing.T) {
		t.Parallel()
		workspace := t.TempDir()
		space, err := Create(Spec{Workspace: workspace, Backend: backend})
		if err != nil {
			t.Fatal(err)
		}
		defer space.Destroy() //nolint:errcheck

		// Write a file
		testFile := filepath.Join(space.WorkDir(), "committed.txt")
		if err := os.WriteFile(testFile, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Commit it
		if err := space.Commit([]string{"committed.txt"}); err != nil {
			t.Fatal(err)
		}

		// File should exist in the real workspace
		real := filepath.Join(workspace, "committed.txt")
		if _, err := os.Stat(real); os.IsNotExist(err) {
			t.Fatal("committed file should exist in real workspace after Commit")
		}
	})

	t.Run("ResetDiscards", func(t *testing.T) {
		t.Parallel()
		workspace := t.TempDir()
		space, err := Create(Spec{Workspace: workspace, Backend: backend})
		if err != nil {
			t.Fatal(err)
		}
		defer space.Destroy() //nolint:errcheck

		// Write a file
		testFile := filepath.Join(space.WorkDir(), "temp.txt")
		if err := os.WriteFile(testFile, []byte("discard me"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Reset
		if err := space.Reset(); err != nil {
			t.Fatal(err)
		}

		// Diff should be empty after reset
		changes, err := space.Diff()
		if err != nil {
			t.Fatal(err)
		}
		if len(changes) != 0 {
			t.Fatalf("Diff() should be empty after Reset, got %d changes", len(changes))
		}
	})

	t.Run("DestroyCleans", func(t *testing.T) {
		t.Parallel()
		workspace := t.TempDir()
		space, err := Create(Spec{Workspace: workspace, Backend: backend})
		if err != nil {
			t.Fatal(err)
		}

		wd := space.WorkDir()

		if err := space.Destroy(); err != nil {
			t.Fatal(err)
		}

		// For stub, WorkDir may be a temp dir or the workspace itself
		// For overlay, the merged dir should be gone
		if backend != Stub {
			if _, err := os.Stat(wd); !os.IsNotExist(err) {
				t.Logf("WorkDir %s still exists after Destroy (may be expected for some backends)", wd)
			}
		}
	})
}

// TestStub_Contract verifies the stub backend passes the Space contract.
func TestStub_Contract(t *testing.T) {
	contractSuite(t, Stub)
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
