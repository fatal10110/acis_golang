# Agent rules for acis_golang

This repository is a native Go implementation of a game server whose observable behavior is
specified elsewhere. Match the contract; do not reproduce another implementation's internal
structure.

## Working directory contract

- Launch and keep the shell at the outer `acis_public/` workspace containing `acis_golang/`,
  `aCis_gameserver/`, and `aCis_datapack/`. Do not persistently `cd acis_golang` or retry commands by
  alternately adding and removing that prefix.
- Shell paths and command examples are outer-root-relative. Use `git -C acis_golang ...` for Git and
  `rtk go -C acis_golang ...` (or `go -C acis_golang ...`) for Go. Root tools and caches remain
  `scripts/...`, `.claude/...`, and `.agent-cache/...`.
- Bare Go paths in prose, such as `internal/...` or `cmd/...`, identify locations inside
  `acis_golang/`; prefix them with `acis_golang/` for shell file operations unless the command uses
  `-C acis_golang`.

## Authority and scope

- Observable behavior is authoritative: wire bytes and ordering, formulas and overflow, data-file
  interpretation, persistence effects, game rules, state transitions, and rejection behavior.
- Write idiomatic Go. Extract behavior from the reference material, then choose Go packages, types,
  interfaces, ownership, and control flow for this codebase.
- Before implementing an area, read the relevant behavioral specification under
  `aCis_gameserver/docs/go-rewrite/` and the applicable section of
  `aCis_gameserver/GO_REWRITE_PLAN.md`. Inspect exact source behavior only where the specification
  is incomplete or an oracle fixture is needed.
- Reuse the XML, HTML, geodata, SQL, and configuration assets under `aCis_datapack/` unchanged.
  Fix readers and consumers instead of changing the shared contract.
- Keep production identifiers, comments, and commit messages free of reference-implementation
  provenance. Such evidence belongs in issues, pull requests, specifications, tests, or
  `.agent-cache/`.

## Issue state and shared research

- GitHub is the only authority for issue state. When an issue number is known, query it directly and
  read the current body and comments. When no number is given, including "next issue" prompts,
  shortlist locally with `python3 scripts/agent-tools/issue_search.py '<milestone or scope terms>'`
  before GitHub; do not list or search all live issues or refresh the cache first. Verify at most
  three plausible candidates live with comments, and use bounded live discovery only if the cache is
  missing, finds none, or every candidate is stale. Cached status is never authoritative.
- Before accepting any implementation gap, deferral, divergence, or out-of-scope item, search the
  cached issue titles, bodies, comments, labels, states, and milestones with
  `python3 scripts/agent-tools/issue_search.py '<terms>'`, then fetch plausible matches live with
  comments. Amend a matching issue's body or milestone instead of creating a duplicate. This is a
  blocking gate: run it when the gap is first identified, before continuing implementation, and
  audit the final diff again before opening the pull request. Do not wait for completion or a user
  reminder.
- If no live issue adequately covers the gap, create a focused follow-up with the source context,
  evidence, dependency and integration point, bounded scope, acceptance criteria, verification, and
  related links. Assign the clearly relevant open milestone; ask instead of guessing when ambiguous.
  Creating or amending these follow-ups is authorized as part of implementation, but closing,
  reopening, or changing unrelated issues is not.
- Issue implementation includes delivery unless the user explicitly requests local-only work. After
  all required gates pass, use a non-`main` branch, stage only in-scope files, commit, push, and open
  a draft pull request against `main` with the issue-closing keyword and verification evidence. Do
  not merge the pull request or close the issue directly without explicit authorization.
- Search `.agent-cache/INDEX.md` and relevant reports before fresh investigation. Store only
  verified, reusable findings; never store chat transcripts, raw logs, large source excerpts, or
  stale status snapshots.
- Record evidence paths and symbols, the revision inspected, uncertainties, and the distinction
  between confirmed behavior and inference.

## Always-loaded engineering rules

- Keep game rules, validation, state mutation, and persistence decisions in domain packages;
  network handlers only decode, resolve context, call domain behavior, and map outcomes to packets.
- Give every shared mutable value an explicit owner through one goroutine or a named synchronization
  mechanism, and do not launch unowned goroutines.
