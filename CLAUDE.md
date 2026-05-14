# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

CLI tool that fetches the latest setlist for a given artist from Setlist.fm and will create a Spotify playlist from it. Uses only the Go standard library — no external dependencies.

Requires the `SETLISTFM_API_KEY` environment variable.

## Commands

```bash
go build ./cmd/...          # build the binary
go test ./...               # run all tests
go test ./internal/...      # run tests (skips cmd which has no tests)
go test -run TestName ./internal/adapters/   # run a single test
go vet ./...                # static analysis
```

## Architecture

Hexagonal architecture (Ports & Adapters):

- **`internal/core/domain/`** — pure structs (`Artist`, `Setlist`), domain logic only
- **`internal/ports/`** — interfaces only; the service depends only on these, not on any concrete adapters
- **`internal/adapters/`** — implementations of ports
- **`internal/core/service/`** — business logic; used by main, receives adapters as dependencies
- **`cmd/main.go`** — wires everything together and runs the CLI

Tests use `net/http/httptest` for the adapter and an inline `mockSearcher` struct for the service. No mocking libraries.
