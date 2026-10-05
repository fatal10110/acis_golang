# Script engine: design and staging (M9 engine, M10 content)

Status: decided 2026-10-05, amended the same day with the owner decisions on the alliance
capability (X4), the first script-event consumer (E8) and the stand-in deletion (section 8).
Implementation has not started.

## Context

The reference server keeps most game content in compiled script classes. The live set, as
listed in the datapack `data/xml/scripts.xml`, is 857 scripts:

| Kind | Count |
|---|---|
| Quests (341 numbered + the third-class saga core) | 342 |
| NPC behaviors (AI) | 474 |
| Feature NPCs | 16 |
| Teleporters | 13 |
| Siegable-hall scripts | 6 |
| Scheduled tasks | 6 |

Spawn makers (43 classes) are a separate mechanism bound from the spawnlist XML.

Go has no script engine: `internal/gameserver/script/` holds package stubs only. Several
basics are missing because the reference implements them in scripts: monsters do not aggro on
sight, hostile NPCs never cast on their own, quests and the tutorial do not exist.

**Owner constraint.** No new scripts will be authored in the near future. The set is closed.
The design therefore optimizes for a faithful one-time port by parallel lanes and for
checking that port against the reference. Nothing is built for script authoring.

## Decision

Scripts are compiled Go values: one type, `script.Script`, holding a table of typed handler
funcs. There is no interpreter, DSL, plugin loading, hot reload or transpiler.

- A behavior is built by copying its parent's table and replacing handlers. Calling the
  parent is a call to the captured parent table.
- `scripts.xml` stays the source of which scripts load, in what order, and on what schedule.
  Go keeps literal catalogs mapping each path to a constructor.
- Hooks run synchronously where the engine raises them. Every piece of script-visible state
  is a small leaf-locked container.
- Verification rests on a manifest produced by running the real reference classes.

## 1. Script shape

```go
// internal/gameserver/script
type Script struct {
    Name, Dir, Title string // Name: character_quests key and page directory leaf
    QuestID  int32          // > 0: shown in the quest journal
    Items    []int32        // taken on exit
    Bind     Bindings       // explicit ids per event (quests, features, teleporters)
    Behavior bool           // NPC behavior: one per (npc, event), last listed wins
    NPCs     []int32        // a behavior's own ids
    Hooks                   // about 29 func fields; nil = does not react
}
```

- **Hook signature:** `func(s *Script, e Payload)`. `s` is the registered script and is
  passed down parent calls unchanged. `e` is one small struct per fact, for example
  `script.Attacked{NPC, Attacker, Damage, Skill}`.
- **Nil-safe invokers:** `func (h *Hooks) Attacked(s, e)`, one per field. A parent call is
  `base.Attacked(s, e)` even when no ancestor reacts, so every parent call ports 1:1.
- **Derive:** `base.With(Hooks{…})` overlays the non-nil fields.
- **Event set:** a behavior is bound, for each of its own `NPCs`, to every NPC event whose
  field is non-nil after `With`. A parent's ids are never inherited.
- **Re-entry into the final script:** `s.Attacked(e)`. The reference has 10 such sites (7
  direct, 3 through a shared attack-event helper).
- **String hooks** (talk, first talk, bypass event, timer) return the reference's string and
  the engine classifies it once: ends with `.htm`/`.html` → page file; starts with `<html>`
  → literal page; any other non-empty string → chat line; empty → nothing.

```go
func warriorAggressive() script.Script {
    base := warrior().Hooks
    return script.Script{Name: "WarriorAggressive", Behavior: true, NPCs: []int32{ /* own ids */ },
        Hooks: base.With(script.Hooks{
            OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
                if !e.Creature.Playable() {
                    return
                }
                tryToAttack(s, e.NPC, e.Creature)
                base.SeeCreature(s, e)
            },
        })}
}
```

Why this shape:

- The reference subscribes a behavior to the union of hooks overridden anywhere in its
  chain. "Field is non-nil" reproduces that with no reflection and no runtime interface
  assertion.
- It carries no type hierarchy. The 8-level behavior tree becomes function composition.
- Embedded structs were rejected: embedding has no virtual dispatch, so the 10 re-entry
  sites would bind to the wrong level, and 427 hand-kept event lists would drift silently.
