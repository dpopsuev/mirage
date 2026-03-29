// Package mirage provides copy-on-write workspace isolation for AI agents.
//
// Five verbs: Create a Space, Diff to see changes, Commit to keep them,
// Reset to discard, Destroy when done. Pure overlay library — no tiers,
// no sandboxing policy, no domain concepts.
//
// Used by Djinn (standalone agent shell) and Misbah (container runtime)
// as their shared overlay contract.
package mirage

// Space is an isolated agent workspace with copy-on-write semantics.
// Reads pass through to the real workspace. Writes are captured in
// a separate layer. Diff shows what changed. Commit promotes changes.
type Space interface {
	// Diff returns files changed since the space was created.
	Diff() ([]Change, error)

	// Commit promotes selected files from the overlay to the real workspace.
	// Only listed paths are promoted. Others stay in the overlay only.
	Commit(paths []string) error

	// Reset discards all overlay changes. The real workspace is untouched.
	Reset() error

	// Destroy tears down the space and removes all temp directories.
	Destroy() error

	// WorkDir returns the path the agent should use as its working directory.
	// This is the merged view (reads from real workspace, writes to overlay).
	WorkDir() string
}

// Change describes one file modification in the space.
type Change struct {
	Path string     `json:"path"` // relative to workspace root
	Kind ChangeKind `json:"kind"`
	Size int64      `json:"size,omitempty"` // bytes (0 for deleted)
}

// ChangeKind classifies what happened to a file.
type ChangeKind string

const (
	Created  ChangeKind = "created"
	Modified ChangeKind = "modified"
	Deleted  ChangeKind = "deleted"
)

// Builder creates Spaces over a given workspace directory.
type Builder interface {
	Create(workspace string) (Space, error)
}
