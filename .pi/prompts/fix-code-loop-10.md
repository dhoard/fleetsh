# Fix Code Loop × 10 — All Modules

Run ten complete loops of analysis → design → specification →
implementation across all modules in the project. Execute each phase in
sequence within each loop, resolve any issues found, and repeat until ten
loops have completed or no issues remain. Produce only the final code
changes and supporting documents.

<!-- ARGUMENTS: user-provided input such as file paths, package names, or scope constraints -->

## Execution Boundary

This prompt orchestrates ten consecutive four-phase loops across all
project modules:

1. `analyze-code.md` — deep correctness analysis of every module.
2. `create-design-plan.md` — design plan to resolve confirmed issues.
3. `create-implementation-spec.md` — implementation specification from the
   design plan.
4. `implement-spec.md` — implement the specification.

Within each loop, each phase must complete fully before the next phase
begins. The handoff from each phase to the next is the document produced:
findings → design plan → implementation spec → code changes.

After each loop completes, begin the next loop immediately using the
updated codebase as the new target. Each loop's analysis must re-examine
the full project, not just the files changed in previous loops.

## Objective

Identify and resolve correctness issues across all project modules through
ten consecutive deep-analysis loops. Each loop independently analyzes the
current state of every module, produces a design plan, converts it into a
concrete implementation specification, and implements the fixes — then
repeats on the improved codebase.

## Input

The full project. Discover all modules automatically by inspecting the
build system:

- **Maven**: parse the root `pom.xml` for `<modules>` declarations. Every
  listed module is in scope. Skip modules with `<packaging>pom</packaging>`
  unless they contain source code.
- **Gradle**: inspect `settings.gradle` or `settings.gradle.kts` for
  included subprojects.
- **Multi-language projects**: include all source-bearing modules
  regardless of language, applying the project's agent instructions for
  each language's conventions.

If no build system can be located or no modules are found, abort and report
the blocker.

Optionally, a scope constraint: specific modules, packages, classes, or
issue references to narrow the analysis. If provided, the same scope
constraint applies to every loop. If omitted, analyze all modules.

## Loop Counter and Tracking

Maintain a loop counter from 1 to 10. At the start of each loop, report:

- **Loop N of 10** (where N is the current loop number).
- Number of loops remaining after this one.
- Modules in scope (list them by name).
- Total issues found and resolved across all previous loops, broken down
  by module.

## Per-Loop Phase 1: Analyze Code

Execute `analyze-code.md` for every module in scope. Analyze modules in
any order but complete the full analysis of one module before starting the
next.

### Deliverables

For each module, produce:

- Complete findings report with severity levels: Critical, High, Medium,
  Low.
- Confirmed bugs tied to specific files, methods, and observable incorrect
  behavior.
- A "needs confirmation" section for suspicious findings lacking sufficient
  evidence.

Aggregate findings across all modules into a single per-loop findings
report.

### Decision Gate

After producing the aggregated findings report, determine which issues to
resolve in this loop, considering all modules together:

- **Prioritize across modules**: Critical and High issues in any module
  take priority over Medium and Low issues in any other module.
- **If Critical or High issues exist anywhere**: resolve all Critical and
  High issues regardless of which module they're in. Resolve Medium and Low
  issues at your discretion based on severity, fix complexity, and risk of
  regression.
- **If no Critical or High issues exist anywhere**: resolve the most
  impactful Medium and Low issues across all modules. Do not skip the
  remaining phases — there is always something to improve.
- **If no issues are found in any module**: produce a brief summary stating
  that analysis found no correctness issues. Skip the remaining phases for
  this loop and proceed immediately to the next loop.

Report the number of issues selected for resolution in this loop, broken
down by module, before moving to Phase 2.

## Per-Loop Phase 2: Design Plan

Execute `create-design-plan.md` with the selected findings from this loop's
Phase 1 as the problem statement. The design plan must cover fixes across
all affected modules and address any cross-module interactions (shared
types, API boundaries, dependency ordering).