- Ports stay mechanical: one class → one constructor; one override → one field; one parent
  call → one `base.X(s, e)` at the same position (1,010 of 1,019 parent calls pass their
  arguments unchanged).

## 2. Registry and boot

| Rule | Mechanism |
|---|---|
| Enablement, order, schedule attributes | read from `scripts.xml`; the engine iterates the parsed list |
| Path → constructor | Go literal catalogs, `map[path]func() script.Script`, joined in `cmd/gameserver` |
| Listed path with no Go script | logged at error level and skipped; completeness is a test |
| Catalog entry not listed | never built |
| One behavior per (npc, event) | registering a behavior removes any other behavior from that list, then appends; the last listed wins. Three ids are claimed by a parent and its child; the child is listed later |
| Other scripts | accumulate in list order |
| First talk | one slot: the first registered keeps it, except that a behavior replaces an earlier behavior. It fires only when exactly one script is bound |
| Abnormal-status hook | dispatched over the see-spell list |
| Mutability | immutable after boot; lock-free reads; `//reload script` answers "unsupported" |

Boot order: the engine is attached before any spawn, so boot spawns fire the created hook.
The boot spawn pass moves out of the `Npcs` constructor
(`internal/gameserver/data/manager/npcs.go`).

**Spawn makers** have their own registry keyed by the spawnlist `<ai type>` string, one maker
value per npcmaker, with the default maker for unknown types. 958 datapack makers
(`random_spawn_treasurebox`, `random_maker`) stay default permanently. Maker timers are plain
engine-queue `After` or `Every` calls outside the timer registry: they are unkeyed, may
overlap and are not cancelled, except that the `benom_maker` siege maker stops its own
periodic tower check, so `Every` hands the maker its ticker handle.

**Scheduled tasks** are catalog scripts with one schedule hook. The reference alternates a
start and an end hook when an entry sets `end`. No live task sets it (the only live `end` in
`scripts.xml` is on a plain quest, which the scheduler does not read) and every task's end
hook is empty, so the start hook alone is observably equivalent. The four schedule kinds in
live use are built (hourly, daily, weekly, monthly-by-week); any other attribute logs loudly.
The 5-minute rescan is kept, and the calendar arithmetic is pinned by a golden.

## 3. Layout

```
cmd/gameserver ─► script/catalog ─► script/quest/q646, script/feature/…, script/teleport/…,
       │                            script/ai/{monster,warrior,wizard,raid,guard,boss/…,hall,…}
       ├────────► script   (engine: Script, Hooks, payloads, registry, timers, handles)
       │              └─► model/…, world, sim, persist
       ├────────► data/manager ─► model/actor/npc   (never imports script)
       └────────► network ─► script
model/actor/{npc,ai,cast}, model/zone and model/door import nothing from script.
```

- **One package per quest, feature and teleporter.** Constants and helpers are ordinary
  package-level code, and lanes cannot collide.
- **Behaviors share family packages.** A child imports its parent's package, so parents-first
  is enforced by the compiler.
- **Catalog files are pre-created per range or family** (`quests_3xx.go`, `ai_warrior.go`) in
  one scaffold PR. A content lane edits one catalog file that no other open lane touches.
- **Quest journal:** new leaf package `model/questlog`, held by `player.Character`.

## 4. Seams

- **NPC facts.** A consumer-side interface in package `npc`, injected through the existing
  `npc.Runtime` struct, next to `Rewards` and `Hits`, and as a new field on `npc.FolkRuntime`.
  It grows one method per seam PR, each with a production consumer. No func fields on actors.
- **Other facts.** Interact, bypass, quest list and abort, tutorial opcodes, item use, zone
  enter, door change, player and summon death: direct engine calls from the owning package.
- **Outbound.** Scripts never touch `serverpackets`. Handles call exported domain methods
  that emit `event.Event` values (quest list, quest mark, sound, page, radar, tutorial,
  items taken). The network layer maps them and records page bypasses in the whitelist.
- **Handles.** Scripts see only `*script.NPC` (wraps `Hostile` or `Folk`), `*script.Player`,
  `script.Creature` and `*script.QuestState`. This surface is the frozen API. No handle
  method logs and returns: an operation a script needs must exist for the NPC kinds the
  script is bound to.

