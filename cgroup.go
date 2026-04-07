//go:build linux

package mirage

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Sentinel errors for cgroup operations.
var (
	ErrCgroupV2NotAvailable = errors.New("mirage: cgroup v2 not available")
)

// CgroupManager manages cgroupv2 resource limits for a named space.
// Writes to /sys/fs/cgroup/mirage/{name}/.
type CgroupManager struct {
	root string // cgroup filesystem root (default /sys/fs/cgroup)
	name string // space name → cgroup subdirectory
}

// NewCgroupManager creates a cgroup manager for the given space name.
func NewCgroupManager(name string) *CgroupManager {
	return &CgroupManager{
		root: "/sys/fs/cgroup",
		name: name,
	}
}

// newCgroupManagerWithRoot creates a cgroup manager with a custom root (for testing).
func newCgroupManagerWithRoot(name, root string) *CgroupManager {
	return &CgroupManager{root: root, name: name}
}

// Setup creates the cgroup directory and writes resource limit files.
func (c *CgroupManager) Setup(limits ResourceLimits) error {
	if !c.isCgroupV2Available() {
		return ErrCgroupV2NotAvailable
	}

	cgPath := c.path()

	// Ensure parent "mirage" cgroup exists and has controllers enabled.
	parentPath := filepath.Dir(cgPath)
	if err := os.MkdirAll(parentPath, 0o755); err != nil {
		return fmt.Errorf("mirage: cgroup parent mkdir: %w", err)
	}
	c.enableControllers(parentPath, limits)

	if err := os.MkdirAll(cgPath, 0o755); err != nil {
		return fmt.Errorf("mirage: cgroup mkdir: %w", err)
	}

	slog.Info("mirage: cgroup setup",
		slog.String("name", c.name),
		slog.String("path", cgPath),
	)

	if limits.Memory != "" {
		bytes, err := ParseMemory(limits.Memory)
		if err != nil {
			return fmt.Errorf("mirage: cgroup memory: %w", err)
		}
		if err := c.write("memory.max", strconv.FormatInt(bytes, 10)); err != nil {
			return err
		}
		// Enable OOM group kill — entire cgroup killed as unit.
		if err := c.write("memory.oom.group", "1"); err != nil {
			slog.Warn("mirage: cgroup memory.oom.group not writable", slog.String("err", err.Error()))
		}
	}

	if limits.CPUWeight > 0 {
		if err := c.write("cpu.weight", strconv.Itoa(limits.CPUWeight)); err != nil {
			return err
		}
	}

	if limits.CPUMax != "" {
		if err := c.write("cpu.max", limits.CPUMax); err != nil {
			return err
		}
	}

	if limits.IOWeight > 0 {
		if err := c.write("io.weight", fmt.Sprintf("default %d", limits.IOWeight)); err != nil {
			return err
		}
	}

	if limits.PIDMax > 0 {
		if err := c.write("pids.max", strconv.Itoa(limits.PIDMax)); err != nil {
			return err
		}
	}

	return nil
}

// AddProcess writes a PID to cgroup.procs, placing it under cgroup limits.
func (c *CgroupManager) AddProcess(pid int) error {
	slog.Info("mirage: cgroup add process",
		slog.String("name", c.name),
		slog.Int("pid", pid),
	)
	return c.write("cgroup.procs", strconv.Itoa(pid))
}

// Cleanup removes the cgroup directory. Fails if processes are still in it.
func (c *CgroupManager) Cleanup() error {
	cgPath := c.path()
	if err := os.Remove(cgPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("mirage: cgroup cleanup: %w", err)
	}
	slog.Info("mirage: cgroup cleaned up", slog.String("name", c.name))
	return nil
}

// path returns the full cgroup directory path.
func (c *CgroupManager) path() string {
	return filepath.Join(c.root, "mirage", c.name)
}

// isCgroupV2Available checks for the cgroupv2 unified hierarchy.
func (c *CgroupManager) isCgroupV2Available() bool {
	controllersFile := filepath.Join(c.root, "cgroup.controllers")
	_, err := os.Stat(controllersFile)
	return err == nil
}

// enableControllers writes to cgroup.subtree_control in the parent to enable
// the controllers needed by the requested resource limits.
func (c *CgroupManager) enableControllers(parentPath string, limits ResourceLimits) {
	var controllers []string
	if limits.Memory != "" {
		controllers = append(controllers, "+memory")
	}
	if limits.CPUWeight > 0 || limits.CPUMax != "" {
		controllers = append(controllers, "+cpu")
	}
	if limits.IOWeight > 0 {
		controllers = append(controllers, "+io")
	}
	if limits.PIDMax > 0 {
		controllers = append(controllers, "+pids")
	}
	if len(controllers) == 0 {
		return
	}

	subtreeControl := filepath.Join(parentPath, "cgroup.subtree_control")
	content := strings.Join(controllers, " ")
	if err := os.WriteFile(subtreeControl, []byte(content), 0o644); err != nil { //nolint:gosec
		slog.Warn("mirage: cgroup enable controllers failed (may need delegation)",
			slog.String("path", subtreeControl),
			slog.String("controllers", content),
			slog.String("err", err.Error()),
		)
	}
}

// write writes content to a cgroup control file.
func (c *CgroupManager) write(file, content string) error {
	path := filepath.Join(c.path(), file)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // cgroup paths are controlled
		slog.Warn("mirage: cgroup write failed",
			slog.String("file", file),
			slog.String("err", err.Error()),
		)
		return fmt.Errorf("mirage: cgroup write %s: %w", file, err)
	}
	return nil
}
