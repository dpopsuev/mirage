//go:build linux

package mirage

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// containerSpace implements Space by composing overlay + cgroup + network + lock.
// The overlay handles filesystem isolation. Cgroup handles resource limits.
// Network handles egress rules. Lock prevents concurrent access to the workspace.
type containerSpace struct {
	overlay *overlaySpace
	cgroup  *CgroupManager
	network *NetworkIsolator // shared across spaces, may be nil
	nsName  string           // network namespace name, empty if no network isolation
	lock    *LockManager
	spec    Spec

	mu        sync.Mutex
	destroyed bool
}

// createContainer creates a container-backed Space from a Spec.
// Composes overlay (filesystem) + cgroup (resources) + network (egress) + lock (workspace).
func createContainer(spec Spec) (Space, error) {
	// 1. Acquire workspace lock.
	lockDir := os.TempDir() + "/mirage-locks"
	lm := NewLockManager(lockDir)
	if _, err := lm.Acquire(spec.Workspace, "mirage-container"); err != nil {
		return nil, fmt.Errorf("mirage: container lock: %w", err)
	}

	slog.Info("mirage: creating container space",
		slog.String("workspace", spec.Workspace),
	)

	// 2. Create overlay (filesystem isolation).
	ov, err := createOverlay(spec)
	if err != nil {
		lm.Release(spec.Workspace) //nolint:errcheck // best-effort
		return nil, fmt.Errorf("mirage: container overlay: %w", err)
	}

	cs := &containerSpace{
		overlay: ov.(*overlaySpace),
		lock:    lm,
		spec:    spec,
	}

	// 3. Setup cgroup (resource limits).
	if spec.Resources != nil {
		name := fmt.Sprintf("space-%d", os.Getpid())
		mgr := NewCgroupManager(name)
		if err := mgr.Setup(*spec.Resources); err != nil {
			slog.Warn("mirage: cgroup setup failed, continuing without resource limits",
				slog.String("err", err.Error()),
			)
		} else {
			cs.cgroup = mgr
		}
	}

	// 4. Setup network isolation.
	if spec.Network != nil && spec.Network.Mode == NetIsolated {
		ni := NewNetworkIsolator("10.88.0.0/24", "mirage0", 1500)
		nsName, err := ni.Setup(name(spec), *spec.Network)
		if err != nil {
			slog.Warn("mirage: network isolation failed, continuing without",
				slog.String("err", err.Error()),
			)
		} else {
			cs.network = ni
			cs.nsName = nsName
		}
	}

	return cs, nil
}

func (s *containerSpace) WorkDir() string { return s.overlay.WorkDir() }

func (s *containerSpace) Diff() ([]Change, error) { return s.overlay.Diff() }

func (s *containerSpace) Commit(paths []string) error { return s.overlay.Commit(paths) }

// Reset resets the filesystem overlay. Cgroup and network stay intact (warm reuse).
func (s *containerSpace) Reset() error { return s.overlay.Reset() }

// Destroy tears down everything in reverse order: network → cgroup → overlay → lock.
func (s *containerSpace) Destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.destroyed {
		return nil
	}
	s.destroyed = true

	var firstErr error

	// Network teardown.
	if s.network != nil {
		if err := s.network.Teardown(name(s.spec)); err != nil {
			slog.Warn("mirage: container network teardown failed", slog.String("err", err.Error()))
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	// Cgroup cleanup.
	if s.cgroup != nil {
		if err := s.cgroup.Cleanup(); err != nil {
			slog.Warn("mirage: container cgroup cleanup failed", slog.String("err", err.Error()))
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	// Overlay teardown.
	if err := s.overlay.Destroy(); err != nil {
		slog.Warn("mirage: container overlay destroy failed", slog.String("err", err.Error()))
		if firstErr == nil {
			firstErr = err
		}
	}

	// Release workspace lock.
	if err := s.lock.Release(s.spec.Workspace); err != nil {
		slog.Warn("mirage: container lock release failed", slog.String("err", err.Error()))
		if firstErr == nil {
			firstErr = err
		}
	}

	slog.Info("mirage: container space destroyed",
		slog.String("workspace", s.spec.Workspace),
	)
	return firstErr
}

// name derives a short name from a Spec for namespace/cgroup naming.
func name(spec Spec) string {
	// Use last path component of workspace.
	w := spec.Workspace
	for len(w) > 1 && w[len(w)-1] == '/' {
		w = w[:len(w)-1]
	}
	for i := len(w) - 1; i >= 0; i-- {
		if w[i] == '/' {
			return w[i+1:]
		}
	}
	return w
}
