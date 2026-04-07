// Package testkit provides test infrastructure for mirage Space consumers.
//
// Three components:
//
//   - Guard: tracks Spaces for panic-safe cleanup via t.Cleanup.
//   - Probe: non-invasive state inspection — poll for changes, check files.
//   - Assert: domain-specific assertions — clean, committed, destroyed.
//
// Consumers (e.g., Djinn) import this package to write Space-aware tests
// without depending on real backends. Works with StubSpace or real overlays.
package testkit
