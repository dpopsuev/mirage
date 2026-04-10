package mirage

import (
	"fmt"
	"sync"
)

// StubSpace is a pure in-memory Space mock for testing.
// No I/O. Records all calls for assertion. Returns configurable Changes from Diff().
// Used by consumers (Djinn) to unit-test Space-consuming code without real backends.
type StubSpace struct {
	mu            sync.Mutex
	workspace     string
	Changes       []Change            // what Diff() returns (configurable by test)
	CommittedLog  [][]string          // history of Commit() calls
	snapshots     map[string][]Change // saved snapshots
	DiffCalls     int
	CommitCalls   int
	ResetCalls    int
	SnapshotCalls int
	RestoreCalls  int
	DestroyCalls  int
	destroyed     bool
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

func (s *StubSpace) Snapshot(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SnapshotCalls++
	if s.snapshots == nil {
		s.snapshots = make(map[string][]Change)
	}
	saved := make([]Change, len(s.Changes))
	copy(saved, s.Changes)
	s.snapshots[name] = saved
	return nil
}

func (s *StubSpace) Restore(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RestoreCalls++
	if s.snapshots == nil {
		return fmt.Errorf("%w: %s", ErrSnapshotNotFound, name)
	}
	saved, ok := s.snapshots[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrSnapshotNotFound, name)
	}
	s.Changes = make([]Change, len(saved))
	copy(s.Changes, saved)
	return nil
}

func (s *StubSpace) Snapshots() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.snapshots))
	for name := range s.snapshots {
		names = append(names, name)
	}
	return names
}

func (s *StubSpace) Destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DestroyCalls++
	s.destroyed = true
	return nil
}