## 5. Where hooks run

Hooks run synchronously on the goroutine that raises them, with no lock held.

| Hook | Runs | Note |
|---|---|---|
| Talk, bypass event, first talk, item use, tutorial opcodes | player queue | |
| Attacked, from a hit | attacker's queue, before the HP change | dead targets skipped |
| Attacked, from a skill | caster's queue, after the skill's effects, no dead check | only offensive debuffs or skills with aggro points; value `max(120, aggro points)` |
| Attacked, from an aggression effect | where the effect is applied | no HP change, no skill |
| No-desire, see-creature, move finished, out of territory | NPC queue, at the reference's point inside the think pass, with the brain mutex released per phase | a think raised from inside a hook or from another queue runs in place, not deferred |
| Skill finished, attack finished | NPC queue, after the brain mutex is released | |
| Created | spawner's goroutine, per NPC, right after its read-locked spawn call returns and before the maker's spawn hook | E6 narrows `RespawnAll`'s write lock to the spawn-list swap |
| Decayed | right after the decayed flag flips, before world removal | then behavior timers are cancelled, then removal. The script-facing delete is synchronous |
| Dying | one 3 s timer on an engine-owned queue; all hooks in list order; no state re-check | corpse queues close at decay; 103 templates decay within 3 s |
| Party died, clan died | killer's queue, inline with the death | |
| Zone enter | mover's goroutine, collected under the zone lock and fired after unlock | one quest uses it |
| Tutorial HP and level triggers | caller, after the vitals lock is released | |

Fan-out rules differ per source and each gets its own function and golden:

| Source | Party | Clan |
|---|---|---|
| Hit | self, then master (alive, not self), then minions (skip self and dead) | self first, then NPCs of any type in clan range: skip dead and self; clan match; ignored ids; line of sight from the caller |
| Aggression effect | master without the not-self check; minion loop does not skip self | self only |
| Skill | as the hit | no self call; attackable NPCs only; line of sight from the target |
| Death | party-died: self, master (no dead check), minions except self; a dying master clears its minions' master link | clan-died: neighbours only, no self call |

## 6. State

Each item is a leaf-locked container. Nothing new is queue-owned.

### Quest journal

- String-map semantics as in the reference. Reserved keys `<state>`, `<cond>`, `<flags>`.
- Mutations append to a pending list under the journal mutex. After unlock, one drain job is
  enqueued on the player's `persist.Worker` lane and applies the pending writes in one
  transaction. DB order equals memory order.

| Operation | Statements, in order |
|---|---|
| set | upsert |
| unset | delete var |
| set cond | upsert or delete `<flags>` when the flags rule fires, then upsert `<cond>` |
| exit, not repeatable | upsert `<state>=COMPLETED`, then delete every var except `<state>` |
| exit, repeatable | delete every row of the quest |
| new state (created) | nothing |

- Load at character selection: `SELECT name,var,value … WHERE charId=?`. Go adds
  `ORDER BY name,var`, which pins the primary-key order `(charId, name, var)` the reference's
  rows arrive in. Names resolve case-insensitively, first match in list order; unknown names
  are skipped with a warning.
- Sealed at detach. A dirty sealed journal is drained before the next selection loads; if
  that drain fails, the selection is refused. Nothing else writes a sealed journal.
- Character purge deletes `character_quests` and `character_memo` rows.

### NPC scratch memory

About 4,200 script uses (3,320 int slots, 900 creature slots). One mutex-guarded container
owned by the spawn slot and re-bound to each respawned NPC: the reference reuses the NPC
object across respawns and resets only the script value.

### Script-shared state

Seven behaviors and about 21 quests keep state on the script. It lives in engine cell types
only, with no closure-taking update method. A `go/types` test fails any script package that
declares a package-level variable, uses `go`, `sync` or `sync/atomic`, or has a hook closure
referencing a mutable constructor-local.

### Timers

One registry keyed by (script, name, NPC, player) identity. NPC identity is the spawn slot's.
A nil NPC or player means "bound to none", never a wildcard.

- Duplicates are refused atomically. A one-shot removes itself before firing, so a hook may
  re-arm the same key. No liveness check at fire time.
- Fixed-rate timers are chained one-shots on a fixed grid (`sim.Queue.Every` drops late
  ticks and has no separate initial delay).

