# AGENTS.md

Instructions for coding agents working in this repository.

## Identity

- **Project**: fleetsh
- **Purpose**: Run shell commands or scripts across a fleet of servers over SSH
- **Language**: Go 1.25
- **Module path**: `github.com/dhoard/fleetsh`

## General Coding Principles

Reusable engineering discipline for coding agents. See
`.pi/prompts/coding-principles.md`.

## Build, Test, and Validation Commands

All commands are run from the project root unless noted otherwise.

```bash
# Run tests (from src/)
cd src && go test ./...

# Run vet (from src/)
cd src && go vet ./...

# Build binary (from src/)
cd src && go build -o ../bin/fleetsh ./cmd/fleetsh

# Full local build: test + vet + build
./build.sh

# Final verify: test + vet + cross-compile + package (requires GoReleaser)
./build-and-package.sh
```

No formatter (`go fmt`, `gofumpt`, `golangci-lint`) is currently configured as a
build gate. `go vet` is the only static analysis step.

## Source Layout

```
src/
  cmd/fleetsh/         # main entry point, argument preprocessing, compile smoke test
  internal/
    cli/               # cobra command definition, flag parsing, target resolution, run logic
    inventory/         # inventory file parsing, host/group model, pattern matching
    sshrun/            # SSH client execution, SCP upload, streaming results, ping
    output/            # text and JSON streaming output formatters
```

## Coding Conventions

- Every Go source file starts with the MIT copyright header block (14-line
  comment).
- Package naming follows Go conventions: lowercase, single word where possible.
- Exported types use PascalCase; unexported use camelCase.
- Error wrapping uses `fmt.Errorf("...: %w", err)`.
- Exit codes use the `*exitError` sentinel pattern centralized in
  `cli.Execute()`.
- Flags use `spf13/cobra` with `SilenceErrors` and `SilenceUsage` set.
- Host display names prefer alias over hostname (`Host.DisplayName()`).
- Inventory naming constraint: aliases and groups must match `[a-zA-Z0-9_-]`.
- Reserved word: `summary` cannot be used as an alias or group name.

### Copyright Header

Every `.go` file must start with the MIT license block:

```go
//
// Copyright (c) 2026-present Douglas Hoard
//
// Permission is hereby granted, free of charge, ... [14 lines total]
//
```

## Test Conventions

- Tests live alongside source in `*_test.go` files.
- Same-package testing is used (e.g., `package cli` in `cli/root_test.go`).
- Assertions use `github.com/stretchr/testify`.
- The `main` package must have a compile smoke test in `main_test.go`.

## Key Dependencies

| Dependency | Role |
|-----------|------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/go-ping/ping` | ICMP ping |
| `github.com/stretchr/testify` | Test assertions |
| `golang.org/x/net` | Network utilities (indirect) |
| `golang.org/x/sync` | Concurrency primitives (indirect) |

## Exit Code Contract

- `0` — all hosts succeeded
- `1` — one or more hosts failed
- `2` — CLI, config, or inventory error

## Git Workflow

- Branch: `main`
- CI: GitHub Actions on push/PR to `main`
- Commits should follow conventional commits where practical
- Signed-off commits preferred

## Constraints

- OpenSSH client (`ssh` and `scp`) must be available at runtime
- No password authentication (key/agent only)
- GoReleaser required for release packaging (v2.13.0 in CI)
- Go 1.25 minimum
