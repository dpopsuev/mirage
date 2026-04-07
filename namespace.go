//go:build linux

package mirage

import (
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
)

// Sentinel errors for namespace operations.
var (
	ErrNamespaceNotSupported = fmt.Errorf("mirage: namespaces not supported on %s", runtime.GOOS)
	ErrUnshareNotFound       = fmt.Errorf("mirage: unshare command not found (install util-linux)")
)

// BuildUnshareArgs builds the unshare command arguments from a NamespaceConfig.
func BuildUnshareArgs(ns NamespaceConfig) []string {
	var args []string

	if ns.User {
		args = append(args, "--user", "--map-root-user")
	}
	if ns.Mount {
		args = append(args, "--mount")
	}
	if ns.PID {
		args = append(args, "--pid", "--fork")
	}
	if ns.Network {
		args = append(args, "--net")
	}
	if ns.IPC {
		args = append(args, "--ipc")
	}
	if ns.UTS {
		args = append(args, "--uts")
	}
	if ns.Cgroup {
		args = append(args, "--cgroup")
	}

	return args
}

// BuildMountScript generates a shell script for mounting inside a namespace.
func BuildMountScript(mounts []MountSpec) string {
	var script strings.Builder

	for _, mount := range mounts {
		switch mount.Type {
		case MountBind:
			buildBindMount(&script, mount)
		case MountTmpfs:
			buildTmpfsMount(&script, mount)
		case MountProc:
			buildProcMount(&script, mount)
		case MountOverlay:
			buildOverlayMount(&script, mount)
		default:
			slog.Warn("mirage: unknown mount type", slog.String("type", string(mount.Type)))
		}
	}

	return script.String()
}

// BuildShellCommand builds a shell command that applies mounts then executes a command.
func BuildShellCommand(mountScript, cwd string, command []string) string {
	var quoted []string
	for _, arg := range command {
		quoted = append(quoted, ShellQuote(arg))
	}
	cmdStr := strings.Join(quoted, " ")

	return fmt.Sprintf("set -e\n%scd %s\nexec %s\n",
		mountScript, ShellQuote(cwd), cmdStr)
}

// CheckNamespaceSupport verifies that unprivileged user namespaces work.
func CheckNamespaceSupport() error {
	if runtime.GOOS != "linux" {
		return ErrNamespaceNotSupported
	}

	if _, err := exec.LookPath("unshare"); err != nil {
		return ErrUnshareNotFound
	}

	cmd := exec.Command("unshare",
		"--user", "--mount", "--map-root-user",
		"--pid", "--fork",
		"true")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mirage: unprivileged user namespaces not available: %w\n"+
			"  sudo sysctl -w kernel.unprivileged_userns_clone=1\n"+
			"  sudo sysctl -w user.max_user_namespaces=15000", err)
	}

	slog.Info("mirage: namespace support verified")
	return nil
}

// ShellQuote escapes a string for safe embedding in a POSIX shell command.
func ShellQuote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func buildBindMount(w *strings.Builder, m MountSpec) {
	fmt.Fprintf(w, "mkdir -p %s\n", ShellQuote(m.Destination))

	options := "bind"
	for _, opt := range m.Options {
		if opt != "bind" && opt != "rbind" {
			options += "," + opt
		}
	}
	fmt.Fprintf(w, "mount --bind -o %s %s %s\n", options, ShellQuote(m.Source), ShellQuote(m.Destination))
}

func buildTmpfsMount(w *strings.Builder, m MountSpec) {
	fmt.Fprintf(w, "mkdir -p %s\n", ShellQuote(m.Destination))

	options := ""
	if len(m.Options) > 0 {
		options = " -o " + strings.Join(m.Options, ",")
	}
	fmt.Fprintf(w, "mount -t tmpfs%s tmpfs %s\n", options, ShellQuote(m.Destination))
}

func buildProcMount(w *strings.Builder, m MountSpec) {
	fmt.Fprintf(w, "mkdir -p %s\n", ShellQuote(m.Destination))
	fmt.Fprintf(w, "mount -t proc proc %s\n", ShellQuote(m.Destination))
}

func buildOverlayMount(w *strings.Builder, m MountSpec) {
	// Overlay uses Source as lowerdir. Needs upper/work dirs created.
	// For full overlay support, the caller should provide these via Options
	// or use the overlay backend directly.
	fmt.Fprintf(w, "mkdir -p %s\n", ShellQuote(m.Destination))
	if m.Source != "" {
		fmt.Fprintf(w, "fuse-overlayfs -o lowerdir=%s %s\n",
			ShellQuote(m.Source), ShellQuote(m.Destination))
	}
}
