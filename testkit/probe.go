package testkit

import (
	"os"
	"path/filepath"
	"time"

	"github.com/dpopsuev/mirage"
)

// Probe provides non-invasive state inspection for a Space.
// Wraps a Space to poll and query without modifying state.
type Probe struct {
	space mirage.Space
}

// NewProbe creates a Probe for the given Space.
func NewProbe(s mirage.Space) *Probe {
	return &Probe{space: s}
}

// FileExists checks if a file exists in the Space's WorkDir.
func (p *Probe) FileExists(relPath string) bool {
	_, err := os.Stat(filepath.Join(p.space.WorkDir(), relPath))
	return err == nil
}

// ReadFile reads a file from the Space's WorkDir.
func (p *Probe) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(p.space.WorkDir(), relPath)) //nolint:gosec // test helper
}

// DiffPaths returns just the paths from Diff (convenience).
func (p *Probe) DiffPaths() ([]string, error) {
	changes, err := p.space.Diff()
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(changes))
	for i, c := range changes {
		paths[i] = c.Path
	}
	return paths, nil
}

// DiffContains checks if a specific path appears in Diff results.
func (p *Probe) DiffContains(relPath string) (bool, error) {
	changes, err := p.space.Diff()
	if err != nil {
		return false, err
	}
	for _, c := range changes {
		if c.Path == relPath {
			return true, nil
		}
	}
	return false, nil
}

// WaitForFile polls until a file appears in the Space's WorkDir or timeout.
func (p *Probe) WaitForFile(relPath string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if p.FileExists(relPath) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// WaitForDiff polls until Diff returns at least one change or timeout.
func (p *Probe) WaitForDiff(timeout time.Duration) ([]mirage.Change, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		changes, err := p.space.Diff()
		if err == nil && len(changes) > 0 {
			return changes, true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, false
}

// IsClean returns true if Diff reports zero changes.
func (p *Probe) IsClean() (bool, error) {
	changes, err := p.space.Diff()
	if err != nil {
		return false, err
	}
	return len(changes) == 0, nil
}