- Define focused interfaces at consumption points, return concrete types, and do not add speculative
  abstractions.
- Classify each lookup before running it: `rg --files` for filenames, `rg` for known exact text,
  grepai for behavior or an unknown location, `gopls` for Go symbols/relationships, ast-grep for
  syntax structure, and the native file-read tool for a known range. Do not use `find`, `grep`, or
  `sed` for ordinary discovery, source search, or file reading. After one failed exact
  `rg` attempt, switch to grepai or a symbol/structural tool instead of trying more shell scans.
- Run the smallest focused check during development and the complete required gates before declaring
  implementation work complete; configuration-only work uses syntax, link, and consistency checks.
- Done means specified behavior and rejection paths match, packet impact is covered, exact contracts
  have independent evidence, boundaries and ownership remain sound, required checks pass, every
  deferral has a verified milestone-assigned follow-up issue, the diff is scoped, and an issue-closing
  draft pull request is open for issue implementation unless the user opted out. The final response
  and pull request must contain `Gap audit: no shipped gaps` or map each shipped gap to its verified
  follow-up issue number, checked comments, and milestone.

## Process skills

Superpowers and Ponytail are available but are not mandatory rituals.

Repository instructions override generic skill triggers. Do not invoke a process skill merely
because it exists or claims universal applicability. Classify the task using
[`docs/agents/process-skills.md`](docs/agents/process-skills.md) and load only the skills justified by
its risk and uncertainty.

- Apply Ponytail Lite implicitly to routine work; minimalism cannot remove required behavior.
- Use TDD for non-trivial behavioral contracts and bug fixes.
- Keep test design, the smallest failing test, implementation, and focused verification in the main
  agent. Do not split TDD from implementation merely to shed context.
- Use systematic-debugging when the cause of a failure is unclear.
- Use brainstorming and writing-plans only for architectural, ambiguous, or dependent multi-stage
  work.
- Use verification-before-completion before claiming implementation work complete.
- Answer inline when `rg`, `gopls`, or a targeted read resolves it in a few calls; delegation is
  never the default. When delegation is warranted, use the bounded named agents rather than a
  generic search agent: `java-lookup` for one exact fact, `java-researcher` for one multi-file
  behavior investigation. They cap model, turns, and report size, so they cost less than
  `general-purpose` or `Explore` for the same question. Never use a generic search agent for aCis
  Java reference research.
- Spawn `port-reviewer` only when the user explicitly requests review. It is the most expensive
  agent here; do not run it speculatively, in parallel, or twice on the same change.
- At most one research agent per ordinary issue and at most two concurrent agent threads. Reserve
  delegation for self-contained work whose raw output would otherwise be large; return at most ten
  lines of conclusions, evidence paths, and failures, never raw logs or full file contents. Do not
  use agent teams, subagent-driven development, or repeated reviewers.

Behavioral parity, packet completeness, persistence correctness, concurrency safety, and required
tests take precedence over process convenience and code-size reduction.

## Scoped operating guides

Load only the guide needed for the current work:

- [`docs/agents/go-style.md`](docs/agents/go-style.md): Go design, naming, types, errors, exact numeric
  behavior, concurrency ownership, dependencies, file shape, and composition-root wiring.
- [`docs/agents/porting-playbook.md`](docs/agents/porting-playbook.md): issue selection, bounded
  reference research, design-approval criteria, implementation sequence, and deferrals.
- [`docs/agents/tooling.md`](docs/agents/tooling.md): deterministic `rg`, grepai, `gopls`, `rtk`, `jq`,
  and ast-grep selection with verified bounded commands and fallbacks.
- [`docs/agents/verification.md`](docs/agents/verification.md): focused checks, final gates, independent
  fixtures, configuration-only validation, and the definition of done.
- [`docs/agents/model-policy.md`](docs/agents/model-policy.md): evidence-based model and effort
  selection.
- [`docs/agents/process-skills.md`](docs/agents/process-skills.md): task-risk classification and
  conditional Superpowers and Ponytail selection.
- [`internal/gameserver/network/AGENTS.md`](internal/gameserver/network/AGENTS.md): packet impact,
  protocol order, rejection responses, orchestration boundaries, and network concurrency. It applies
  to every change below that directory.
