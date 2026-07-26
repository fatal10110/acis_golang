# Process skill selection

Use process skills only when their expected reduction in mistakes, retries, or human intervention
exceeds their context and subscription cost. Repository instructions override generic skill triggers;
checking this task classification satisfies the initial skill-selection requirement.

## Task classes

### Class A — trivial and mechanical

Use for comments, formatting, exact mappings or supplied vectors, known constants, mechanical
renames, and deterministic one-file edits following an established pattern.

Inspect the exact target, make the smallest change, run the focused check, and verify the diff.
Apply Ponytail Lite implicitly. Do not load brainstorming, writing-plans, TDD,
subagent-driven-development, parallel-agent, or worktree skills.

### Class B — routine issue-scoped implementation

Use for established packet, loader, method, formula, handler, and ordinary behavior-port patterns.

1. Read the live issue, specification, cache, and relevant local pattern.
2. Delegate substantial reference research only when needed.
3. Keep the plan to roughly 3–7 steps in the active task; do not create a plan document by default.
4. Apply Ponytail Lite and run focused tests while implementing.
5. Use TDD only for non-trivial or regression-prone behavior.
6. Invoke verification-before-completion near the end.

No separate architecture approval, worktree, subagent-driven development, or formal design document
is required when no architecture choice exists.

### Class C — behavior-critical implementation

Use for packet codecs or branching handlers, rejection paths, parsers, exact formulas, persistence,
economy or trade, state transitions, concurrency, lifecycle, and regression-testable bug fixes.

Define observable behavior, use TDD or the equivalent concise test-first cycle, confirm the smallest
meaningful failing test, implement the smallest correct change, run focused checks, and finish with
verification-before-completion. Refactor only when it improves the required solution. Request a
separate review only when risk is high; do not build speculative test infrastructure.

### Class D — architectural or ambiguous

Use for a new package boundary, public API, concurrency ownership model, persistence representation,
significant dependency, lifecycle framework, major cross-system design, redesign, or conflicting
behavioral evidence.

Complete bounded research, invoke brainstorming, present 2–3 realistic approaches, and apply
Ponytail Full to remove unnecessary abstraction and scope. Obtain approval for the direction, use
writing-plans when implementation has dependent stages, apply TDD to behavior-critical portions,
request independent review, and run complete verification. Several touched files alone do not make
a task Class D; a design document belongs here only when it captures a real decision.

### Class E — unclear failure or debugging

Use when a failure's root cause is unknown, intermittent, concurrent, contradicted by evidence, or
still present after an attempted fix.

Invoke systematic-debugging. Reproduce, gather evidence, test one hypothesis at a time, add a
regression test after the defect is understood, apply the root-cause fix, then run focused and final
verification. Do not brainstorm or refactor before establishing the cause.

## Superpowers selection

- `verification-before-completion` is required before claiming implementation work complete, but
  load it near the end rather than at task start. Evidence must come from commands and the diff.
- `systematic-debugging` is required for Class E.
- `receiving-code-review` is required only when feedback is ambiguous, conflicts with evidence, may
  break required behavior, or would add significant scope.
- Use TDD for Class C. Skip it for comments, formatting, mechanical renames, deterministic generated
  mappings, trivial covered wiring, or checks that duplicate compiler behavior. Briefly justify any
  skip for non-trivial behavior.
- Use brainstorming only for Class D. Use writing-plans only when dependent sequencing, coordinated
  package changes, cross-session continuation, or an approved multi-stage design needs a durable
  plan.
- Request code review only for high-risk changes or a broad implementation before merge.

By default, do not invoke subagent-driven-development, dispatching-parallel-agents,
using-git-worktrees, finishing-a-development-branch, or repeated reviewer agents.

Parallel agents are justified only when deliverables are independent and bounded, agents will not
write the same package or files, repeated exploration will not multiply, and the expected wall-clock
gain justifies subscription use. Maximum default concurrency is two threads. Use subagent-driven
development only for an approved plan with several independent implementation tasks. Use a worktree
only for genuinely parallel branches, necessary isolation from unrelated changes, a risky experiment,
or an explicit user request.

Do not delegate test design or the failing-test phase separately from implementation; those phases
share the same behavioral context. If the user explicitly requests delegation, use a fresh-context
agent only for bounded, self-contained research or noisy verification, and require a result of at
most ten lines containing conclusions, evidence paths, and failures rather than raw output.

## Ponytail selection

Ponytail governs implementation minimalism, never behavioral scope.

- **Lite:** default for ordinary implementation and review; apply it without a separate announcement
  turn. Prefer YAGNI, the smallest correct diff, existing patterns, standard library where complete,
  and no speculative interfaces, frameworks, indirection, extensibility, Java abstractions, or
  unrelated refactoring.
- **Full:** use for new packages, dependencies, public APIs, major abstractions, overgrown proposals,
  custom infrastructure choices, or work expanding materially beyond its required behavior.
- **Ultra:** use only when the user explicitly requests aggressive simplification.

Minimalism must not remove packet output or order, rejection responses, state gates, persistence,
validation, security, concurrency safety, cancellation, exact numeric or compatibility behavior,
required errors, fixtures, regression tests, or milestone integration. When size and completeness
conflict, completeness wins; find the smallest complete implementation.

## Precedence

1. User instructions and behavioral specification.
2. Observable compatibility requirements.
3. Repository architecture and safety requirements.
4. Required verification.
5. Selected Superpowers process guidance.
6. Ponytail minimalism.
7. Generic tool and skill defaults.

## Anti-overhead safeguards

- Do not invoke a skill only to announce it or reproduce its body in a plan.
- Do not create ritual design documents, approval gates, reviewer agents, worktrees, or abstractions.
- Do not make a plan longer than the expected implementation or spawn an agent for one obvious command.
- Reuse `.agent-cache/` reports; do not repeat research across brainstorming, planning, coding, and review.
- Keep research context separate from implementation context.
- Bound test output at the command boundary; do not retain complete passing logs in the conversation.
- Run focused verification during development and the complete applicable gate once before completion.

## Task classification template

Use this short record for Classes C–E or when classification is unclear; omit it for Class A.

```text
Class:
Risk factors:
Established pattern:
Behavioral contract:
Architecture decision required:
Unknown failure cause:
Subagent justified:
Selected skills:
Skipped skills and reason:
```
