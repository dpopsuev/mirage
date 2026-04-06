package mirage

import (
	"os"
	"path/filepath"
	"sync"
)

// StubSpace is an in-memory Space for testing. No I/O beyond a temp directory.
// Records all calls for assertion. Configurable Changes for Diff().
type StubSpace struct {
	mu           sync.Mutex
	workDir      string
	workspace    string
	DiffCalls    int
	CommitCalls  int
	ResetCalls   int
	DestroyCalls int
	Changes      []Change // what Diff() returns (configurable)
	destroyed    bool
}

// Compile-time interface verification.
var _ Space = (*StubSpace)(nil)

func createStub(spec Spec) *StubSpace {
	// Create a real temp directory so file writes work in contract tests.
	tmpDir, _ := os.MkdirTemp("", "mirage-stub-*")
	return &StubSpace{
		workDir:   tmpDir,
		workspace: spec.Workspace,
	}
}

func (s *StubSpace) WorkDir() string { return s.workDir }

func (s *StubSpace) Diff() ([]Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DiffCalls++

	// If Changes are pre-configured, return those.
	if len(s.Changes) > 0 {
		return s.Changes, nil
	}

	// Otherwise, scan the temp dir for real files (supports contract tests).
	var changes []Change
	filepath.Walk(s.workDir, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil || info.IsDir() || path == s.workDir {
			return nil
		}
		rel, _ := filepath.Rel(s.workDir, path)
		changes = append(changes, Change{
			Path: rel,
			Kind: Created,
			Size: info.Size(),
		})
		return nil
	})
	return changes, nil
}

func (s *StubSpace) Commit(paths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CommitCalls++

	// Copy files from temp dir to real workspace.
	for _, p := range paths {
		src := filepath.Join(s.workDir, p)
		dst := filepath.Join(s.workspace, p)
		data, err := os.ReadFile(src) //nolint:gosec
		if err != nil {
			continue
		}
		os.MkdirAll(filepath.Dir(dst), 0o755) //nolint:errcheck
		os.WriteFile(dst, data, 0o644)        //nolint:errcheck,gosec
	}
	return nil
}

func (s *StubSpace) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ResetCalls++

	// Remove all files from temp dir.
	entries, _ := os.ReadDir(s.workDir)
	for _, e := range entries {
		os.RemoveAll(filepath.Join(s.workDir, e.Name())) //nolint:errcheck
	}
	return nil
}

func (s *StubSpace) Destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DestroyCalls++

	if s.destroyed {
		return nil
	}
	s.destroyed = true
	return os.RemoveAll(s.workDir)
}
