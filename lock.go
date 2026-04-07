//go:build linux

package mirage

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Sentinel errors for lock operations.
var (
	ErrWorkspaceLocked = errors.New("mirage: workspace is locked")
)

// Lock represents a workspace lock file.
type Lock struct {
	Workspace string    `json:"workspace"`
	Provider  string    `json:"provider"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	User      string    `json:"user"`
}

// IsStale checks if the locking process is still alive.
// Uses kill(pid, 0) — no signal sent, just existence check.
func (l *Lock) IsStale() bool {
	return syscall.Kill(l.PID, syscall.Signal(0)) != nil
}

// LockManager manages workspace locks via JSON files.
type LockManager struct {
	locksDir string
}

// NewLockManager creates a lock manager that stores lock files in dir.
func NewLockManager(dir string) *LockManager {
	return &LockManager{locksDir: dir}
}

// Acquire acquires a lock for a workspace. Returns ErrWorkspaceLocked if already locked.
func (lm *LockManager) Acquire(workspace, provider string) (*Lock, error) {
	if err := os.MkdirAll(lm.locksDir, 0o755); err != nil {
		return nil, fmt.Errorf("mirage: lock mkdir: %w", err)
	}

	lockPath := lm.path(workspace)

	// Check for existing lock.
	if existing, err := lm.read(lockPath); err == nil {
		if existing.IsStale() {
			slog.Warn("mirage: removing stale lock",
				slog.String("workspace", workspace),
				slog.Int("pid", existing.PID),
			)
			if err := os.Remove(lockPath); err != nil {
				return nil, fmt.Errorf("mirage: remove stale lock: %w", err)
			}
		} else {
			return nil, fmt.Errorf("%w: %s locked by PID %d (user: %s, since: %s)",
				ErrWorkspaceLocked, workspace, existing.PID, existing.User, existing.StartedAt.Format(time.RFC3339))
		}
	}

	username := os.Getenv("USER")
	if username == "" {
		username = "unknown"
	}

	lock := &Lock{
		Workspace: workspace,
		Provider:  provider,
		PID:       os.Getpid(),
		StartedAt: time.Now(),
		User:      username,
	}

	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("mirage: lock marshal: %w", err)
	}

	// Atomic write: temp file + rename.
	tmpPath := lockPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("mirage: lock write: %w", err)
	}
	if err := os.Rename(tmpPath, lockPath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup
		return nil, fmt.Errorf("mirage: lock rename: %w", err)
	}

	slog.Info("mirage: lock acquired",
		slog.String("workspace", workspace),
		slog.Int("pid", lock.PID),
	)
	return lock, nil
}

// Release releases a lock for a workspace. Only the owning process can release.
func (lm *LockManager) Release(workspace string) error {
	lockPath := lm.path(workspace)

	lock, err := lm.read(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // already released
		}
		// Can't read it — remove anyway.
		slog.Warn("mirage: unreadable lock, removing", slog.String("err", err.Error()))
		return os.Remove(lockPath)
	}

	currentPID := os.Getpid()
	if lock.PID != currentPID {
		if lock.IsStale() {
			slog.Warn("mirage: releasing stale lock from other process",
				slog.Int("owner_pid", lock.PID),
			)
			return os.Remove(lockPath)
		}
		return fmt.Errorf("mirage: cannot release lock owned by PID %d", lock.PID)
	}

	if err := os.Remove(lockPath); err != nil {
		return fmt.Errorf("mirage: lock remove: %w", err)
	}
	slog.Info("mirage: lock released", slog.String("workspace", workspace))
	return nil
}

// ForceRelease forcefully releases a lock by signaling the owning process.
// Sends SIGTERM, waits 5s, then SIGKILL.
func (lm *LockManager) ForceRelease(workspace string) error {
	lockPath := lm.path(workspace)

	lock, err := lm.read(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("mirage: lock read: %w", err)
	}

	if lock.IsStale() {
		return os.Remove(lockPath)
	}

	slog.Warn("mirage: force releasing lock",
		slog.String("workspace", workspace),
		slog.Int("pid", lock.PID),
	)

	// SIGTERM + 5s grace.
	_ = syscall.Kill(lock.PID, syscall.SIGTERM) //nolint:errcheck // best-effort
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if lock.IsStale() {
			return os.Remove(lockPath)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// SIGKILL + 1s wait.
	_ = syscall.Kill(lock.PID, syscall.SIGKILL) //nolint:errcheck // best-effort
	time.Sleep(1 * time.Second)

	return os.Remove(lockPath)
}

// IsLocked checks if a workspace is currently locked by an active process.
func (lm *LockManager) IsLocked(workspace string) bool {
	lock, err := lm.read(lm.path(workspace))
	if err != nil {
		return false
	}
	return !lock.IsStale()
}

// GetLock returns the current lock for a workspace, if any.
func (lm *LockManager) GetLock(workspace string) (*Lock, error) {
	return lm.read(lm.path(workspace))
}

func (lm *LockManager) path(workspace string) string {
	// Sanitize workspace path for use as filename.
	// Replace path separators with underscores.
	safe := strings.ReplaceAll(workspace, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	return filepath.Join(lm.locksDir, safe+".lock")
}

func (lm *LockManager) read(lockPath string) (*Lock, error) {
	data, err := os.ReadFile(lockPath) //nolint:gosec // lock paths are controlled
	if err != nil {
		return nil, err
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("mirage: lock unmarshal: %w", err)
	}
	return &lock, nil
}
