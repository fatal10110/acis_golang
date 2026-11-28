# 0013. Production code describes this Go system only

- **Status:** Accepted
- **Date:** in force before the 2026-09 actor-model plan, which lists it as an invariant
- **Sources:** `internal/provenance`,
  [actor-model-degoifying-plan.md](../actor-model-degoifying-plan.md) (invariants),
  [agents/go-style.md](../agents/go-style.md) (Comments and documentation)

## Context

The code is a port. While porting, it is tempting to write comments such as "same as
`L2PcInstance.java` line 1200", or to keep Java class names. Those comments stop being useful
quickly: the Java file moves, the reader may not have the Java sources, and the comment explains
where code came from instead of what it does and why.

## Decision

Comments and names in production Go code describe this Go system. Comments state the behavior and
the rules the code keeps. Names say what a thing does, not where it came from. Production code does
not cite Java classes, files or line numbers.

Game terms stay as they are (Npc, Playable, Servitor, Cubic, Henna), because they are the domain
language, not Java artifacts.

`internal/provenance` enforces the rule. Its test scans every non-test Go file and fails on
markers that point at another implementation.

## Consequences

- A reader understands the code from the code alone.
- The link between a Go behavior and its reference lives in tests, issues, pull requests and the
  [porting playbook](../agents/porting-playbook.md), where it belongs.
- Contributors must rephrase porting notes as behavior before committing.
- Test files and documentation may mention the reference.

## Alternatives considered

- **Allow reference citations in comments.** Handy during porting, but they go stale and make the
  code depend on sources most readers do not have.
