package mirage

import "errors"

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
