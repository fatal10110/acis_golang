# #258 parity sweep: protocol

This is the procedure for the #258 behavioral parity sweep. The plan (why the split looks like this)
is linked from #258. All data comes from `manifest.json`, which `slicer.py` generates; never edit it by
hand.

## Files

| File | Use |
| --- | --- |
| `manifest.json` | Per slice: owned files, routed and secondary `GameClientLink` handlers, read-only context files, Java starting point, evidence packages, wave. Also the waves, domains and domain runs, plus the SHA it was generated at. |
| `slicer.py` | Regenerates the manifest. Deterministic. Exits non-zero on unassigned files, dead rules, bad or stale overrides, routing ties, a slice missing from the waves or the domains, or an evidence path that does not exist. |
| `sweep.py` | Creates issues (`bootstrap`, `delta-create`), renders and patches bodies (`body`, `patch-body`), fills every prompt, lists due domain runs (`runs`). Dry run unless `--apply`. Safe to rerun after a failure. `--manifest` and `--issues` select the files to use. |
| `issues.json` | Slice, wave, domain and delta → issue number, id and link state. Written by `sweep.py`; committed through the wave-prepare PRs. |
| `prompts/wave-prepare.md` | Wave phase A: refresh the manifest and open its PR. |
| `prompts/wave-start.md` | Wave phase B, after the merge: pin, check, open delta issues, gather evidence, launch. |
| `prompts/slice.md` | One parity slice review (P01–P67, P80, P81). |
| `prompts/quest-triage.md` | P68: ranks every quest and sets each band's review set. |
| `prompts/quest-band.md` | One quest band review (P69–P79). |
| `prompts/delta.md` | Review of files that changed after their slice closed. |
| `prompts/wave-triage.md` | Dedupes findings, files follow-ups, closes slice and delta issues; on the last wave, launches the final domain runs. |
| `prompts/domain.md` | One domain review run. |
| `prompts/domain-triage.md` | Files the findings of finished domain runs. |

## Before wave 1

1. Commit this folder as `docs/parity-sweep/` in a PR that refs #258; the owner merges it.
2. From a worktree at the merge commit, run `python3 <worktree>/docs/parity-sweep/sweep.py bootstrap`
   first as a dry run, then with `--apply`. This creates 21 labels, 12 epics, 81 slice issues and
   20 domain issues, links them, and writes `issues.json`. If a call fails, rerun the same command: it
   resumes, and an issue that already exists with the same title is reused, not duplicated.
3. Commit `issues.json` in a PR; the owner merges it.
4. The owner records these decisions on #258:
   - up to 9 agents may run at once, overriding the default of two threads in process-skills.md;
   - triage runs may close slice, delta, wave and domain issues;
   - the owner merges each wave's manifest PR.

## Repo rules that apply to every run

- **Commands** run from the outer `acis_public/` root. Go commands use `rtk go -C <go-root>`
  (docs/agents/tooling.md).
- **Worktrees:** every run works in its own linked worktree at the pinned SHA:
  `git -C acis_golang worktree add ../.worktrees/p258-<run> <sha>`. `<go-root>` is then
  `.worktrees/p258-<run>`. The outer root stays an ancestor, so the reference and geodata tests find
  `aCis_gameserver/` and `aCis_datapack/`. grepai indexes the primary checkout, so confirm its hits with
  `rg` inside the worktree. Remove the worktree when the run ends.
- **One source of truth:** `sweep.py`, `manifest.json` and `issues.json` are always used from the
  current run's worktree, never from the primary checkout.
- **Read-only reviews:** slice, quest, delta and domain runs change no code, write no tests and open no
  PRs. Writing tests and fixes is the follow-up issue's work.
- **No full suites locally** (docs/agents/verification.md). Evidence is the CI run for the pinned SHA
  plus the local-only tests that CI cannot run.
- **Issue search before filing** (porting-playbook §7):
  - run `python3 scripts/agent-tools/issue_search.py '<terms>' --state all --limit 10`;
  - run a live `gh issue list -R fatal10110/acis_golang --state all --label <sweep label> --search '<terms>'`,
    because the cache misses issues filed earlier in the sweep;
  - run `gh issue view <n> --comments` on each plausible match.
- **Java is the truth** for observable behaviour: outer `aCis_gameserver/`,
  `aCis_gameserver/GO_REWRITE_PLAN.md`, `aCis_gameserver/docs/go-rewrite/`. Use
  `.agent-cache/INDEX.md`, the java-lookup and java-researcher agents, and grepai.

