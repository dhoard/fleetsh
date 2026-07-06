# Go Performance Review Playbook

Review the provided code, profile data, benchmark output, or performance
concern and recommend optimizations that preserve behavior and improve
measurable outcomes.

## Objective

Identify performance issues with credible impact, recommend specific
changes, and flag any correctness or maintainability risks those changes
introduce. Do not optimize without evidence.

## Input

The code, profile results, benchmark output, or performance concern to
review. Specify files, packages, or a target description.

## Priorities

1. Correct code first.
2. Clean, maintainable code second.
3. Fast code third.

## Rules

- Do not change behavior unless explicitly asked.
- Identify performance improvements only when justified by code, benchmarks,
  profiling data, workload details, or clear hot-path reasoning.
- Prefer simple, readable optimizations over clever or fragile ones.
- Call out any optimization that could reduce readability, safety, or
  maintainability.
- Avoid premature optimization. Ask for benchmarks, profiling data, workload
  details, or hot paths when needed.
- Do not rewrite large sections unless there is a clear benefit.
- Preserve the project's Go version compatibility (`go 1.25` in go.mod) and
  public APIs unless there is a strong reason not to.
- Prefer standard library APIs unless a dependency is already present.
- Consider:
  - **Goroutine leaks**: unclosed channels, missing cancellation, waitgroup
    mismatches.
  - **Channel buffer sizing**: too small causes unnecessary blocking; too
    large wastes memory.
  - **Synchronization**: `sync.Mutex` vs `sync.RWMutex` vs `atomic` (choose
    the lightest that meets the invariant).
  - **Escape analysis**: values that allocate on the heap when they could
    stay on the stack (e.g. captured-by-reference in closures, interface
    boxing).
  - **GC pressure**: frequent allocations in hot paths, large
    per-goroutine allocations, missing `sync.Pool` opportunities.
  - **Memory**: slice pre-allocation (`make([]T, 0, cap)`), map sizing
    hints, `strings.Builder` growth.
  - **I/O**: buffered vs unbuffered readers/writers, `bufio.Scanner` vs
    `bufio.Reader`, connection reuse, DNS resolution frequency.
  - **Concurrency**: semaphore patterns, worker pools, errgroup,
    context propagation and timely cancellation.
  - **exec.Cmd**: process lifecycle, pipe management, `Wait` after `Start`.
  - **Algorithmic complexity**: avoid O(n²) where O(n) or amortized O(1) is
    expected.
- Flag data races, deadlocks, or goroutine lifetime risks.
- Follow the language idiom guardrails in the project's `.go`-level agent
  instructions and AGENTS.md.

## Project-Specific Context

This is `fleetsh` — a fleet SSH runner. Key performance surfaces:

- **SSH execution** (`internal/sshrun/client.go`): each host launches an
  `exec.Cmd` for `ssh` and `scp`. Process startup cost, argument
  construction, and `bufio.Reader`-based line streaming are in the hot path.
- **Parallel runner** (`internal/sshrun/runner.go`): semaphore-based
  concurrency limits via `sem chan struct{}`. Channel buffer sizes (64
  events) and atomic fail-fast checks sit on the critical path.
- **Ping** (`internal/sshrun/runner.go`): ICMP fallback to TCP ping over
  multiple ports. TCP dialer reuse and DNS resolution patterns matter at
  fleet scale.
- **Inventory** (`internal/inventory/`): in-memory host/group resolution
  with regex matching. Large inventories can stress pattern-matching
  efficiency.
- **Output streaming** (`internal/output/`): text and JSON formatting of
  real-time event streams.

## Output Format

For each review, provide the following sections.

### 1. Summary of Likely Performance Issues

A concise list of the most impactful performance concerns found.

### 2. Correctness Risks

Any correctness risks introduced by each proposed change. If a change
introduces no correctness risk, state that explicitly. Pay special
attention to concurrency — goroutine lifetimes, channel closure order,
and context cancellation.

### 3. Readability and Maintainability Impact

How each change affects code clarity and maintainability.

### 4. Recommended Changes

For each recommendation:

- Before and after code when useful.
- Why the change is faster or more efficient (fewer allocations, less
  blocking, lower GC pressure, better parallelism, etc.).
- Tradeoffs, explained clearly.
- Confidence level: High, Medium, or Low.

### 5. Benchmarking or Profiling Suggestions

How to measure the impact of the recommended changes. Include specific
commands and approaches:

- `go test -bench=. -benchmem ./...` for benchmark comparisons
- `go test -race ./...` for data race detection
- `go test -cpuprofile=cpu.out -memprofile=mem.out -bench=.` for pprof
- `go tool pprof -http=:8080 cpu.out` for interactive flame graphs
- `go build -gcflags="-m"` for escape analysis output
- `benchstat` for comparing benchmark runs before and after
- Stress-testing with fleet-scale workloads (many hosts, large output)

### 6. No-Change Cases

Explicitly state when no change is recommended and why.
