.PHONY: test test-unit test-integration test-e2e test-e2e-netns lint build clean

# Default: unit + integration
test: test-unit test-integration

# Unit tests — always safe, no system resources, no build tags
test-unit:
	go test -race -count=1 ./...

# Integration tests — real fuse-overlayfs, real cgroup writes (Linux only)
test-integration:
	go test -race -count=1 -tags=integration ./...

# E2E tests — full container lifecycle with real resources
test-e2e:
	go test -race -count=1 -tags=e2e ./...

# E2E + network namespace tests — requires root
test-e2e-netns:
	sudo go test -race -count=1 -tags='e2e netns' ./...

# Lint
lint:
	golangci-lint run ./...

# Lint only new changes (used by pre-commit hook)
lint-new:
	golangci-lint run --new-from-rev=HEAD ./...

# Build
build:
	go build ./...

# Clean test caches
clean:
	go clean -testcache
