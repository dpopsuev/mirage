package mirage

import (
	"fmt"
	"log/slog"
)

// Create creates an isolated Space using the specified backend.
// This is the facade — one function, pluggable backends.
func Create(spec Spec) (Space, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	slog.Info("mirage: creating space",
		slog.String("workspace", spec.Workspace),
		slog.String("backend", string(spec.Backend)),
	)

	switch spec.Backend {
	case Overlay:
		return createOverlay(spec)
	case Stub:
		return createStub(spec), nil
	case Container:
		return nil, fmt.Errorf("%w: container backend not yet implemented", ErrUnknownBackend)
	case Sandbox:
		return nil, fmt.Errorf("%w: sandbox backend not yet implemented", ErrUnknownBackend)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownBackend, spec.Backend)
	}
}
