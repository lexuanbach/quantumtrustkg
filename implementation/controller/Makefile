# Build and test entry for the Go controller module (Sect. 4, implementation).
# test runs the unit suite in tests/unit, which checks the promotion gates and
# the mesh and status writers. build and run start cmd/manager.
GO ?= go

.PHONY: test build run fmt tidy

test:
	$(GO) test ./...

build:
	$(GO) build ./cmd/manager

run:
	$(GO) run ./cmd/manager

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy
