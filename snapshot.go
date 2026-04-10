//go:build linux

// snapshot.go — named snapshot support for overlay and container backends.
package mirage

import (
	"fmt"
	"os"
	"path/filepath"
)

// --- overlaySpace: real Snapshot/Restore ---

func (s *overlaySpace) Snapshot(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.mounted {
		return ErrNotMounted
	}

	snapDir := filepath.Join(s.tempDir, "snapshots", name)
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return fmt.Errorf("%w: mkdir: %w", ErrSnapshotNotImpl, err)
	}

	return copyDir(s.upper, snapDir)
}

func (s *overlaySpace) Restore(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.mounted {
		return ErrNotMounted
	}

	snapDir := filepath.Join(s.tempDir, "snapshots", name)
	if _, err := os.Stat(snapDir); err != nil {
		return fmt.Errorf("%w: %s", ErrSnapshotNotFound, name)
	}

	// Clear upper
	entries, err := os.ReadDir(s.upper)
	if err != nil {
		return fmt.Errorf("mirage: restore read upper: %w", err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(s.upper, e.Name())); err != nil {
			return fmt.Errorf("mirage: restore clear %s: %w", e.Name(), err)
		}
	}

	return copyDir(snapDir, s.upper)
}

func (s *overlaySpace) Snapshots() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapBase := filepath.Join(s.tempDir, "snapshots")
	entries, err := os.ReadDir(snapBase)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

// --- containerSpace: not supported ---

func (s *containerSpace) Snapshot(_ string) error {
	return ErrSnapshotNotImpl
}

func (s *containerSpace) Restore(_ string) error {
	return ErrSnapshotNotImpl
}

func (s *containerSpace) Snapshots() []string {
	return nil
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		data, err := os.ReadFile(path) //nolint:gosec // controlled overlay path
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
