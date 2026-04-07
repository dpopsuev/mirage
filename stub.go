package mirage

import "sync"

// StubSpace is a pure in-memory Space mock for testing.
// No I/O. Records all calls for assertion. Returns configurable Changes from Diff().
// Used by consumers (Djinn) to unit-test Space-consuming code without real backends.
type StubSpace struct {
	mu           sync.Mutex
	workspace    string
	Changes      []Change // what Diff() returns (configurable by test)
	CommittedLog [][]string // history of Commit() calls
	DiffCalls    int
	CommitCalls  int
	ResetCalls   int
	DestroyCalls int
	destroyed    bool
}

// Compile-time interface verification.
var _ Space = (*StubSpace)(nil)

func createStub(spec Spec) *StubSpace {
	return &StubSpace{
		workspace: spec.Workspace,
	}
}

func (s *StubSpace) WorkDir() string { return s.workspace }

func (s *StubSpace) Diff() ([]Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DiffCalls++
	return s.Changes, nil
}

func (s *StubSpace) Commit(paths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CommitCalls++
	s.CommittedLog = append(s.CommittedLog, paths)
	return nil
}

func (s *StubSpace) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ResetCalls++
	s.Changes = nil
	return nil
}

func (s *StubSpace) Destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DestroyCalls++
	s.destroyed = true
	return nil
}
