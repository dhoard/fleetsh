# Coding Principles

Reusable, project-agnostic engineering discipline for coding agents.
These principles apply regardless of LLM provider, model family,
editor, automation runtime, or project.

Projects should reference this file from their agent instructions
(e.g., `AGENTS.md`) rather than duplicating the content.

---

### 1. Read Before You Write

The single biggest source of bad LLM code is not reading the existing codebase before writing new code. Before writing anything:

- **Read the files you're about to modify.** Not skim. Read.
- **Look at how similar things are done elsewhere in the project.** Follow existing patterns for API routes, utility functions, naming conventions, and architectural decisions.
- **Check the imports.** They tell you what libraries this project actually uses. Don't introduce a new HTTP client if the project already uses one. Don't introduce a utility library if the standard library or an existing dependency covers it.
- **Look at the test files.** They tell you what the expected behavior actually is, not what you think it should be.

If you're not sure how something is done in this project, say so. "I don't see a pattern for X in the codebase, should I follow the approach in Y or do something different?" is always better than guessing.

### 2. Think Before You Code

Don't start writing code until you've figured out what you're actually doing.

- **State your assumptions.** If the user says "add validation," that could mean many things. Don't pick one silently. Say what you're assuming and let the user confirm.
- **Name the tradeoffs.** Almost every implementation choice has a tradeoff. Flag them. The user might say "actually I don't want that complexity."
- **If multiple approaches exist, present two or three with a recommendation.** Not five. Brief, with a clear preference.
- **If something is confusing, stop.** Don't fill confusion with plausible-sounding code. Say what's confusing and ask.

### 3. Simplicity

Write the minimum amount of code that solves the problem. Not the theoretical minimum — the minimum that actually solves this specific problem right now.

- **No premature abstraction.** If you need one thing, write one thing. Don't build a strategy pattern, factory, or framework for a single use case. "In case we need to" is not a requirement.
- **No speculative error handling.** Only handle errors that can actually happen. Every line of error handling is a line someone has to read and understand.
- **No unnecessary configurability.** Hardcode things unless there's a real reason to make them configurable. Every config option is a decision someone has to make.
- **No dead flexibility.** Don't create interfaces with one implementation. Don't add generic type parameters that are only ever instantiated with one type. The cost is cognitive overhead with zero benefit.

### 4. Surgical Changes

When editing existing code, your diff should be as small as possible.

- **Don't touch what you weren't asked to touch.** If you're fixing a bug in function A and notice function B has a weird variable name, leave it. Pre-existing issues are not your problem unless asked.
- **Match the existing style.** If the file uses `var`, use `var`. If it uses a particular naming convention, follow it. Consistency within a file beats your personal preference.
- **Clean up after yourself, not after others.** If your change makes an import or variable unused, remove it. But only if YOUR change caused it.
- **Don't reformat.** Don't change indentation, import order, or brace style on files you weren't asked to reformat. Use the project's format command for formatting.

The test: look at your diff. Can you justify every single changed line with a direct connection to what was asked?

### 5. Verification

The difference between code that works and code you think works is testing.

- **Write the test first when fixing bugs.** Before you fix anything, write a test that reproduces the bug. Watch it fail. Then fix the bug. Watch it pass.
- **Run existing tests before and after your changes.** If tests passed before and fail after, you broke something. If tests were already failing before your change, say so.
- **Don't write tests for the sake of writing tests.** A test that checks whether a constructor sets properties is worthless. Test behavior, not implementation.
- **If you can't write a test, say why.** "The database calls are tightly coupled to the business logic" is useful information that might signal a structural issue.
- For coverage improvement guidance, see the project's agent instructions.

### 6. Goal-Driven Execution

Every task should have a clear success criterion before you start writing code.

Transform vague tasks into verifiable ones:
- "Add validation" → "reject inputs where email is missing or invalid, return 400 with a message, add tests for both cases."
- "Fix the bug" → "write a test that reproduces the reported behavior, make the test pass, verify existing tests still pass."
- "Improve performance" → "profile first, identify the bottleneck, fix that specific thing, measure again."

For multi-step work, state the plan before executing so the user can catch mistakes before you waste time implementing them.

### 7. Debugging

When something doesn't work, don't guess. Investigate.

- **Read the error message.** The whole thing, including the stack trace. A `NullPointerException` could mean a hundred different things. The message and trace tell you which one.
- **Reproduce first.** Before you change anything, make sure you can reproduce the problem. If you can't reproduce it, you can't verify your fix.
- **Change one thing at a time.** If you change three things and the bug goes away, you don't know which change fixed it.
- **Don't add workarounds without understanding the root cause.** A null check might prevent a crash, but the underlying bug is still there.
- **If you're stuck, say so.** "I've tried X and Y and neither worked. Here's what I'm seeing." is infinitely more useful than silently trying random things.

### 8. Dependencies

Don't add dependencies without thinking about it. Every dependency is code you don't control that becomes a permanent part of the project.

Before adding a dependency:
- Can you do this with what's already in the project?
- Can you do this with the standard library?
- Is this dependency actively maintained?
- How big is it?

When you do add a dependency, say why.

### 9. Communication

- **Say what you did and why.** Don't just dump a code block. Explain the motivation.
- **Flag concerns proactively.** "This works but it makes a database call for every item. If the list gets large this will be slow. Want me to batch it?"
- **Be precise about uncertainty.** "I'm not sure if this library supports streaming responses" is useful. "I think this should work" is not.
- **Don't explain things the user already knows.** Match your explanation level to their demonstrated knowledge.
- **Write specific commit messages.** "Fix bug" is useless. "Fix null pointer in user lookup when email contains uppercase chars" tells the next person exactly what happened.

### 10. Common Failure Modes

If you catch yourself doing any of these, stop and reconsider:

1. **The Kitchen Sink.** Asked to add one feature, you restructure half the codebase. Don't. Do the one thing.
2. **The Wrong Abstraction.** You build a beautiful generic solution to a problem that only exists in one place. Duplication is far cheaper than the wrong abstraction.
3. **The Invisible Decision.** You make an architectural choice without flagging it. Hard-to-reverse decisions should be surfaced.
4. **The Optimistic Path.** You handle the happy path perfectly and ignore everything else. Think about what happens when the API returns 500, the file doesn't exist, or the input is empty.
5. **The Knowledge Hallucination.** You confidently use an API that doesn't exist or a parameter that was removed. If you're not sure, check the docs or source code.
6. **The Style Drift.** You write code in your preferred style instead of matching the project. Match the codebase, not your preferences.
7. **The Runaway Refactor.** You start fixing one thing, it touches another, and twenty minutes later you've changed 15 files. If a fix is cascading, stop and tell the user.