| Timer | Home queue | Removed when |
|---|---|---|
| Script is a behavior bound to that NPC's template, NPC alive | NPC queue | at decay, after the decayed hooks; at detach if a player is bound |
| Otherwise, a player is bound | player queue | at detach, for every script |
| Otherwise | engine queue | script cancel only; survives the NPC's decay |

### Other

- In-memory global memo; `spawn_data` db value saved at shutdown.
- **Panics:** recovered per hook invocation and logged with the stack. No quarantine.
  Helpers panic where the reference throws, so an invocation aborts at the same point. A
  string hook that aborts sends nothing.
- **Shutdown:** engine queues close and timers cancel before the persist worker closes, with
  a final journal drain beside the item drain.

## 7. Dialog rules

- The last quest NPC is set on every interact, before first talk, and again in the single
  quest window.
- `npc_<id>_Quest` opens the general window: talk-bound real quests with a state other than
  created (completed included) first, then quest-start entries. Zero candidates → the
  no-quest page; one → the single window; more → the choice list.
- `npc_<id>_Quest <name>` does not require the script to be bound to the NPC. An unknown
  name gives the no-quest page. A script that is not a real quest gets no overweight check,
  no 25-quest check and no state creation.
- Single window for a real quest: overweight → the 80 % system message only; no state and 25
  started quests → the too-many page; then last NPC is set and talk runs.
- Top-level `Quest <name> <event>`: the page whitelist must allow it; the last quest NPC must
  be in the world and strictly within 150 (3D, centre to centre); that NPC must have a
  talk-bound script equal to the named one (any two behaviors count as equal). Every
  rejection is silent.
- The `npc_` handler appends one ActionFailed to every accepted command; a whitelist
  rejection or an id that does not parse sends nothing. A page therefore gives page,
  ActionFailed, ActionFailed.
- Hostile-typed NPCs need the same path: 48 quests bind talk or quest start to guards.
- Range checks are strict `<`, centre to centre. `party.InRange` (`<=` plus collision radii)
  is not reused.

## 8. Existing stand-ins

Stand-ins stay behind one predicate, "no behavior is bound to this NPC id". A template
switches to real behavior when its script registers. The three ids claimed by a parent and a
child switch only when the child is ported.

