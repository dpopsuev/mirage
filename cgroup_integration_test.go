//go:build integration && linux

package mirage

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// requireCgroupV2 skips the test if cgroupv2 is not available or writable.
func requireCgroupV2(t *testing.T) string {
	t.Helper()
	root := "/sys/fs/cgroup"
	if _, err := os.Stat(filepath.Join(root, "cgroup.controllers")); err != nil {
		t.Skipf("cgroupv2 not available: %v", err)
	}

	// Find a writable cgroup subtree. Try user slice first (systemd delegation),
	// then fall back to creating under root (needs privileges).
	uid := os.Getuid()
	userSlice := filepath.Join(root, fmt.Sprintf("user.slice/user-%d.slice", uid))
	if _, err := os.Stat(userSlice); err == nil {
		// Try to create a test cgroup under user slice.
		testDir := filepath.Join(userSlice, "mirage-test-probe")
		if err := os.MkdirAll(testDir, 0o755); err == nil {
			os.Remove(testDir) //nolint:errcheck
			return userSlice
		}
	}

	// Try root directly (requires root).
	testDir := filepath.Join(root, "mirage-test-probe")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Skipf("cannot create cgroup directories (need root or systemd delegation): %v", err)
	}
	os.Remove(testDir) //nolint:errcheck
	return root
}

func TestCgroup_Integration_MemoryLimit(t *testing.T) {
	t.Parallel()
	cgRoot := requireCgroupV2(t)

	name := fmt.Sprintf("mirage-test-mem-%d", os.Getpid())
	mgr := newCgroupManagerWithRoot(name, cgRoot)

	err := mgr.Setup(ResourceLimits{Memory: "64MB"})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Cleanup() //nolint:errcheck

	// Verify memory.max was written correctly.
	content, err := os.ReadFile(filepath.Join(mgr.path(), "memory.max"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(content))
	want := strconv.FormatInt(64*1024*1024, 10)
	if got != want {
		t.Errorf("memory.max = %q, want %q", got, want)
	}
}

func TestCgroup_Integration_CPUWeight(t *testing.T) {
	t.Parallel()
	cgRoot := requireCgroupV2(t)

	name := fmt.Sprintf("mirage-test-cpu-%d", os.Getpid())
	mgr := newCgroupManagerWithRoot(name, cgRoot)

	err := mgr.Setup(ResourceLimits{CPUWeight: 500})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Cleanup() //nolint:errcheck

	content, err := os.ReadFile(filepath.Join(mgr.path(), "cpu.weight"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(content))
	if got != "500" {
		t.Errorf("cpu.weight = %q, want 500", got)
	}
}

func TestCgroup_Integration_PIDMax(t *testing.T) {
	t.Parallel()
	cgRoot := requireCgroupV2(t)

	name := fmt.Sprintf("mirage-test-pid-%d", os.Getpid())
	mgr := newCgroupManagerWithRoot(name, cgRoot)

	err := mgr.Setup(ResourceLimits{PIDMax: 256})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Cleanup() //nolint:errcheck

	content, err := os.ReadFile(filepath.Join(mgr.path(), "pids.max"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(content))
	if got != "256" {
		t.Errorf("pids.max = %q, want 256", got)
	}
}

func TestCgroup_Integration_AddProcess(t *testing.T) {
	t.Parallel()
	cgRoot := requireCgroupV2(t)

	name := fmt.Sprintf("mirage-test-proc-%d", os.Getpid())
	mgr := newCgroupManagerWithRoot(name, cgRoot)

	err := mgr.Setup(ResourceLimits{})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Cleanup() //nolint:errcheck

	// Add our own PID to the cgroup.
	err = mgr.AddProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}

	// Verify PID is in cgroup.procs.
	content, err := os.ReadFile(filepath.Join(mgr.path(), "cgroup.procs"))
	if err != nil {
		t.Fatal(err)
	}
	pidStr := strconv.Itoa(os.Getpid())
	if !strings.Contains(string(content), pidStr) {
		t.Errorf("cgroup.procs does not contain PID %s: %q", pidStr, content)
	}
}

func TestCgroup_Integration_Cleanup(t *testing.T) {
	t.Parallel()
	cgRoot := requireCgroupV2(t)

	name := fmt.Sprintf("mirage-test-clean-%d", os.Getpid())
	mgr := newCgroupManagerWithRoot(name, cgRoot)

	err := mgr.Setup(ResourceLimits{})
	if err != nil {
		t.Fatal(err)
	}

	// Cleanup removes the directory.
	if err := mgr.Cleanup(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(mgr.path()); !os.IsNotExist(err) {
		t.Fatal("cgroup directory should not exist after cleanup")
	}
}
