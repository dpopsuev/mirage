//go:build linux

// overlay.go — fuse-overlayfs implementation of Space.
//
// Adapted from:
//   - misbah/daemon/overlay.go (diff/commit logic)
//   - djinn/sandbox/namespace/overlay.go (fuse mount/unmount)
package mirage

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Sentinel errors.
var (
	ErrFuseNotAvailable = errors.New("mirage: fuse-overlayfs not available")
	ErrNotDirectory     = errors.New("mirage: not a directory")
	ErrNotMounted       = errors.New("mirage: overlay not mounted")
	ErrUnmountFailed    = errors.New("mirage: unmount failed")
)

// OverlayBuilder creates fuse-overlayfs backed Spaces.
type OverlayBuilder struct{}

// NewOverlayBuilder creates a builder that uses fuse-overlayfs.
// Requires fuse-overlayfs binary on PATH. No root required.
func NewOverlayBuilder() *OverlayBuilder {
	return &OverlayBuilder{}
}

// createOverlay creates an overlay Space from a Spec.
func createOverlay(spec Spec) (Space, error) {
	b := NewOverlayBuilder()
	s, err := b.Create(spec.Workspace)
	if err != nil {
		return nil, err
	}
	if len(spec.RWPaths) > 0 {
		s.(*overlaySpace).rwPaths = spec.RWPaths
	}
	return s, nil
}

// Create mounts a fuse-overlayfs overlay over the workspace.
// The agent sees the merged view; writes go to an upper layer.
// The real workspace is never modified until Commit().
func (b *OverlayBuilder) Create(workspace string) (Space, error) {
	info, err := os.Stat(workspace)
	if err != nil {
		return nil, fmt.Errorf("mirage: workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrNotDirectory, workspace)
	}

	if _, err := exec.LookPath("fuse-overlayfs"); err != nil {
		return nil, ErrFuseNotAvailable
	}

	tempDir, err := os.MkdirTemp("", "mirage-*")
	if err != nil {
		return nil, fmt.Errorf("mirage: temp dir: %w", err)
	}

	upper := filepath.Join(tempDir, "upper")
	work := filepath.Join(tempDir, "work")
	merged := filepath.Join(tempDir, "merged")

	for _, d := range []string{upper, work, merged} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			os.RemoveAll(tempDir) //nolint:errcheck // best-effort cleanup
			return nil, fmt.Errorf("mirage: mkdir %s: %w", d, err)
		}
	}

	cmd := exec.Command("fuse-overlayfs", //nolint:gosec // paths are controlled
		"-o", fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", workspace, upper, work),
		merged)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tempDir) //nolint:errcheck // best-effort cleanup
		return nil, fmt.Errorf("mirage: fuse-overlayfs: %s: %w", out, err)
	}

	return &overlaySpace{
		lower:   workspace,
		upper:   upper,
		work:    work,
		merged:  merged,
		tempDir: tempDir,
		mounted: true,
	}, nil
}

// overlaySpace implements Space using fuse-overlayfs.
type overlaySpace struct {
	lower   string // real workspace (read-only in overlay)
	upper   string // writable layer (agent's changes)
	work    string // overlayfs scratch
	merged  string // what the agent sees
	tempDir string
	rwPaths []string // if set, only these paths are visible in Diff/Commit

	mu      sync.Mutex
	mounted bool
}

func (s *overlaySpace) WorkDir() string { return s.merged }

func (s *overlaySpace) Diff() ([]Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.mounted {
		return nil, ErrNotMounted
	}

	var changes []Change
	err := filepath.Walk(s.upper, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == s.upper || info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(s.upper, path)

		// Skip overlayfs whiteout/opaque files.
		base := filepath.Base(rel)
		if strings.HasPrefix(base, ".wh.") {
			return nil
		}

		kind := Created
		if _, statErr := os.Stat(filepath.Join(s.lower, rel)); statErr == nil {
			kind = Modified
		}

		if !s.isWritable(rel) {
			return nil // outside RWPaths, skip
		}

		changes = append(changes, Change{
			Path: rel,
			Kind: kind,
			Size: info.Size(),
		})
		return nil
	})
	return changes, err
}

func (s *overlaySpace) Commit(paths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.mounted {
		return ErrNotMounted
	}

	for _, p := range paths {
		if !s.isWritable(p) {
			return fmt.Errorf("mirage: commit %s: path not in RWPaths", p)
		}
		src := filepath.Join(s.upper, p)
		dst := filepath.Join(s.lower, p)

		data, err := os.ReadFile(src) //nolint:gosec // path from controlled overlay
		if err != nil {
			return fmt.Errorf("mirage: commit read %s: %w", p, err)
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mirage: commit mkdir: %w", err)
		}

		info, _ := os.Stat(src) //nolint:gosec // path from controlled overlay
		if err := os.WriteFile(dst, data, info.Mode()); err != nil {
			return fmt.Errorf("mirage: commit write %s: %w", p, err)
		}
	}
	return nil
}

func (s *overlaySpace) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.mounted {
		return ErrNotMounted
	}

	// Clear contents of upper without removing the directory itself.
	// Removing+recreating the dir confuses the FUSE daemon's cached FD.
	entries, err := os.ReadDir(s.upper)
	if err != nil {
		return fmt.Errorf("mirage: reset read upper: %w", err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(s.upper, e.Name())); err != nil {
			return fmt.Errorf("mirage: reset remove %s: %w", e.Name(), err)
		}
	}
	return nil
}

func (s *overlaySpace) Destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.mounted {
		return nil
	}
	s.mounted = false

	// Unmount fuse-overlayfs.
	if out, err := exec.Command("fusermount3", "-u", s.merged).CombinedOutput(); err != nil { //nolint:gosec // path controlled
		if out2, err2 := exec.Command("fusermount", "-u", s.merged).CombinedOutput(); err2 != nil { //nolint:gosec // path controlled
			return fmt.Errorf("%w: %s / %s", ErrUnmountFailed, out, out2)
		}
	}

	return os.RemoveAll(s.tempDir)
}

// isWritable checks if a relative path falls under the configured RWPaths.
// If RWPaths is empty, all paths are writable (default).
func (s *overlaySpace) isWritable(rel string) bool {
	if len(s.rwPaths) == 0 {
		return true // no filter = all writable
	}
	for _, rw := range s.rwPaths {
		if rel == rw || strings.HasPrefix(rel, rw+"/") {
			return true
		}
	}
	return false
}
