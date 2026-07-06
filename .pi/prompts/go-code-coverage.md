# Go Coverage Improvement Playbook

Run exactly one safe, deterministic coverage-improvement iteration for
this Go project.

## Objective

Increase statement and branch coverage for one cohesive target area
while preserving production behavior. Stop after one successful iteration.

## Agent Operating Rules

1. **Evidence-first.** Infer behavior from current source, tests, and build
   configuration. Do not guess.
2. **One-iteration scope.** Select one target area and stop after that
   iteration succeeds.
3. **Minimal safe diff.** Prefer test-only changes. Avoid broad refactors.
4. **Deterministic tests only.** Avoid flaky timing, environment, network, or
   SSH dependencies unless the project already provides stable utilities for
   them.
5. **No hidden tradeoffs.** If coverage gain requires risky behavior changes,
   stop and surface the tradeoff.
6. **Fail-safe workflow.** If validation fails, fix the tests or revert the
   failing change before finishing.

## Project Context

- Read the project's AGENTS.md and any `.go`-level agent instructions before
  changing or validating files.
- Identify the build command, test commands, and coverage tooling.
- Follow the project's test conventions: same-package testing, testify
  assertions, `*_test.go` files alongside source.
- Every `.go` file must start with the MIT copyright header block.
- Static analysis gate: `go vet ./...` (from `src/`).

### Coverage Commands

This project uses Go's built-in coverage tooling:

```bash
# Quick per-package coverage summary
cd src && go test -cover ./...

# Detailed function-level coverage report
cd src && go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# HTML visualization (open in browser)
go tool cover -html=coverage.out

# Coverage for a single package
cd src && go test -cover ./internal/<pkg>/
```

## Target Selection Priority

Choose targets in this order:

1. Packages with zero or very low coverage.
2. Small utility, helper, or configuration functions with clear behavior.
3. Functions with uncovered branches visible in the coverage report.
4. More complex functions only when scope is clearly bounded.

### Current Coverage Baseline (for reference)

The project's coverage distribution reveals high-value targets:

- `internal/sshrun` (~22%) — SSH client execution, script upload, result
  streaming, runner orchestration, ping logic. Many functions at 0% coverage.
- `internal/cli` (~47%) — CLI command definition, argument parsing, target
  resolution, run orchestration. Key functions at 0% and <30%.
- `internal/output` (~86%) — Text and JSON streaming formatters. Most gaps
  in edge cases and error paths.
- `internal/inventory` (~96%) — Already well-covered. Focus on remaining
  uncovered branches.
- `cmd/fleetsh` (~0%) — Argument preprocessing and `main`. Low-priority
  (thin wrapper over `cli.Execute`).

## Constraints

- Prefer adding or extending tests only. Test files must use the same
  package as the source (e.g., `package cli` in `internal/cli/root_test.go`).
- Do not change production code unless tests reveal a real bug and the user
  explicitly asks for a production fix.
- Preserve public API and behavior.
- Do not weaken `go vet` or test gates.
- Do not add dependencies unless clearly necessary and acceptable in the
  repository.
- Use `github.com/stretchr/testify` for assertions (`assert`, `require`).
- Follow Go table-driven test conventions for parameterized test cases.

## Iteration Workflow

### 1. Preflight

- Read the project's AGENTS.md for test conventions, build commands, and
  coding standards.
- Run `cd src && go test -cover ./...` to verify the current baseline.
- Note the target area's current coverage from the output.

### 2. Generate or Inspect Coverage

- Run `cd src && go test -coverprofile=coverage.out ./...` to generate the
  coverage profile.
- Run `go tool cover -func=coverage.out | grep <target-package>` to get
  function-level detail for the target.
- Optionally run `go tool cover -html=coverage.out` for visual inspection.

### 3. Select One Cohesive Target

- Choose one function or a tightly related small set of functions in the
  same file.
- Confirm real uncovered branches from the coverage report and source code.
- Prefer functions that are self-contained with clear input/output contracts.

### 4. Implement Tests

- Add tests to the existing `*_test.go` file for the target package, or
  create a new one following the same conventions.
- Match the existing style: testify `assert`/`require`, table-driven tests
  where appropriate, descriptive test names.
- Include the MIT copyright header if creating a new test file.
- Cover meaningful branches: success paths, error paths, edge cases,
  boundary values, empty/nil inputs.

### 5. Validate

- Run `cd src && go vet ./...` to ensure no static analysis regressions.
- Run `cd src && go test ./internal/<target-pkg>/` to verify the target
  package passes.
- Run `cd src && go test ./...` to ensure the full test suite has no
  regressions.
- Optionally re-run `go test -cover ./...` to confirm the coverage delta.

### 6. Report

Produce:

- Target selected and rationale.
- Files changed (test files only, unless a bug was found and fixed).
- Commands run with pass or fail results.
- Coverage delta observed: before → after for the target function(s).
- Remaining high-value uncovered branches in the target area.

## Coverage Opportunity Checklist

When creating tests, prioritize these Go-specific patterns:

- **Error return paths**: functions returning `(T, error)` — cover both the
  success and error branches.
- **Nil/zero-value inputs**: `nil` slices, maps, pointers, interfaces; empty
  strings; zero structs.
- **Boundary values**: empty slices vs nil slices, max/min int values,
  string length boundaries (empty, single character, very long).
- **Table-driven completeness**: add rows to existing table-driven tests
  for uncovered branches.
- **Exported API surface**: unexported helper functions called through
  exported entry points — ensure the public path exercises the private
  branches.
- **Error wrapping**: verify `fmt.Errorf("...: %w", err)` wrapping is
  exercised and error messages are correct.
- **Concurrency safety**: if the target uses goroutines, channels, or
  mutexes, ensure test coverage includes concurrent access patterns.
- **Context cancellation**: functions accepting `context.Context` — cover
  cancellation and deadline-exceeded paths where feasible.

## Stop Condition

Stop after one successful focused iteration, even if additional coverage
gaps remain.