### Deliverables

- Design plan document written to
  `plans/fix-<description>-loop<N>.md` covering all selected issues
  across all modules.
- Concrete approach, tradeoffs, test strategy, and acceptance criteria for
  each fix.
- Explicit note of any cross-module implications or ordering requirements.

## Per-Loop Phase 3: Implementation Specification

Execute `create-implementation-spec.md` with the design plan from this
loop's Phase 2 as input.

### Deliverables

- Implementation specification document written to
  `plans/fix-<description>-loop<N>-spec.md` (or append `-spec` to the
  design plan path).
- Ordered implementation steps beginning with reproduction tests for every
  confirmed bug.
- Steps ordered to respect cross-module dependencies (e.g., fix shared
  types before dependents).
- Exact files, method signatures, behavior rules, and acceptance criteria.
- Module name prefix on every file path to disambiguate across modules.

## Per-Loop Phase 4: Implement Specification

Execute `implement-spec.md` with the implementation specification from this
loop's Phase 3 as input.

### Deliverables

- Reproduction tests that fail before the fix and pass after.
- The smallest focused source changes that resolve every selected issue.
- Passing test suite across all modules with no regressions.
- All acceptance criteria from the specification satisfied.

## Post-Loop Git Commit

After each loop completes (all four phases finish successfully), commit
the changes to git and push to the remote:

- Add all new, changed, and deleted files to the staging area.
- Create a signed-off commit:

    git commit -s -m "fix: Misc fix"

- Push the commit to the remote:

    git push

This ensures every loop's work is captured as an independent commit and
pushed before the next loop begins.

## Phase Handoffs

- The deliverable from each phase is the input to the next.
- Wait for each phase to complete before starting the next.
- Start each loop's Phase 1 by reading every module's source files in their
  current state.

## Stop Conditions

Stop the entire process and report the blocker if:

- **Phase 1**: Any module's files cannot be read, or the code cannot be
  understood sufficiently to identify contracts. Report which module is
  blocked. Analysis may still proceed on readable modules if the blocker is
  isolated, but flag the gap explicitly.
- **Phase 2**: The selected issues are too vague to produce a concrete
  design, or required design decisions remain unresolved.
- **Phase 3**: The design plan is too incomplete to convert into a
  specification, or the expected failing behavior cannot be identified.
- **Phase 4**: The specification is ambiguous, contradicts project
  conventions, or describes unreproducible issues.

Early termination: If two consecutive loops find no issues in any module
(Phase 1 produces empty findings reports for all modules), stop early.
Report the total number of loops completed and summarize all fixes applied.
Do not fabricate problems to fill remaining loops.

## Completion Criteria

The entire ten-loop process is complete when:

- All ten loops have been attempted (or early termination was triggered).
- Each completed loop produced a findings report covering all source files
  in every module at that point in time.
- Each loop with issues produced a design plan document, an implementation
  specification with reproduction tests and ordered steps, passing
  reproduction tests, the minimal source changes to resolve every selected
  issue, and a clean full test suite with no regressions across any module.
- The final codebase has been validated by the last loop's Phase 1
  analysis, confirming no remaining Critical or High issues in any module
  (and preferably no issues at all).
- Every document and code change follows the project's agent instructions
  for formatting, validation, and commit conventions.

## Final Report

In the final response, report for each loop that had issues:

- Loop number and the number of issues found and resolved, broken down by
  module.
- A summary of each fix applied in that loop, indicating the module it
  belongs to.
- The paths of all documents produced (findings report, design plan,
  implementation specification).
- The changed source and test files, with module name prefixes.
- The validation commands run and their results, per module.

Then report the aggregate summary:

- Total loops completed.
- Total issues found and resolved across all loops, broken down by module.
- Final state of the codebase (no remaining Critical/High issues in any
  module, or clean analysis).
- All document paths and changed files across the entire process.