- They are never extended. They are deleted in one PR (#3497, M12) once every scripted NPC id
  has its behavior, including the siegable-hall behaviors of #3502 (M12). No id is exempted,
  and M10's exit does not include the deletion.
- The list: idle wander and follow, `attackedHateWeight`, Warrior-style party assist, the
  shot-recharge roll, the walker alias rule, `Party_Type` 2 privates, `despawnMinions`, and
  default treatment of the scripted maker type strings (predicate: no maker registered).
- A script cannot register until every seam it subscribes to, and every handle operation it
  calls, is implemented for the NPC kinds it is bound to (boot check and test).

## 9. Deliberate differences from the reference

1. A detaching player gets no script give, take or journal write. Enforced by one new flag
   on `player.Character`, set where `markDetaching` runs, before inventory persistence is
   released. This closes a relog duplication window.
2. A failed quest load refuses the character selection. The reference continues with an
   empty journal, which lets one-time rewards be taken again.
3. The top-level `Quest` bypass is refused with ActionFailed while the player trades or runs
   a private store. This closes a check-then-take race.

Everything else follows the reference, including its quirks (abort by id, the 25-quest check
only at talk, the abnormal-status dispatch list).

## 10. Not built

Interpreter, DSL, plugin loading, hot reload, transpiler, script lookup APIs, hooks with no
user (enter-world, game-time, zone-exit, maker-npcs-killed), and schedule kinds no live entry
uses.

## 11. Verification

1. **Oracle spike (first).** Compile the whole reference tree with `javac` in the
   `eclipse-temurin:21-jdk-alpine` image; shadow four infrastructure classes (thread pool,
   random source, connection send path, connection pool); boot to the script loader. This is
   unproven: existing probes are single files.
   - Gate 1: the registration manifest for all 857 scripts. Fallback: a static scan for the
     behavior union, plus a hand-audited manifest, reviewed blind, for the 31 sagas and about
     27 collection- or loop-driven registrations.
   - Gate 2: two quests produce deterministic packet traces. Fallback: the reviewer
     re-derives expectations blind.
2. **Per script, mechanical, in CI.**
   - The Go registry dump equals the manifest: bindings, each class's own hooks, parent-call
     shape, and the folded per-(npc, event) lists.
   - Distinctive literals match between the reference source and the Go package.
   - Every page literal exists in a committed HTML index (names, bypass commands,
     placeholders, hashes; no page text).
   - One unit test covers the four hand-kept `Hooks` lists (fields, invokers, `With`, bound
     set).
3. **Engine contracts, once.** About 45 reference-derived goldens: cond and flags bits; set
   cond and exit packet and SQL order; quest list contents; four drop types with roll counts
   and a fractional rate; range checks; the dialog rules of section 7; the fan-out rules of
   section 5; timer rules; the first-aggro tick; schedule calendar.
4. **Per-script behavior.** A scenario format run by one generic test in `tests/quest` and
   `tests/ai` through `gameservertest.Boot`: talk, link, kill ×N, advance, relog → expected
   pages, messages, items, rows. Expectations come from the reference trace if gate 2 passes.
5. **Concurrency.** The brain-locking change and every emit path get packet-order scenarios
   on both the inline and pool executors under `-race`.

## 12. Staging

### M9: engine and proof batch (about 60–65 PRs)

Chains run in parallel. Each hot file allows one open PR: `attackable.go`, `hostile*.go`,
`npcs_spawn.go`, `bypass.go` / `folk.go`, `character_flow.go` / `client_loop.go`,
`live_player_events.go`, `gameservertest/boot.go`, `cmd/gameserver`.

| ID | Slice | Depends on | Issue |
|---|---|---|---|
| V0 | Reference probe harness; registration manifest golden | — | #3476 |
| V1 | Static fingerprint extractor (literals, helper calls, parent-call shape) | V0 | #3482 |
| V2 | HTML index golden | — | #3477 |
| V3 | Engine-contract goldens | V0 | #3483 |
| V4 | Scenario runner for `tests/quest` and `tests/ai` | E5 | #3481 |
| E1 | Registry rules, `scripts.xml` loader, catalog lookup, dispatcher with panic isolation, registry dump, seam gate | — | #170 (+#3411) |
| E2 | Journal read: store, test schema, load at selection, purge, real quest list, list request | E1 | #167 |
| E3 | Journal write: set/unset/state/cond flags/exit, lane persistence, seal, quest mark packet, abort, detach flag | E2, V3 | #167 |
| E4 | Helpers: give, take, reward, four drop types, sounds, checks, party and clan-leader lookups, quest rates, radar, script random source | E3, V3 | #3487 |
| E5 | Dialog path of section 7 for folk and hostile NPCs, with the first plain quest | E1, E3, E4 | #130 |
| E6 | Created, dying (3 s), decayed; boot spawn pass moved; `RespawnAll` lock narrowed; synchronous delete | E1, E4 | #3490 (+#2164) |
| E7 | Timer registry | E1, A3 | #168 |
| E8 | One-off seams, each with its first consumer: item use, zone enter, player and summon death (proof quests); script events (the Warrior base of A11, which sends and receives them; own engine PR just ahead of A11) | E5, E6; script events: E1 | #3493 (+#867) |
| A0 | Brain phase locking: hooks at the reference's points, nested think in place | — | #3478 |
| A1 | NPC skill types kept at load | — | #3479 (+#3470) |
| A2 | Spawn-memo params first, then template; slot-owned scratch memory; life-time accessor | — | #3480 (+#3470) |
| A3 | Script spawn API: handle-returning spawn, privates from the template, timed despawn | A2 | #3484 (+#2081, #3306) |
| A4 | Desires I: cast (with hold and conditions), follow, wander, do-nothing, attack variants | A0, A1 | #3485 |
| A5 | Desires II: flee with its guard; social with the social-broadcast gate | A4 | #1804 |
| A6 | Attacked from three sources and party attacked, per called NPC; gates the matching stand-ins | E1, A2 | #3486 |
| A7 | Clan attacked scan, party died, clan died, folk attacked | A6 | #1800 |
| A8 | Skill finished, attack finished with target, see-spell, spelled, abnormal status | A6 | #3488 |
| A9 | No-desire, move finished, out of territory, for hostile and folk; gates idle stand-ins | A4 | #3489 (+#2148, #2162, #2163, #2240) |
| A10 | See-creature tick scan with its gates, look-neighbor throttle, re-fire after teleport | A9 | #3491 |
| A11 | Behavior base plus the first Warrior chain: Warrior base, Warrior, WarriorAggressive (first cut-over: 514 ids, 299 + 215) | A1–A10, E6, E7, E8 (script events), X4 | #3494 |
| A12 | Rest of the behavior spine (20 classes, about 840 ids in total) | A10, A11 | #3495 (+#2162, #2163, #2240, #3306, #176) |
| A13 | Route movement as a weighted desire, with the walker script | A4, E6 | #2165 (+#3497, M12) |
| A14 | Folk-side desires for the 81 folk ids bound to behaviors | A4, A9 | #3492 |
| M1 | Maker registry and hooks, with the default, no-on-start and event makers | E1 | #171 |
| T1 | Schedule runner with three tasks | E1, V3 | #172 (+#3149) |
| T2 | The other three tasks | T1 | #172 (+#3149) |
| U1 | Tutorial opcodes and packets, voice sound, radar, memo store, engine triggers | E3 | #174 |
| U2 | Tutorial script and its creation-time state | U1, E5, E7 | #174 |
| U3 | Newbie helper | U2, E6 | #174 |
| X1 | Subclass gates on two quests and the class-change exit of a third | E5 | #3070 |
| X2 | Academy graduation on class change | — | #3181 |
| X3 | Admin NPC script info pages | A2, A11, E7 | #3398 |
| X4 | Player alliance level and the faction capability | E3 | #908 |

X4 (#908) is an M9 slice: it lands before the petrify branch of A11 (the petrify helper of the
behavior base reads both ally checks) and before the alliance-gated proof quest Q607 (#175)
registers. The faction quests, features and behaviors that use it stay in their M10 issues and
depend on it.

Must land in the same PR:

- A9: no-desire dispatch and the gate on legacy idle.
- A6: attacked dispatch and the gate on hate weight, party assist and the shot roll.
- A11: the Warrior registration with every layer below it.
- M1: maker dispatch with the three makers (13 no-on-start makers stop boot-spawning).
- A13: route desire, the walker script and the alias-rule gate.
- U2: the creation-time tutorial state and the tutorial script.

**Proof batch.** It freezes the API: one script per mechanic (plain talk and collect; party
credit; multi-item drop; quest attacked hook; newbie shots; timer and spawn; decayed hook;
clan leader state; second-class change; saga core; item use; zone; player death; the five
engine-referenced quests: the item-use quest above, whose state also gates the drain-soul
skill handler of E8, the diary-write quest of E5, the two subclass-gate quests and the
class-change quest of X1), the tutorial and newbie helper, four features, two teleporters,
six tasks, three makers and the behavior spine. The API freezes when the census of every
engine call made by the 857 scripts has no unmapped row.

### M10: content (about 165–175 PRs)

- Quests are independent and never on the critical path: about 8 plain quests per PR; the 30
  remaining sagas are one data PR; any script over 500 lines goes alone.
- Behaviors go parents-first in waves; a lane may take a parent and its children together.
- Bosses last, after the global memo and makers. Siege-hall scripts wait for M12 (#3502),
  and so does the stand-in deletion of section 8 (#3497), which is not part of M10's exit.
- The Dimensional Rift quest is M10 content in #184, ported after its rift maker (#213) and
  the maker registry (M1, #171). It is not in the proof batch.
- Content PRs never touch the engine or the script-facing API. A gap drops that script from
  the batch and files an issue.
- Start with three canary batches (plain quests, one behavior subtree, one feature).

## 13. Risks

1. The brain-locking change (A0) alters tick and attack interleaving. It lands alone, before
   any behavior, with packet-order scenarios.
2. The frozen API turns out wrong after hundreds of scripts. Mitigated by the call census,
   the proof batch and the canaries.
3. The probe fails gate 2. Per-script behavior evidence then rests on blind reviewer
   re-derivation.
4. A reward traded to another player can become durable before the giver's quest rows if the
   process is killed in that window. It is the same class as the accepted item-write
   trade-off; the soak test gets a case for it.
