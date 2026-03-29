// helpers.go — Pure diff/commit functions for external overlay management.
//
// When the caller manages the fuse mount (e.g., Misbah daemon),
// these functions provide diff/commit over raw lower/upper directories.
package mirage

import (
	"fmt"
	"os"
	"path/filepath"
)

// DiffDirs compares upper directory against lower to find changes.
// Returns files in upper that are new (created) or different (modified).
func DiffDirs(lower, upper string) ([]Change, error) {
	var changes []Change
	err := filepath.Walk(upper, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == upper || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(upper, path)

		kind := Created
		if _, statErr := os.Stat(filepath.Join(lower, rel)); statErr == nil {
			kind = Modified
		}
		changes = append(changes, Change{
			Path: rel,
			Kind: kind,
			Size: info.Size(),
		})
		return nil
	})
	return changes, err
}

// CommitFiles copies selected files from upper to lower directory.
func CommitFiles(upper, lower string, paths []string) error {
	for _, p := range paths {
		src := filepath.Join(upper, p)
		dst := filepath.Join(lower, p)

		data, err := os.ReadFile(src) //nolint:gosec // path from controlled overlay
		if err != nil {
			return fmt.Errorf("mirage: commit read %s: %w", p, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mirage: commit mkdir: %w", err)
		}
		info, _ := os.Stat(src) //nolint:gosec // path from controlled overlay
		if err := os.WriteFile(dst, data, info.Mode()); err != nil {
			return fmt.Errorf("mirage: commit write %s: %w", p, err)
		}
	}
	return nil
}
