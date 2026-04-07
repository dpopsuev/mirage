package testkit

import (
	"sync"
	"testing"

	"github.com/dpopsuev/mirage"
)

// Guard tracks Spaces for panic-safe cleanup. Register via t.Cleanup
// to ensure all Spaces are destroyed even if the test panics or calls t.Fatal.
type Guard struct {
	mu     sync.Mutex
	spaces []mirage.Space
}

// NewGuard creates a Guard and registers its cleanup with t.Cleanup.
func NewGuard(t *testing.T) *Guard {
	t.Helper()
	g := &Guard{}
	t.Cleanup(g.Cleanup)
	return g
}

// Track registers a Space for cleanup.
func (g *Guard) Track(s mirage.Space) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.spaces = append(g.spaces, s)
}

// Cleanup destroys all tracked Spaces in reverse order.
// Errors are logged but do not cause test failure (cleanup is best-effort).
func (g *Guard) Cleanup() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := len(g.spaces) - 1; i >= 0; i-- {
		g.spaces[i].Destroy() //nolint:errcheck // best-effort cleanup
	}
	g.spaces = nil
}