## Test runs

- The pinned SHA must have a green CI run (go.yml). CI covers every package except the local-only
  tests.
- **Local-only tests** (verification.md, decision on #4426):
  - geodata: `geo/probe` and `geo/pathfind`. They find geodata in the outer `aCis_datapack/` or in
    `ACIS_GEODATA_DIR`, and always skip without it.
  - reference: the tests that read the outer `aCis_gameserver/`. Set `ACIS_REQUIRE_REFERENCE=1` so a
    missing checkout fails instead of skipping.

  Wave start runs these once per wave; the commands are in prompts/wave-start.md.
- **Extra runs by reviewers:** a focused `-run` on one package at a time, with
  `ACIS_REQUIRE_DATAPACK=1 -v`.
- **Database load:** at most 3 agents may run DB-backed packages (`sqltest.SharedDB`) at the same
  time. `-race` on a DB-backed package counts toward that limit.
- **SKIP** counts as not run; name the test and the reason.
- **FAIL** in a reviewer's own focused run: rerun with `-count=3`. If it is intermittent it is flaky;
  triage files it with the existing `flaky-test` label under D20. If it fails every time it is a
  finding, rated by impact. Never fix it in a review.

## Parity slice method

Scope comes from `sweep.py body P##` at the pinned SHA:

- the owned files;
- the routed handlers (the handler functions, not their host files);
- the secondary handlers, entry points owned elsewhere that reach this slice's code;
- the context files, which are read-only and owned by other slices.

P05 reviews only its kept handlers; its issue lists the routed-out handlers that belong to other
slices.

1. **Map entry points.** Map every routed and secondary handler, bypass, timer or task, script hook and
   loader in scope to its Java counterpart. An entry point with no Java counterpart is a finding.
2. **Extract the contract from Java,** per entry point:
   - inputs and validation;
   - rejection: a reply packet, a system message, or silence;
   - pending-action release and `ActionFailed`, on success and on rejection
     (docs/agents/action-response-contract.md);
   - state changes;
   - persistence;
   - formulas and constants;
   - timing and lifecycle;
   - packets sent: recipients, opcode, field layout, order;
   - the opcode's state gate in `state.go`.
3. **Compare** Go to the contract item by item. Documented parity exceptions (porting-playbook §3), and
   simplifications documented in code or the rewrite spec, are not drift. An undocumented
   simplification is S3.
4. **Check production wiring** with the `deadcode` and empty-adapter checks in
   docs/agents/verification.md.
5. **Check evidence.** For each contract item, name the oracle, golden or behaviour test that covers it,
   or add the item to the oracle-gap list.
6. **Ask the abuse question:** what does a player gain by repeating, reordering or mistiming this path
   (double submit, logout mid-action, a packet in the wrong state, during teleport or death)?
7. **Report** in the format below.

### Findings outside your scope

Drift found in a context file belongs to the slice that owns the file. Report it with `owner: P##`.
Triage files it under the owner's issue, whether that issue is open or closed. If the owner has not been
reviewed yet, triage also posts the finding there as a lead to verify.

## Quest flow

CI runs the per-quest fingerprint and page tests, so at a green SHA they pass and cannot pick quests
for review.

- **P68** (wave 9) collects these signals for every quest: has a `tests/quest` scenario, rewards adena
  or items, repeatable, gated by class, race or level, and any failing or skipped test. From them it
  sets each band's review set:
  1. every quest with a failing or skipped test;
  2. every quest that rewards items or adena and has no scenario;
  3. the highest-ranked remaining quests, until the set reaches max(5, 15% of the band, rounded up).
     Rank by number of signals, then by quest id.
- **Bands P69–P79** hand-review their review set at their own SHA. Quests with no scenario go to the
  band's oracle-gap issue. The rest of the band counts as covered by tooling.

## Severity

| Level | Parity meaning | Domain meaning |
| --- | --- | --- |
| S0 | Exploit, item or currency duplication, crash, disconnect, data loss, or a client stuck until relog | same |
| S1 | Wrong client-visible behaviour in normal play: wrong packet, value, recipient, order or rejection; a missing `ActionFailed` the next action clears | same |
| S2 | Drift not seen in normal play: edge timing, admin-only paths, logs | Internal defect with no player impact today |
| S3 | Undocumented simplification with no observable effect | Smell or cleanup |

Oracle gaps are not a severity; they carry the `oracle-gap` label.

## Report format (one comment per run)

```
Parity P## at <sha> — <n> entry points mapped; evidence: <CI run link>, <wave evidence comment link>
Not run (skipped or unavailable): <tests or "none">

| Sev | Go (file:line, func) | Java (Class.java:line, method) | Contract item | Expected (Java) | Actual (Go) | Evidence | Fix | Owner |
| S1 | ... | ... | rejection | ActionFailed | silence | tests/items TestX or repro steps | S | P36 |

Oracle gaps: <contract items with no oracle, golden or behaviour test, or "none">
Checked clean: <entry points with no drift>
```

P68 posts the per-band review sets and no-scenario lists instead.

## Triage, follow-ups and closing

- **What gets filed:**
  - one issue per S0, S1 or S2 finding;
  - one "[P##] document or fix simplifications" issue per slice that has S3 findings;
  - one "[P##] oracle gaps" issue per slice that has gaps.

  Nothing is filed for an empty list.
- **Metadata:** titles start with `[P##]`. A follow-up is a sub-issue of its slice or delta issue, on
  milestone M14, with the labels `parity-sweep`, `sev-S#` (or `oracle-gap`) and `wave-N`.
- **Body** (porting-playbook §7):
  - the source slice and review comment;
  - observed (Go) and expected (Java) behaviour;
  - evidence paths and symbols;
  - bounded scope and non-scope;
  - acceptance criteria;
  - verification, including the oracle or behaviour test the fix must add;
  - the smoke path, for a client-visible fix.
- **Known drift:** link an existing issue rather than filing a duplicate.
- **Closing issues:**
  - A slice or delta issue closes once triage has filed or linked all its findings.
  - A wave epic closes once its slice and delta issues are closed.
  - A domain issue closes once all its runs are triaged. D20 closes only after the last wave.
- **#258's acceptance criteria, mapped:**
  1. *All child issues completed or split into follow-ups:* every epic is closed, and no open
     `parity-sweep` or `domain-review` follow-up carries `sev-S0` or `sev-S1`.
  2. *Close to Java unless a Go simplification is documented:* every parity `sev-S3` issue is closed,
     with the simplification either documented in code or the spec, or removed. Domain S3 issues
     may stay open.
  3. *The milestone exit criterion still holds:* the owner confirms it on #258.

  Verification:
  - `go test ./...` passes in CI at the closing SHA.
  - Every closed S0 or S1 parity follow-up was fixed by a PR that adds or updates an oracle, golden or
    behaviour test.
  - Every client-visible fix records its smoke path.
  - Open parity S2, domain S2 and `oracle-gap` issues are allowed only with the owner's
    `accepted-deferral` label and a comment giving the reason. M14 is the last milestone, so there is
    no later milestone to move them to.

## Smoke path for client-visible fixes

`ops/docker/smoke-test.sh` only checks that the stack builds and the schema loads; it does not count. A
client-visible fix needs one of:

1. a behaviour test in `tests/<domain>` that drives the real packet path through
   `internal/gameservertest`;
2. an `internal/botclient` scenario against a running stack;
3. documented manual steps with an Interlude client, in the fix PR.

## Manifest drift and deltas

- **Wave prepare (phase A)** regenerates the manifest at the tip of `main`:
  - It fixes `slicer.py` until the slicer passes.
  - It checks the slice of every new or moved file, because globs and catch-alls absorb new files
    silently.
  - It opens a PR with the manifest, the slicer and `issues.json`.
- **Wave start (phase B)** pins the merge commit and re-runs the slicer there. If file ownership,
  routing or context differ, phase A runs again.
- **Delta issues:** `git diff --name-only <previous pin>..<pin>` lists every changed, new, renamed or
  deleted file. Files that belong to a closed slice get a delta issue (`sweep.py delta-create`) under
  the current wave epic, reviewed with prompts/delta.md. A file that was only edited still gets one.
- **Slice IDs:** a new slice takes the next free ID (P82, ...). Rerunning `sweep.py bootstrap`
  creates its issue. IDs are never reused.

## Domain reviews

Weights, scope notes and runs are in `manifest.json` → `domains` and `domain_runs`.

- **Vertical domains D1–D17** have two runs:
  - run **A** covers architecture, design, smell and performance, and starts at wave 1. D1, D6 and D8
    split it into A1 (architecture and design) and A2 (smell and performance).
  - run **B** covers bugs, and starts at the wave after the last wave holding one of the domain's
    slices, so it can read their parity follow-ups.
- **D18** (concurrency) and **D19** (security) run A once at wave 1, over the whole tree at design
  level. After each wave they run a bugs pass over that wave's slices.
- **D20** covers tooling, tests, ops and observability, not the runtime tree. Both of its runs start at
  wave 1.
- **Launch:**
  - Each wave start launches `sweep.py runs --wave N`.
  - The last wave triage launches `runs --final`.
  - Each run first posts "started <run> at <sha>" on its domain issue. A launcher skips runs that
    already have that comment, unless they never reported.
  - Domain runs share the 9-agent cap.
- **Ownership:**
  - D18 owns races, lock order and goroutine ownership;
  - D19 owns input validation, authentication, access levels and flood protection;
  - a vertical domain's bugs run owns the logic bugs in its slices;
  - parity drift goes to the owning slice.

  When two owners fit, the cross-cutting domain owns the finding and the other links to it.

### Lens checklists

**Architecture**
- Ownership matches docs/network-ownership-map.md: `network` decodes, gates state and orders sends; gameplay rules live in domain packages.
- Dependencies point one way: domain packages never import `network`; nothing reaches around the composition root.
- One owner per piece of mutable actor state (docs/concurrency-refactor-plan.md); list every goroutine kind that touches it.
- No Java structure kept for its own sake (docs/actor-model-degoifying-plan.md): singletons, getter-only types, class-hierarchy mirrors.
- Seams and interfaces exist to break a real cycle or allow a test.

**Design**
- God types and packages (`GameClientLink`, `network`, `actor/npc`, `actor/player`): propose a cut along existing domain lines.
- Constructors establish invariants; no half-built actor is reachable from the world.
- Accept and reject paths follow docs/agents/action-response-contract.md consistently.
- Go idioms per docs/agents/go-style.md: errors returned, not logged and dropped; context and shutdown respected.
- Data-driven rather than hard-coded ids where the datapack has the value.

**Code smell**
- Logic duplicated across quests, AI families and admin handlers that the script ops API should own.
- Dead code, unused exports, unreachable branches (`deadcode`).
- Provenance words in production comments.
- `ponytail:` debt markers (process-skills.md → Ponytail).
- Magic numbers with no named Java constant; functions over ~150 lines; untracked TODOs.

**Performance**
- Hot paths: packet encode allocations, broadcast fan-out, known-list and region updates, movement ticks, pathfinding, AI think loops.
- Lock contention and goroutine count per online player.
- Memory: docs/memory-redesign-plan.md measured 1.38 GB live heap at 0 players; check the domain's share.
- DB: flush batching and N+1 queries on login and save.
- Evidence: pprof (docs/profiling.md), `tests/perf`, `cmd/botswarm`. Without a measurement it is a question, not a finding.

**Bugs**
- Races: focused `-race` runs of the behaviour tests that cover the code (within the database limit); every shared map and slice.
- Abuse: double submit, reorder, packet in the wrong state, logout or disconnect mid-action, request during teleport or death.
- Check-then-act gaps between validation and mutation (items, adena, ownership).
- Integer width and overflow vs Java `int` and `long`; negative counts.
- Panics on malformed input (decodefuzz); goroutine and timer leaks on logout; persistence that can half-apply.

## Labels and issue layout

`sweep.py labels` creates 21 labels, and the existing `flaky-test` label is reused:

- `parity-sweep`, `parity-epic`, `domain-review`, `parity-delta`, `oracle-gap`, `accepted-deferral`;
- `wave-1` to `wave-11`;
- `sev-S0` to `sev-S3`.

The issues hang together like this:

- #258 has 12 sub-issues: the Wave 1–11 epics and the Domain reviews epic, all labelled `parity-epic`
  and on milestone M14.
- Wave epics hold their 5–9 slice issues and any delta issues. The Domain reviews epic holds D1–D20.
- A slice issue is titled `#258 parity P## — <title>` and carries the labels `parity-sweep` and
  `wave-N`. Its body comes from `sweep.py body P##` and is refreshed by `patch-body` at each wave
  start.
- Follow-ups are sub-issues of their slice, delta or domain issue. GitHub allows 100 sub-issues per
  parent, and closed ones count. If a domain issue nears 90, open a "D## part 2" issue under the Domain
  reviews epic.
