//go:build linux

package mirage

import (
	"strings"
	"testing"
)

func TestBuildUnshareArgs_Full(t *testing.T) {
	t.Parallel()
	args := BuildUnshareArgs(NamespaceConfig{
		User: true, Mount: true, PID: true,
		Network: true, IPC: true, UTS: true, Cgroup: true,
	})

	want := []string{
		"--user", "--map-root-user",
		"--mount",
		"--pid", "--fork",
		"--net",
		"--ipc",
		"--uts",
		"--cgroup",
	}
	if len(args) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(want), args)
	}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d] = %q, want %q", i, args[i], w)
		}
	}
}

func TestBuildUnshareArgs_Minimal(t *testing.T) {
	t.Parallel()
	args := BuildUnshareArgs(NamespaceConfig{User: true, Mount: true})
	if len(args) != 3 {
		t.Fatalf("got %d args, want 3: %v", len(args), args)
	}
}

func TestBuildUnshareArgs_Empty(t *testing.T) {
	t.Parallel()
	args := BuildUnshareArgs(NamespaceConfig{})
	if len(args) != 0 {
		t.Fatalf("got %d args, want 0: %v", len(args), args)
	}
}

func TestBuildMountScript_Bind(t *testing.T) {
	t.Parallel()
	script := BuildMountScript([]MountSpec{
		{Type: MountBind, Source: "/src", Destination: "/dst", Options: []string{"ro"}},
	})
	if !strings.Contains(script, "mount --bind") {
		t.Errorf("expected bind mount in script: %s", script)
	}
	if !strings.Contains(script, "bind,ro") {
		t.Errorf("expected ro option: %s", script)
	}
	if !strings.Contains(script, "mkdir -p") {
		t.Errorf("expected mkdir: %s", script)
	}
}

func TestBuildMountScript_Tmpfs(t *testing.T) {
	t.Parallel()
	script := BuildMountScript([]MountSpec{
		{Type: MountTmpfs, Destination: "/tmp", Options: []string{"size=1G", "huge=always"}},
	})
	if !strings.Contains(script, "mount -t tmpfs") {
		t.Errorf("expected tmpfs mount: %s", script)
	}
	if !strings.Contains(script, "size=1G,huge=always") {
		t.Errorf("expected options: %s", script)
	}
}

func TestBuildMountScript_Proc(t *testing.T) {
	t.Parallel()
	script := BuildMountScript([]MountSpec{
		{Type: MountProc, Destination: "/proc"},
	})
	if !strings.Contains(script, "mount -t proc proc") {
		t.Errorf("expected proc mount: %s", script)
	}
}

func TestBuildMountScript_Empty(t *testing.T) {
	t.Parallel()
	script := BuildMountScript(nil)
	if script != "" {
		t.Errorf("expected empty script, got %q", script)
	}
}

func TestBuildShellCommand(t *testing.T) {
	t.Parallel()
	cmd := BuildShellCommand("mount stuff\n", "/workspace", []string{"bash", "-c", "echo 'hello world'"})
	if !strings.Contains(cmd, "set -e") {
		t.Error("expected set -e")
	}
	if !strings.Contains(cmd, "mount stuff") {
		t.Error("expected mount script")
	}
	if !strings.Contains(cmd, "cd '/workspace'") {
		t.Error("expected cd")
	}
	if !strings.Contains(cmd, "exec") {
		t.Error("expected exec")
	}
}

func TestShellQuote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"it's", "'it'\\''s'"},
		{"", "''"},
	}
	for _, tt := range tests {
		got := ShellQuote(tt.input)
		if got != tt.want {
			t.Errorf("ShellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
