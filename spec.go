package mirage

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Backend identifies the isolation mechanism.
type Backend string

const (
	// Overlay uses fuse-overlayfs for filesystem-only isolation.
	Overlay Backend = "overlay"

	// Container uses Linux namespaces + cgroups + optional Kata VM.
	Container Backend = "container"

	// Sandbox uses K8s agent-sandbox CRD (Day 2).
	Sandbox Backend = "sandbox"

	// Stub is an in-memory backend for testing. No I/O.
	Stub Backend = "stub"
)

// Sentinel errors.
var (
	ErrWorkspaceRequired = errors.New("mirage: workspace path is required")
	ErrBackendRequired   = errors.New("mirage: backend is required")
	ErrUnknownBackend    = errors.New("mirage: unknown backend")
)

// Spec configures a Space. The only config surface for mirage.Create().
type Spec struct {
	Workspace  string           // base workspace path (required)
	Backend    Backend          // explicit backend selection (required)
	RWPaths    []string         // writable paths (default: all)
	Mounts     []MountSpec      // additional mounts (bind, tmpfs, proc, overlay)
	Network    *NetworkPolicy   // egress allowlist (nil = no network isolation)
	Resources  *ResourceLimits  // cgroup limits (nil = unlimited)
	Namespaces *NamespaceConfig // which namespaces to create (nil = overlay only)
}

// Validate checks that required fields are set.
func (s Spec) Validate() error {
	if s.Workspace == "" {
		return ErrWorkspaceRequired
	}
	if s.Backend == "" {
		return ErrBackendRequired
	}
	switch s.Backend {
	case Overlay, Container, Sandbox, Stub:
		return nil
	default:
		return ErrUnknownBackend
	}
}

// MountSpec describes an additional mount inside the space.
type MountSpec struct {
	Type        MountType // bind, tmpfs, proc, overlay
	Source      string    // host path (for bind mounts)
	Destination string    // path inside the space
	Options     []string  // ro, rw, nosuid, nodev, noexec
}

// MountType classifies mount behavior.
type MountType string

const (
	MountBind    MountType = "bind"
	MountTmpfs   MountType = "tmpfs"
	MountProc    MountType = "proc"
	MountOverlay MountType = "overlay"
)

// NetworkPolicy controls network isolation.
type NetworkPolicy struct {
	Mode        NetworkMode // none, host, isolated
	AllowEgress []string   // domain:port allowlist (isolated mode)
	DNS         []string   // DNS servers
}

// NetworkMode classifies network isolation level.
type NetworkMode string

const (
	NetNone     NetworkMode = "none"     // no network access
	NetHost     NetworkMode = "host"     // share host network
	NetIsolated NetworkMode = "isolated" // isolated netns with egress allowlist
)

// ResourceLimits controls cgroup resource constraints.
type ResourceLimits struct {
	Memory    string // human-readable: "512MB", "2GB"
	CPUWeight int    // 1-10000 (cgroup cpu.weight)
	IOWeight  int    // 1-10000 (cgroup io.weight)
}

// NamespaceConfig controls which Linux namespaces to create.
type NamespaceConfig struct {
	User    bool // required for unprivileged containers
	Mount   bool // required for overlay filesystem
	PID     bool // process isolation
	Network bool // network isolation (requires NetworkPolicy)
	IPC     bool // inter-process communication isolation
	UTS     bool // hostname isolation
}

// --- Validation methods ---

// Validate checks MountSpec constraints.
func (m MountSpec) Validate() error {
	if m.Destination == "" {
		return errors.New("mirage: mount destination is required")
	}
	switch m.Type {
	case MountBind:
		if m.Source == "" {
			return errors.New("mirage: bind mount requires source path")
		}
	case MountTmpfs, MountProc, MountOverlay:
		// no source required
	default:
		return fmt.Errorf("mirage: unknown mount type %q", m.Type)
	}
	return nil
}

// Validate checks NetworkPolicy constraints.
func (p NetworkPolicy) Validate() error {
	if p.Mode == "" {
		return errors.New("mirage: network mode is required")
	}
	switch p.Mode {
	case NetNone, NetHost, NetIsolated:
		return nil
	default:
		return fmt.Errorf("mirage: unknown network mode %q", p.Mode)
	}
}

// Validate checks ResourceLimits constraints.
func (r ResourceLimits) Validate() error {
	if r.Memory != "" {
		if _, err := ParseMemory(r.Memory); err != nil {
			return fmt.Errorf("mirage: invalid memory spec %q: %w", r.Memory, err)
		}
	}
	if r.CPUWeight < 0 || r.CPUWeight > 10000 {
		return fmt.Errorf("mirage: cpu_weight must be 0-10000, got %d", r.CPUWeight)
	}
	if r.IOWeight < 0 || r.IOWeight > 10000 {
		return fmt.Errorf("mirage: io_weight must be 0-10000, got %d", r.IOWeight)
	}
	return nil
}

// Validate checks NamespaceConfig constraints.
func (n NamespaceConfig) Validate() error {
	// If any namespace beyond User+Mount is requested, User must be set
	// (unprivileged namespace creation requires user namespace).
	if (n.PID || n.Network || n.IPC || n.UTS) && !n.User {
		return errors.New("mirage: user namespace required when enabling PID, Network, IPC, or UTS namespaces")
	}
	return nil
}

// ParseMemory parses a human-readable memory spec into bytes.
// Supports: "512KB", "512MB", "2GB", or raw bytes "1073741824".
// Empty string returns 0 (no limit).
func ParseMemory(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}

	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)

	var multiplier int64 = 1
	numStr := s

	switch {
	case strings.HasSuffix(upper, "KB"):
		multiplier = 1024
		numStr = s[:len(s)-2]
	case strings.HasSuffix(upper, "MB"):
		multiplier = 1024 * 1024
		numStr = s[:len(s)-2]
	case strings.HasSuffix(upper, "GB"):
		multiplier = 1024 * 1024 * 1024
		numStr = s[:len(s)-2]
	}

	n, err := strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse %q as memory size", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("memory size cannot be negative: %s", s)
	}
	return n * multiplier, nil
}
