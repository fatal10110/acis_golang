# 0006. Write game scripts as compiled Go, with no script interpreter

- **Status:** Accepted and implemented
- **Date:** 2026-10-05
- **Sources:** [script-engine-plan.md](../script-engine-plan.md), `internal/gameserver/script`

## Context

The reference server keeps most game content in script classes. `data/xml/scripts.xml` lists 857
live scripts: 342 quests, 474 NPC behaviors, 16 feature NPCs, 13 teleporters, 6 siegable-hall
scripts and 6 scheduled tasks. There are also 43 spawn maker classes. Before this decision the Go
server had no script engine, so basic things were missing: monsters did not attack on sight, hostile
NPCs never cast spells, and quests did not exist.

The owner set one constraint: **no new scripts will be written in the near future**. The set is
closed. What matters is a faithful one-time port that can be checked against the reference, not a
tool for writing new content.

## Decision

Scripts are compiled Go values. There is one type, `script.Script`, which holds a table of typed
handler functions (about 29 hooks, such as attacked, killed, talk and timers). A `nil` hook means
the script does not react to that event.

- A script that extends another copies its parent's table and replaces some hooks
  (`base.With(Hooks{...})`). Calling the parent is a call to the parent's hook.
- `scripts.xml` still decides which scripts load, in what order and on what schedule.
  `script/catalog` maps each entry to its Go constructor.
- Hooks run synchronously where the engine raises them.
- The port is checked against a manifest produced by running the real reference classes, and
  against fingerprints of their source.

There is no interpreter, scripting language, plugin loading, hot reload or transpiler.

## Consequences

- Scripts are type-checked, fast, and can be read and debugged like any other Go code.
- Ports stay mechanical: one class becomes one constructor, one override becomes one hook, and one
  parent call becomes one `base.X(s, e)` call in the same place.
- Changing a script means rebuilding and restarting the server.
- Adding new content needs a Go developer. That is acceptable while the content set is closed.

## Alternatives considered

- **Embed a scripting language, or interpret the Java scripts.** It would allow hot reload and
  authoring without recompiling, but the project writes no new scripts. An interpreter would add a
  large runtime and a second language to check for parity.
- **Model the behavior tree with embedded structs.** Rejected: Go embedding has no virtual
  dispatch, so the 10 places where a script calls back into its most-derived version would call
  the wrong level. It would also need 427 hand-kept event lists that could drift silently.
