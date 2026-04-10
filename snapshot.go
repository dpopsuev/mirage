//go:build linux

// snapshot.go — Snapshot/Restore stubs for overlay and container backends.
// Real overlay implementation in Step 5 (Forge: stub first, impl second).
package mirage

// --- overlaySpace stubs (real impl comes after RED tests) ---

func (s *overlaySpace) Snapshot(_ string) error {
	return ErrSnapshotNotImpl
}

func (s *overlaySpace) Restore(_ string) error {
	return ErrSnapshotNotImpl
}

func (s *overlaySpace) Snapshots() []string {
	return nil
}

// --- containerSpace stubs ---

func (s *containerSpace) Snapshot(_ string) error {
	return ErrSnapshotNotImpl
}

func (s *containerSpace) Restore(_ string) error {
	return ErrSnapshotNotImpl
}

func (s *containerSpace) Snapshots() []string {
	return nil
}
