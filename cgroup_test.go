//go:build linux

package mirage

import (
	"os"
	"path/filepath"
	"testing"
)

// mockCgroupRoot creates a fake cgroup filesystem in t.TempDir
// with a cgroup.controllers file so isCgroupV2Available returns true.
func mockCgroupRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory io pids"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCgroupManager_Setup_Memory(t *testing.T) {
	t.Parallel()
	root := mockCgroupRoot(t)
	mgr := newCgroupManagerWithRoot("test-agent", root)

	err := mgr.Setup(ResourceLimits{Memory: "512MB"})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(mgr.path(), "memory.max"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "536870912" {
		t.Errorf("memory.max = %q, want 536870912", content)
	}

	// OOM group should also be set.
	oomContent, err := os.ReadFile(filepath.Join(mgr.path(), "memory.oom.group"))
	if err != nil {
		t.Fatal(err)
	}
	if string(oomContent) != "1" {
		t.Errorf("memory.oom.group = %q, want 1", oomContent)
	}
}

func TestCgroupManager_Setup_CPUWeight(t *testing.T) {
	t.Parallel()
	root := mockCgroupRoot(t)
	mgr := newCgroupManagerWithRoot("test-cpu", root)

	err := mgr.Setup(ResourceLimits{CPUWeight: 5000})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(mgr.path(), "cpu.weight"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "5000" {
		t.Errorf("cpu.weight = %q, want 5000", content)
	}
}

func TestCgroupManager_Setup_CPUMax(t *testing.T) {
	t.Parallel()
	root := mockCgroupRoot(t)
	mgr := newCgroupManagerWithRoot("test-cpumax", root)

	err := mgr.Setup(ResourceLimits{CPUMax: "200000 100000"})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(mgr.path(), "cpu.max"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "200000 100000" {
		t.Errorf("cpu.max = %q, want \"200000 100000\"", content)
	}
}

func TestCgroupManager_Setup_IOWeight(t *testing.T) {
	t.Parallel()
	root := mockCgroupRoot(t)
	mgr := newCgroupManagerWithRoot("test-io", root)

	err := mgr.Setup(ResourceLimits{IOWeight: 200})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(mgr.path(), "io.weight"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "default 200" {
		t.Errorf("io.weight = %q, want \"default 200\"", content)
	}
}

func TestCgroupManager_Setup_PIDMax(t *testing.T) {
	t.Parallel()
	root := mockCgroupRoot(t)
	mgr := newCgroupManagerWithRoot("test-pids", root)

	err := mgr.Setup(ResourceLimits{PIDMax: 100})
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(mgr.path(), "pids.max"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "100" {
		t.Errorf("pids.max = %q, want 100", content)
	}
}

func TestCgroupManager_Setup_AllLimits(t *testing.T) {
	t.Parallel()
	root := mockCgroupRoot(t)
	mgr := newCgroupManagerWithRoot("test-all", root)

	err := mgr.Setup(ResourceLimits{
		Memory:    "2GB",
		CPUWeight: 1000,
		CPUMax:    "max 100000",
		IOWeight:  500,
		PIDMax:    50,
	})
	if err != nil {
		t.Fatal(err)
	}

	checks := map[string]string{
		"memory.max":      "2147483648",
		"cpu.weight":      "1000",
		"cpu.max":         "max 100000",
		"io.weight":       "default 500",
		"pids.max":        "50",
		"memory.oom.group": "1",
	}
	for file, want := range checks {
		got, err := os.ReadFile(filepath.Join(mgr.path(), file))
		if err != nil {
			t.Errorf("read %s: %v", file, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", file, got, want)
		}
	}
}

func TestCgroupManager_Setup_Empty(t *testing.T) {
	t.Parallel()
	root := mockCgroupRoot(t)
	mgr := newCgroupManagerWithRoot("test-empty", root)

	// Empty limits should create the directory but write no files.
	err := mgr.Setup(ResourceLimits{})
	if err != nil {
		t.Fatal(err)
	}

	// Directory should exist.
	if _, err := os.Stat(mgr.path()); err != nil {
		t.Fatal("cgroup directory should exist")
	}

	// No resource files should be written.
	for _, file := range []string{"memory.max", "cpu.weight", "cpu.max", "io.weight", "pids.max"} {
		if _, err := os.Stat(filepath.Join(mgr.path(), file)); err == nil {
			t.Errorf("%s should not exist for empty limits", file)
		}
	}
}

func TestCgroupManager_AddProcess(t *testing.T) {
	t.Parallel()
	root := mockCgroupRoot(t)
	mgr := newCgroupManagerWithRoot("test-proc", root)

	// Must setup first to create directory.
	if err := mgr.Setup(ResourceLimits{}); err != nil {
		t.Fatal(err)
	}

	if err := mgr.AddProcess(12345); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(mgr.path(), "cgroup.procs"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "12345" {
		t.Errorf("cgroup.procs = %q, want 12345", content)
	}
}

func TestCgroupManager_Cleanup(t *testing.T) {
	t.Parallel()
	root := mockCgroupRoot(t)
	mgr := newCgroupManagerWithRoot("test-cleanup", root)

	if err := mgr.Setup(ResourceLimits{}); err != nil {
		t.Fatal(err)
	}

	// Cleanup should remove the directory.
	if err := mgr.Cleanup(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(mgr.path()); !os.IsNotExist(err) {
		t.Fatal("cgroup directory should not exist after Cleanup")
	}

	// Cleanup on non-existent should not error (idempotent).
	if err := mgr.Cleanup(); err != nil {
		t.Fatalf("second Cleanup should be idempotent, got %v", err)
	}
}

func TestCgroupManager_NoCgroupV2(t *testing.T) {
	t.Parallel()
	// Root without cgroup.controllers = no cgroupv2.
	root := t.TempDir()
	mgr := newCgroupManagerWithRoot("test-nocg", root)

	err := mgr.Setup(ResourceLimits{Memory: "512MB"})
	if err != ErrCgroupV2NotAvailable {
		t.Errorf("expected ErrCgroupV2NotAvailable, got %v", err)
	}
}

func TestCgroupManager_Path(t *testing.T) {
	t.Parallel()
	mgr := newCgroupManagerWithRoot("agent-1", "/sys/fs/cgroup")
	want := "/sys/fs/cgroup/mirage/agent-1"
	if mgr.path() != want {
		t.Errorf("path() = %q, want %q", mgr.path(), want)
	}
}
