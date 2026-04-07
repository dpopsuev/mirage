//go:build linux

package mirage

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLockManager_AcquireRelease(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lm := NewLockManager(dir)

	lock, err := lm.Acquire("myproject", "test")
	if err != nil {
		t.Fatal(err)
	}
	if lock.Workspace != "myproject" {
		t.Errorf("Workspace = %q, want myproject", lock.Workspace)
	}
	if lock.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", lock.PID, os.Getpid())
	}

	// Should be locked.
	if !lm.IsLocked("myproject") {
		t.Fatal("workspace should be locked")
	}

	// Release.
	if err := lm.Release("myproject"); err != nil {
		t.Fatal(err)
	}

	// Should no longer be locked.
	if lm.IsLocked("myproject") {
		t.Fatal("workspace should not be locked after release")
	}
}

func TestLockManager_DoubleAcquire(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lm := NewLockManager(dir)

	_, err := lm.Acquire("proj", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Second acquire should fail.
	_, err = lm.Acquire("proj", "test")
	if err == nil {
		t.Fatal("expected error on double acquire")
	}

	// Cleanup.
	lm.Release("proj") //nolint:errcheck
}

func TestLockManager_ReleaseIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lm := NewLockManager(dir)

	// Release non-existent lock should not error.
	if err := lm.Release("nonexistent"); err != nil {
		t.Fatalf("release non-existent should succeed, got %v", err)
	}
}

func TestLockManager_GetLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lm := NewLockManager(dir)

	_, err := lm.Acquire("proj", "mirage")
	if err != nil {
		t.Fatal(err)
	}

	lock, err := lm.GetLock("proj")
	if err != nil {
		t.Fatal(err)
	}
	if lock.Provider != "mirage" {
		t.Errorf("Provider = %q, want mirage", lock.Provider)
	}
	if lock.User == "" {
		t.Error("User should not be empty")
	}

	lm.Release("proj") //nolint:errcheck
}

func TestLockManager_StaleDetection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lm := NewLockManager(dir)

	// Create a lock with a PID that doesn't exist.
	lock := &Lock{
		Workspace: "stale",
		Provider:  "test",
		PID:       999999999, // hopefully doesn't exist
	}
	if !lock.IsStale() {
		t.Skip("PID 999999999 exists on this system, skipping")
	}

	// Write the stale lock manually.
	data, _ := lock.marshalJSON()
	os.WriteFile(lm.path("stale"), data, 0o644) //nolint:errcheck

	// Acquire should succeed because the existing lock is stale.
	newLock, err := lm.Acquire("stale", "test")
	if err != nil {
		t.Fatalf("should acquire over stale lock, got %v", err)
	}
	if newLock.PID != os.Getpid() {
		t.Errorf("new lock PID = %d, want %d", newLock.PID, os.Getpid())
	}

	lm.Release("stale") //nolint:errcheck
}

func TestLockManager_IsLockedFalseWhenNoLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lm := NewLockManager(dir)

	if lm.IsLocked("nothing") {
		t.Fatal("should not be locked when no lock exists")
	}
}

// marshalJSON is a test helper to create lock file content.
func (l *Lock) marshalJSON() ([]byte, error) {
	return json.MarshalIndent(l, "", "  ")
}
