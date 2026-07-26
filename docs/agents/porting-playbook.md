# Behavioral porting playbook

Use this workflow for issue-scoped implementation. Scale the ceremony to uncertainty and risk.

## 1. Establish current scope

1. If no issue number is given, shortlist locally with `python3
   scripts/agent-tools/issue_search.py '<milestone or scope terms>' --state open --milestone
   '<milestone>' --limit 20`. Do not list/search live issues or refresh the cache first. Query at most
   three plausible candidates live; use bounded live discovery only if the cache cannot supply one.
2. Fetch the known or shortlisted issue from `fatal10110/acis_golang`; read its current body,
   comments, labels, milestone, parent/child context, and state. Local snapshots are never status
   authority.
3. Read the relevant section of `aCis_gameserver/GO_REWRITE_PLAN.md` and the behavioral documents
   under `aCis_gameserver/docs/go-rewrite/`.
4. Search `.agent-cache/INDEX.md` and linked reports. Treat cached findings as hints until their
   evidence and revision are verified.
5. State the bounded deliverable and any adjacent subsystem that is deliberately out of scope.

Routine work following an established package pattern may use a short plan and proceed. Require
explicit design approval before introducing a new package boundary, public API, concurrency ownership
model, persistence representation, significant dependency, or major cross-system design.

## 2. Run one bounded investigation

Do not scatter a feature's research across many trivial agents or repeated broad reads.

- Use `rg` for known identifiers, constants, opcodes, filenames, and strings.
- Use grepai semantic search when the implementation location is unknown, and grepai trace for call
  relationships. Constrain path, output format, and result count.
- Use `gopls` for Go definitions, references, implementations, diagnostics, and call hierarchy.
- Use ast-grep for syntax-shaped matches or structural rewrites.
- Read only the returned ranges. Expand to a full file when control flow cannot be understood safely
  from targeted ranges.
- Do not use `find`, POSIX `grep`, or `sed` as a discovery loop. After one exact `rg`
  miss, switch to grepai, `gopls`, or ast-grep according to the question.
- Use `java-lookup` for one exact lookup. Use `java-researcher` once for a feature-level multi-file
  investigation. An ordinary issue gets at most one research agent.

The investigation reports observable behavior, not source architecture:

- inputs and preconditions;
- state transitions;
- outputs and packet order;
- every rejection branch;
- persistence effects;
- constants, formulas, rounding, and overflow;
- exact evidence paths and symbols;
- uncertainties and conflicts in the evidence.

Never paste complete methods or large source excerpts into reports.

## 3. Check the client-visible surface

For any client-visible behavior, inspect both inbound handlers and outbound sends before coding.
Identify opcodes, field layout, state gates, send order, broadcasts, and every early return. Read
[`../../internal/gameserver/network/AGENTS.md`](../../internal/gameserver/network/AGENTS.md) for the
packet-impact and silent-rejection contract.

If required behavior depends on a later system, document the exact dependency and integration point.
Do not leave an accepted opcode as a quiet no-op.

## 4. Design the native Go shape

Search the Go repository for existing domain types, helpers, constructors, fixtures, and package
patterns before creating anything. Reuse the smallest existing mechanism that holds.

Extract the required behavior, then design native Go packages and APIs. Do not preserve class graphs,
singleton patterns, getter walls, or control flow merely because the behavior was discovered there.
Keep interfaces at their consumers and keep network packages limited to orchestration.

Classify the task with [`process-skills.md`](process-skills.md). A routine established-pattern port is
Class B; behavior-critical work uses the concise Class C test-first flow; only a real architecture
choice or ambiguity needs the Class D design and approval flow.

## 5. Implement the bounded unit

- Keep shared datapack assets read-only. Correct the parser or consumer.
- Preserve integer width, byte order, formula order, rejection behavior, and persistence effects.
- When a dependency is not ported, do not hardcode its data or absorb the neighboring subsystem.
  Defer the member only when the remaining unit is still useful and correct; otherwise stop and fix
  the scope.
- Return errors from library code. Let composition roots decide whether boot-time failure is fatal.
- Search `cmd/loginserver/main.go` and `cmd/gameserver/main.go` for the relevant Fx provider, invoke,
  data load, listener, lifecycle hook, or runbook entry. Wire boot behavior when the component must be
  live; otherwise document why it is library-only.

## 6. Verify against the contract

- Non-trivial logic leaves the smallest standard-library test that would fail if the behavior
  regressed.
- Exact formulas use expected values produced by an independent reference probe.
- Loaders compare counts and representative field dumps from the same asset.
- Protocol uses byte fixtures or `cmd/packetdiff`; geodata uses `cmd/geoprobe` where applicable.
- Concurrent code gets a focused race test.
- Run focused checks during implementation, then the final gates in
  [`verification.md`](verification.md).

## 7. Resolve gaps immediately and audit before delivery

Do not postpone this workflow until implementation is complete. When a gap, deferral, divergence, or
out-of-scope item is first identified, pause and complete steps 1–5 before continuing. Repeat the
audit against the final diff before delivery.

For every implementation gap, deferral, divergence, or deliberately out-of-scope item that will
ship:

1. From the outer workspace root, run
   `python3 scripts/agent-tools/issue_search.py '<specific terms>' --state all --limit 10`. The cache
   search covers issue numbers, titles, bodies, comments, labels, states, and milestones; comments
   are evidence, not optional metadata.
2. Fetch each plausible match live with
   `gh issue view <number> -R fatal10110/acis_golang --comments --json number,title,body,comments,state,milestone,labels,url`.
   Cached content is discovery-only.
3. When a live issue covers the gap, reuse it. If its body or milestone is incomplete, amend it in
   place while preserving useful history; never create a duplicate. If it is closed but appears
   unresolved, inspect the closure reason and ask before reopening it.
4. When no issue covers the gap, create one containing: source issue or PR context; observed and
   expected behavior; evidence paths and symbols; why the work is deferred; dependency and Go
   integration point; bounded scope and explicit non-scope; acceptance criteria; verification; and
   related issue, PR, and specification links.
5. Reuse the current issue's milestone when the follow-up belongs to the same workstream. Otherwise
   inspect live open milestones and select the one whose documented scope owns the work. Ask instead
   of creating an unassigned or guessed issue when ownership is ambiguous.

Link the existing, amended, or created issue from the current issue or pull request and report its
number in the completion summary. Creating and amending required follow-ups is authorized by the
repository instructions; closing, reopening, or changing unrelated issues still requires explicit
authorization.

Record exactly one compact result in the pull request and final response:

- `Gap audit: no shipped gaps`; or
- `Gap audit: <gap> -> #<issue> (OPEN, comments checked, milestone <name>)` for every shipped gap.

Update `.agent-cache/` only for reusable confirmed findings that future work cannot recover cheaply
from the issue, pull request, code, or specifications. Update an existing report instead of creating
a duplicate.

## 8. Deliver issue implementations

Unless the user explicitly requests local-only work, an issue implementation is incomplete until it
is delivered. After all required gates pass, use a non-`main` branch (default `codex/` prefix), stage
only in-scope files, create a buildable commit, push it, and open a draft pull request against `main`.
Include tests and oracle evidence, the issue-closing keyword, and links for deferred gaps. If branch
safety, credentials, network access, or a required gate blocks delivery, report that exact blocker
instead of claiming completion. Never close the issue directly or merge the pull request without
explicit authorization.
