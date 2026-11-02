# Dormant NPC memory — plan v3 (narrow, no ark)

Status: **SHELVED until measured (2026-11-02). v3 below is the plan to execute** once the probe recorded
on [fatal10110/acis_golang#4392](https://github.com/fatal10110/acis_golang/issues/4392) reports, and if
the numbers pass §9. #4392 also records the verified blockers (v2 A1, A3, A5) that any wider design must
solve. ark is not used. v3 has not had its own port-reviewer pass yet; run one before implementation. Task class
D (concurrency, lifecycle). Parent: [memory-redesign-plan.md](memory-redesign-plan.md) §P7/§P8. Go revision
inspected: `7ce14d479`.

**Goal.** Stop paying for NPC parts that a sleeping NPC never uses, without stubs, without releasing
wired parts, and without a new concurrency protocol.

**Hard constraint.** No client-visible difference from Java: same packets, bytes, per-client order,
rules and persistence.

## 1. What v3 does (and does not do)

| Part | What | Why it is safe |
|---|---|---|
| **A. Brain on first use** | `ai.Attackable` (the NPC brain) is not built at spawn. It is built the first time something needs it: the AI tick in an active region, a mutating script call, or a foreign hate or desire write. Once built, it is never released. | **Creation-only** compare-and-swap (CAS): there is no release, so there is no retire protocol (v2 A3). Same NPC object (v1). Move, attack and cast are untouched (v2 A1/A2). |
| **B. Shrink on deactivation** | In the existing deactivation reset (owner queue, `enterInactiveRegion`): the threat and hate tables drop their maps instead of `clear()`ing them (`clear` keeps the buckets). After `StopAll`, the effect list drops its `buffs`/`debuffs`/`stacks` backing storage. The desire queue keeps its entries (the reference keeps desires) and only trims spare capacity. | Same point and same locks as today's `Clear`/`StopAll`. A nil container is re-made on the next insert under the same lock, which is indistinguishable from a cleared one. |
| C. (optional) Effect list on first use | `effect.List` built on first mutation from the wiring `Live` already holds (owner + options) | Only if A+B measure short; 153 `EffectList()` sites to audit |

Not doing, with reasons:
- releasing any part after creation: wired dependencies plus foreign writers (v2 A1–A3);
- stub types: identity (v1);
- ark: fixed-shape state, so no role (v2 A6);
- a separate sleeper set for the AI tick: this belongs to #4388/#4402 (v2 A7).

## 2. Facts this relies on

| # | Fact | Evidence |
|---|---|---|
| G1 | The brain is built once, in `NewHostile`. It is wired in four places: `SetRandomWalkRate` (`npcs_spawn.go:193`), `SetCastController` (`npcs_hostile.go:237`), `SetRoute` (`npcs_hostile.go:243`), `KeepTerritory` (`hostile.go:411`). | `npc/hostile.go:295` |
| G2 | No code stores the brain pointer as a field outside `Hostile`. Access goes through 53 `h.brain` sites in package `npc` and 31 `.AI()` calls elsewhere. | `hostile.go:62, 539`; rg at `7ce14d479` |
| G3 | Sleep is latched in the reference (spawn, deactivation, activation, out-of-territory teleport, death, delete). The Go per-tick re-decision is gap **#4402**. | Java `Npc.java:489, 576-577, 627, 666, 672-705` |
| G4 | While asleep, the reference refuses `AddSocialDesire`; other desire adders queue (they materialize the brain in v3, exactly as they would allocate today). | Java `NpcAI:861-869`; Go `ai/attackable_desires.go:139` |
| G5 | Deactivation clears only threat and hate (`setBackToPeaceLocked`) and stops effects. Desires, intention and counters survive. | `ai/attackable.go:707-710`; Java `NpcAI:403-407` |
| G6 | `ThreatTable.Clear` uses `clear(t.entries)`, so the map keeps its memory. | `attackable/threat.go:317-322` |
| G7 | `effect.List` is a pointer in `creature.Live` and holds wiring (owner, activity registry, night source, admission) plus `buffs`, `debuffs` and the `stacks` map. | `creature/live.go:16-18`, `skill/effect/list.go:171-201` |

## 3. Part A design: brain on first use
- **Field:** `brain atomic.Pointer[ai.Attackable]` on `Hostile`, plus a **recipe**: the inputs to
  `ai.NewAttackable(h, movement, attack)` and the four wirings (G1). Those are pointers the NPC
  already holds, plus one walk rate, about 40 B.
- **`AI()` (mutating accessor):** load the pointer; if nil, build from the recipe and CAS-publish;
  the loser discards its copy. The constructor and the four wirings must have **no side effects**
  outside the new object: no registrations, no RNG, no clock reads that persist. A Phase 1 test
  enforces it.
- **Nil-safe reads** return exactly what a freshly built brain would return. These cover the paths a
  sleeping NPC hits: `Intention()`, `LifeTime()`, hate and desire queries (empty), `AISleeping`,
  `SetBackToPeace` (nothing to clear), hate `refresh`/`removeIfOutOfRange` (no-op).
- **`AddSocialDesire` on an NPC with no brain:** check the sleep latch first (#4402). Asleep → refused
  without building, as today. Awake → materialize, then add.
- **Eager exceptions:** walkers and route NPCs, noSleepMode, out-of-territory, and anything a
  `CREATED` hook mutates. They build at spawn (the hook materializes it), just like today.
- **Who triggers creation:**
  - the AI tick on an awake NPC (owner queue);
  - script hooks calling mutators (owner queue: hooks run on the NPC queue);
  - foreign writers such as skill aggro, clan calls, `AI().Hates().Add`, `AddDamageHate`. They all
    pass through `AI()`, and CAS makes the first one win.
- **Equivalence:** a brain built late must equal a brain built at spawn and left untouched since.
  This holds if construction depends only on the recipe. Brain fields that the reference updates
  while the NPC sleeps would break it, and G4/G5 show none do without a mutator, which materializes
  the brain anyway. Phase 1 proves it with a deep-equality test.

## 4. Part B design: shrink on deactivation
- `ThreatTable`/`HateTable`: add a `Reset()` that sets `entries = nil` under the table lock, used only
  by `setBackToPeaceLocked`. `AddDamage`/`Add` lazily `make` on a nil map under the same lock (nil-map
  reads are already safe in Go).
- `effect.List.StopAll`: after stopping, set `buffs`/`debuffs` to nil and `stacks` to nil under `l.mu`.
  Insert paths lazily make them.
- `DesireQueue`: if `cap > 2×len`, clip by copying under `mu`. Entries and order are kept.
- `world.KnownBuffer` free-list slices: drop them on deactivation, until memory-redesign P4 (#4399)
  removes per-NPC buffers.

## 5. Phases

| Phase | Content | Gate |
|---|---|---|
| Pre | #4402 sleep latch (Part A reads it); P3 #4392 lazy maps (Part B shares the lazy-make code) | Their own gates |
| 1 | Accessor conversion: 53 `h.brain` sites become `AI()` or nil-safe peeks; 31 external `.AI()` sites stay. Tests: (a) late-built brain deep-equals spawn-built brain for every datapack Hostile template; (b) constructor side-effect freedom (two builds, discard one, no registrations or leaks); (c) the access test forbids direct `h.brain` field use outside the accessor file. | Tests green; `make test-race` |
| 2 | Part A on, behind config `NpcLazyBrain` for the canary | §6 suite; flag on vs off byte-identical per client; heap profile delta at 0 players |
| 3 | Part B (always on; no flag needed: same locks, same point) | Unit tests for nil re-make; combat full-flow re-fight after deactivation |
| 4 (opt.) | Part C lazy effect list | Only if Phase 2+3 measured gain < target; 153-site audit |

## 6. Tests (full-flow under `tests/npc/`, real MariaDB, `gameservertest.Boot`, Inline clock)
- **Never-woken NPC:**
  - "1001" timer fires and no brain is built;
  - "2001" timer builds the brain and queues the same cast desire as flag-off;
  - activation then gives the same first think.
- **Foreign hit:** an awake noSleep NPC's AoE reaches a never-woken NPC. The brain is built and its
  hate entries equal flag-off's.
- **Race:** two attackers hit a never-woken NPC at once; one brain, both hates recorded.
- **Walk-in:** NpcInfo plus MoveTo/cast state packets match flag-off byte for byte, in the same
  order.
- **Re-fight:** fight, attacker leaves, deactivation shrink, a player returns and fights again. Same
  packets and hate behavior as flag-off.
- **Lifecycle:**
  - maker `DeleteAll`, decay hooks, `//reload npc`, shutdown persistence on never-woken NPCs;
  - nothing changes, because the object is the same.

## 7. Expected gain (heap profile, production, 0 players)
- **A:** `ai.NewAttackable` 35 MB cum + `attackable.NewThreatTable` 6.5 MB + `NewHateTable` ≈ **~40 MB**
  at idle. With players online, only NPCs in active regions or ones that were hit get brains.
- **B:** retained container capacity on NPCs that fought and then went inactive. It bounds growth over
  uptime; measured in Phase 3.
- **C (optional):** `effect.NewList` 11 MB + its closures ≈ ~11–15 MB.
- These overlap with memory-redesign P3 (#4392). Land them as an extension of its scope.

## 8. Review resolution
- **v1 findings:** stub/identity, walk-in, assertion sites, references, queue close, timer homing,
  decay, reload and lock graph are all moot: no stubs, no release, same object.
  - #3 wander timer: unaffected; v3 has no eligibility gate.
  - #4 sleep latch: prerequisite #4402.
  - #5/#14 lived state: no capture needed; nothing is released after creation.
- **v2 A1** (parts not rebuildable): only the brain is built late, from a 4-wiring recipe. Nothing
  wired is ever rebuilt.
- **v2 A2** (part pointers held elsewhere): move, attack and cast are never released. The brain is
  never cached outside `Hostile` (G2).
- **v2 A3** (retire vs table locks): no release, so no retire. Creation is CAS-only, and foreign
  writers go through `AI()`.
- **v2 A4** (audit size): 53 + 31 brain sites. The 153 effect-list sites move to optional Part C.
- **v2 A5** (gain vs desires): no eligibility gate. Part A targets never-woken NPCs, the majority at
  idle; Part B applies to every deactivation.
- **v2 A6** (ark): dropped.
- **v2 A7** (sleeper set): out of scope (#4388/#4402).

## 9. Resume trigger (when the #4392 probe reports)

Run the probe from the #4392 comment on production or the bot swarm (#260), then decide per part:

| Probe result | Action |
|---|---|
| Brain + tables held by never-woken sleeping NPCs ≥ **30 MB** | Do **Part A** (brain on first use), after #4402 and #4392 land |
| Capacity held by cleared hate/threat maps and effect-list containers ≥ **10 MB**, or growing with uptime | Do **Part B** (drop cleared containers at deactivation) |
| Effect lists on never-buffed NPCs ≥ **10 MB** after A+B | Consider **Part C** |
| Below all thresholds | Stay shelved; P2/P3 already cover it |

Before implementing:
1. Run a port-reviewer pass on v3.
2. Re-check G1–G7 against the then-current `main` (line numbers will have moved).
3. Confirm #4402 has landed.

A wider design (releasing parts after creation) must first solve blockers A1, A3 and A5 (§8 and #4392).
