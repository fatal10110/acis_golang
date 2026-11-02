# Gameserver memory redesign

Status: proposal (2026-11-02). Scope: internal layout only. **Wire bytes, packet order, payloads, formulas,
data-file interpretation and persistence stay identical.** Every phase is an internal-structure change
gated by the existing oracle and full-flow tests.

## 1. Where the memory goes (production, 0 players)

Profiled `acis-gameserver` on Hetzner (commit `8163215`, cx23 2 vCPU / 3.8 GB): live heap **1.38 GB**,
RSS 1.77 GB, `NextGC` **2.5 GB** (GOGC=100, no `GOMEMLIMIT`). Heap profile (sampled 1.20 GB):

| Owner | Live | Notes |
|---|---|---|
| Geodata (`block.Region` data + index) | **~693 MB (58%)** | 613 MB packed cell data + 36 MB block index + ~45 MB `append` growth slack |
| Spawned NPC object graph | **~350–400 MB (~30%)** | ~35k NPCs (from `cast.Controller` 39 MB / 1152 B) → **~10 KB per idle NPC** |
| Per-NPC broadcast buffers (`KnownBuffer`) + region scratch | ~68 MB | capacity sized to every object in the 3×3 regions |
| Skill table, templates, HTML, makers | ~60 MB | `skill.NewTable` 29 MB (`map[ID]map[int]Definition`, 832 B values) |
| Script timers + indexes | ~25 MB | plus ~45% of idle allocation churn |

Two structural facts drive everything:

1. **Static data lives on the GC heap.** With GOGC=100 the heap target is 2× live, so 650 MB of
   immutable geodata costs ~1.3 GB of heap headroom.
2. **Every spawned NPC is a fully built combat actor**, even in regions no player has visited: ~25
   separate allocations, ~16 mutexes, three logger copies, a by-value 832 B skill definition, a
   976 B stat-calculator pointer array, its own compiled passive-skill mods, eager threat/hate maps,
   and a periodic script timer that allocates on every firing.

Geodata block mix (all 139 `_conv.dat` regions, measured on the server):

| Kind | Blocks | Stored bytes |
|---|---|---|
| Flat | 5,792,898 | 0 (height lives in the index entry) |
| Complex | 2,699,208 | 345 MB (128 B each) |
| Multilayer | 617,398 | 267 MB (192 B header + 74.4M layers × 2 B) |

Measured compressibility (exact, lossless; Go scanner over all 9.1M blocks):
- Complex blocks: only 2.2% exact duplicates, 71 uniform → **dedup is not worth it**. Height span per
  block (in 8-unit steps) fits 4 bits for 57%, 5 bits for 84%, 6 bits for 96% of blocks. 42% of complex
  blocks are fully passable (every cell NSWE = all directions).
- With a per-block integer plane fit, the residual fits 3 bits for 85% of complex blocks.
- Multilayer: 17% exact duplicates; the 192 B per-block header is 44% of multilayer storage. 97.1% of
  blocks hold ≤ 255 layers in total (max 1,295).
- Block index: 36 MB of `[65536]uint32` per region.

## 2. Target architecture

```
                 ┌──────────────── GC heap (small, pointer-light) ────────────────┐
 boot ──▶ geo cache file ──mmap──▶ Region{index, data} views (off-heap, page cache) │
          (lossless compact,        │                                             │
           versioned, ~254 MB)      │                                             │
                                     │  Template (immutable, shared)               │
                                     │    ├─ compiled passive mods per stat        │
                                     │    ├─ AI/raid/reward config pointers        │
                                     │    └─ *skill.Definition (immutable)         │
                                     │  NPC core (always): identity, position, HP,  │
                                     │    zone flags, queue, AI intention           │
                                     │  NPC combat parts (lazy, CAS-published):     │
                                     │    threat/hate, cast/attack ctrl, effects,   │
                                     │    own stat calculators (copy-on-write)      │
                                     │  script timers re-armed in place (no per-    │
                                     │    fire allocation) — still fire when idle   │
                                     └─────────────────────────────────────────────┘
```

Principles: immutable data off-heap or shared per template (flyweight); mutable per-NPC state created
on first mutation; no per-NPC buffers sized by neighborhood; no allocation on periodic paths.

## 3. Phases

Ordered by gain ÷ risk. Each phase ships alone, behind the existing tests plus the named gate.

### P0 — Runtime limit (ops, minutes) — fatal10110/acis_public#13 (moved from #4393)
`Environment=GOMEMLIMIT=2800MiB` in the gameserver drop-in. Bounds the 2.5 GB heap target on a 3.8 GB box.
Green Tea GC is already the default on Go 1.26 (profile shows its span-scan frames); nothing else to tune.

### P1 — Geodata off the GC heap via mmap (−~690 MB heap, −~1.4 GB heap target) — biggest win
- New packed cache file holding exactly today's `block.Region` layout: header (magic, format version,
  digest of source region files: name + size + mtime, or CRC-64 of contents) then, per region, the
  `[65536]uint32` index and the packed data.
- Cache key: format version, `GeoDataType`, the sorted list of regions in `geoengine.properties`, and a
  digest per listed source file. Boot today fails when any listed region fails to load
  (`geo/probe/load.go:63-77`); keep that by still checking every listed source file, so a stale cache can
  never mask a missing or changed one.
- `Region.index` is an inline `[65536]uint32` array today, not a slice, so the query code does change a
  little: either a `[]uint32` view over the mapping (`unsafe.Slice`, little-endian only, 4-byte-aligned
  offsets, checked at load) or index reads through `binary.LittleEndian.Uint32`. `data` is already a slice.
- Built by `cmd/geopack` at deploy time, or at boot when missing/stale (write to temp file, `fsync`,
  `rename` — never rewrite a mapped file in place: truncation under a live mapping raises SIGBUS).
- Open with `unix.Mmap(PROT_READ, MAP_PRIVATE)` + `unix.Madvise(MADV_RANDOM)` from
  `golang.org/x/sys/unix` (already in the module graph as an indirect dependency; Windows dev builds use
  the current heap loader as fallback).
- Effects: Go heap drops by the whole geodata; pages become clean file-backed page cache that the kernel
  can evict under pressure and keeps warm across restarts (no 20–30 s decode); RSS only counts pages
  pathfinding touches. Removes the 45 MB `append` slack as a side effect (#4391 becomes moot).
- Gate: byte-compare every region's index/data between heap loader and cache; `geoprobe` oracle
  dumps unchanged; dynamic door overlays (`geo/dynamic`) keep overriding on top of the read-only base.

### P1b — Lossless compact geodata encoding (649 MB → ~254 MB, −61%)
This is a re-encoding, not compression. Every query (`HeightNearest`, `NSWENearest`, `Nearest`,
`Above`, `Below`, `Height`, `NSWE`, `Layers`, `AppendCells`) returns exactly today's values, so
pathfinding, `CanSee`/`CanMove` and movement validation answer the same. Every cell stays O(1) random
access (a few shifts, masks and two small multiplies), with no streaming decoder and no cache.
Best shipped **together with P1**, so the cache file format is defined once.

Measured over all 139 regions:

| Part | Today | Encoding | Size |
|---|---|---|---|
| Complex blocks (2.70M) | 345.5 MB | Per block: mode byte, base height, integer plane slopes `gx, gy` (i8), NSWE palette (≤16 nibbles). Each cell: residual `q − base − gx·x − gy·y` in *k* bits + palette index in *p* bits (0 bits when the block has one NSWE value). The encoder picks plane or min-base per block, whichever is smaller. | **92.7 MB** |
| Multilayer header (617k) | 118.5 MB | 65 cumulative u8 layer offsets when total layers ≤ 255 (97.1%), else today's header | **42.4 MB** |
| Multilayer layers (74.4M) | 148.8 MB | Per block: base height + *k*; each layer `(q − base)` in *k* bits + NSWE palette index | **98.5 MB** |
| Block index | 36.4 MB | Per region: 2-bit kind array + u16 value per block (flat height or offset inside its 16×16 superblock) + 256 u32 superblock bases. The 413 of 35,584 superblocks (1.2%) whose data exceed 64 KB set a flag in their base and use a u32 offset side table. | **20.6 MB** |
| **Total** | **649 MB** | | **~254 MB** |

Why it is exact:
- Heights in the file are already quantized to 8 units (`q = (code & 0xFFF0) >> 4`, signed), and the
  residual is an exact integer.
- NSWE is a 4-bit value restored through the palette.
- Multilayer keeps today's lowest-to-highest layer order.

Implementation notes:
- `block.Region` keeps its method set; only the private `complexCode`/`multilayerCode`/`entry` readers
  change.
- Pad the data buffer by 8 bytes so bit reads are a single unaligned `binary.LittleEndian.Uint64`.
- Flat blocks are unchanged: their height stays in the index value.
- `geo/dynamic` door overlays copy cells out through `AppendCells`, so they don't notice the format.

Rejected:
- Dedup: 2% of complex and 17% of multilayer blocks, plus a hash and an extra indirection.
- zstd/LZ4/S2 per region or per block: no O(1) cell access, and CPU on every pathfinding step.
- Prediction from neighbouring cells: needs a sequential decode inside the block for a few percent
  more.

Gates:
- An exhaustive equivalence test decodes every block both ways and compares every cell, every layer,
  plus `Nearest`/`Above`/`Below` at sampled heights. It runs in CI against the datapack geodata.
- `geoprobe` oracle dumps unchanged.
- Pathfinding benchmark shows no meaningful regression.

### P2 — Template flyweight for immutable NPC state (−~70–100 MB)
- **Passive mods per template:** `TemplatePassiveMods` recompiles the template's passive skills and
  conditions for every NPC, and `AddMod` allocates a new slice per insert. Compile once per template into
  an immutable per-stat `[]Mod`; a `Calculator` starts pointing at that shared slice. No new copy-on-write
  machinery is needed: `AddMod` and `RemoveOwner` already build a fresh slice and swap `c.mods`
  (`skill/effect/mod.go:286-327`); verify every other writer (`RemoveStatsByOwner`) does the same.
  Precondition to verify: attach conditions evaluated per NPC at attach time (`Attachable`) give the same
  result for every NPC of a template; otherwise those mods stay per NPC.
- **Read-only stat reads stop creating calculators:** a nil slot computes with the shared base +
  builtin directly (today `MaxHP`/`MaxMP`/`RunSpeed` reads at spawn create calculators).
- **`statCalcs [122]*Calculator` inline array (976 B)** → one pointer to a lazily created slot array.
- **`cast.Controller.current skill.Definition` (832 B by value)** → `*skill.Definition` into the
  immutable table. Precondition: definitions are never mutated after load (table reloads swap the whole
  snapshot atomically today, so old pointers stay valid). Also cuts 832 B copies on every cast.
- **Config and logger copies:** `AIConfig`, raid multipliers, kill-reward config copied per NPC; three
  112 B `zerolog.Logger` copies per NPC → shared pointers; add the object id when logging (logging is rare).
- Gate: stat/formula oracle tests and combat full-flow scenarios unchanged; `make test-race`.

### P3 — Lazy combat components (−~100–150 MB)
Created on first use: threat and hate maps (nil-map reads are free in Go), `DesireQueue`, cast +
AI-cast controller, attack controller, `effect.List`, death rewards, held items when they equal
the template's, plus removing the throwaway `TerritoryMemory` that `KeepTerritory` replaces at attach.
`NewScratch` is **not** throwaway: it is kept when the slot has no Scratch and is the script-timer NPC
identity, so build it only when the slot provides none, before the NPC is published (#4398).
- Idle paths mutate these too: `enterInactiveRegion` calls `EffectList().StopAll()` and
  `clearAttackers()` for every NPC whose region goes inactive (`hostile.go:926-932`). Stop/clear paths
  need non-creating, nil-safe accessors, or the lazy objects get built for every NPC at 0 players.
- Ownership rule: these are touched from attackers' queues (stats, threat/hate, `attackedBy`), so
  each lazy field is either created under the lock that already guards it or published with
  `atomic.Pointer.CompareAndSwap` (loser discards its copy). No new goroutines, no new locks.
- Fold the pure wiring objects (`statOwnerRef`, `locatedRef`, `creatureActorRef`, `walkerActorRef`,
  `hostileControl`, `routeMove`) into the `Hostile` allocation itself.
- Gate: full-flow combat, cast, death/reward and AI scenarios; race tests that hit a cold NPC from
  two attackers at once.

### P4 — Broadcast without per-object known buffers (−~68 MB, plus churn)
Every network sink holds a `KnownBuffer` whose capacity grows to *every object* in its 3×3 regions,
then filters to `frameReceiver`s. That covers hostile and civilian (`folkSink`) NPCs, effect points,
decorations, doors, boats and summons. The 26 MB `appendObjectsExcept` site is the buffer's own `tracked`
slice, so the gain is ~42 + 26 MB. Instead:
- `world.Region` keeps a players-only slice next to `objects`. Leave `objects` and its swap-remove
  order untouched, because the order of packets a player gets on entering an area depends on it.
  The players slice changes under `r.mu` together with `objects`; today `playersCount` is updated after
  `r.mu` is released (`world/region.go:96-106`), so it can't be reused as is.
- Broadcast collects receivers from the 3×3 players lists into a pooled scratch (`sync.Pool` or a
  per-worker buffer), then fans out per the existing `CopyFrame` rule.
- Why packets are unchanged: every receiver gets the same bytes, and each client's own packet order is
  unchanged. Only the order in which *different* clients are handed their copies changes, and no client
  can observe that.
- Gate: broadcast frame-isolation tests; visibility/spawn-order tests unchanged.

### P5 — Allocation-free timers in `sim` (−~25 MB live, −~45% idle allocation)
Each NPC periodic script timer currently costs a `scriptTimer`, two map-of-map index entries, a
`sim.Timer`, a runtime `time.AfterFunc` timer, closures and a `q.timers` map entry. Every firing re-arms
by allocating all of the timer objects again.

**Reconsidered: timing-wheel libraries.**
- My earlier objection was wrong. `sim` already hides the clock behind `executor.afterFunc`:
  `Pool` → `time.AfterFunc`, `Inline` → its own virtual `vtimer` heap. A wall-clock wheel would only
  replace `Pool.afterFunc`, and tests keep driving `Inline`.
- Judged on their merits, the candidates still lose:
  - `RussellLuo/timingwheel`: runs every expiry in a new goroutine (`go t.task()`), allocates a new
    `Timer` per `AfterFunc`, has no `Reset`, and writes `t.expiration` from the task goroutine without
    synchronization.
  - `thinkgos/timer` (v0.10.1): no `Reset` (`SetDelay` + re-`AddTask` allocates a new entry);
    callbacks go through an `ants` goroutine pool, a new dependency plus a goroutine hand-off per
    expiry; `Periodic` rounds delays under 1 s up to 1 s.
  - `andy2046/tik`: manual stepping, but no confirmed `Reset`, and unmaintained.
- The Go runtime timer (1.23+) already uses per-P heaps, needs no goroutine per timer, and supports
  `Stop`/`Reset` in place without allocating. The measured cost was **allocation on every re-arm**,
  not timer-heap operations. With ~50k timers a heap operation is about 16 comparisons.

Plan:
1. Keep one `*sim.Timer` and one fire closure per script timer and re-arm it with a new
   reset-to-deadline method on the one-shot `sim.Timer` (#4389). The allocation is in the script layer:
   `sim.Queue.Every` already resets in place. Do **not** move script timers onto `Every`: it drops a tick
   while one is queued and skips missed deadlines, whereas script timers re-arm after the hook and catch
   up with `max(due−now, 0)`. This gets the re-arm path to 0 allocations; hook dispatch still allocates.
2. Script timer index: per-NPC slice on the NPC handle instead of `map[any]map[*scriptTimer]struct{}`;
   replace `q.timers map[*Timer]struct{}` with an intrusive list; shard or remove the single global
   `timers.mu`.
3. Dropped: an own timing wheel behind `Pool.afterFunc`. One ticker would round every `sim` timer
   deadline to its tick (~98 call sites, attack and cast hits included) and could reorder deadlines in
   the same slot, which clients can see. It would only save ~5 MB.
- Script timers **keep firing in inactive regions** (they do today, and the reference server
  behaves the same way); only their cost changes.
- Gate: sim timer ordering/cancel tests under `Inline`; `testing.AllocsPerRun` = 0 for the re-arm
  path; script timer scenarios with late hooks (catch-up unchanged).

### P6 — Small items (−~20–40 MB)
- Spawn slot key `fmt.Sprintf("%s#%d#%d")` strings + `map[string]slotInfo` (by-value `spawn.Entry`)
  → integer slot ids and pointers; queue id string built lazily (only used in logs).
- `world.Grid` builds 45,056 regions with eager `index` maps; most never hold an object → create the map on
  first `add`.
- Skill table `map[ID]map[int]Definition` → flat `[]Definition` sorted by (id, level) plus a small id
  index; hand out `*Definition`.
- Repeated template strings (AI params, names, HTML keys) → `unique.Make` (stdlib, Go 1.23+).

### P7 — Dormant NPCs (deep change, not recommended yet)
Represent NPCs in never-activated regions as a compact record (template, slot, position, heading,
object id) and hydrate into a full actor on demand. Gain would be ~9 KB → ~100 B per dormant NPC. Blocked:
periodic script timers run AI hooks on those NPCs, and world queries, GM commands and scripts address
them by id. Every one of those paths would need to hydrate first, which is a large parity risk. After
P2, P3 and P5 an idle NPC should cost ~3–4 KB, so revisit only if NPC count grows a lot.

### P8 — ECS (`github.com/mlange-42/ark`), reconsidered
Facts that decide it (ark v0.8.x docs):
- Archetype tables: rows are entities, columns are components.
- Adding or removing a component moves the entity to another archetype, at about 20 ns per
  component. That move is a world-wide structural change, so component addresses are not stable
  across it.
- The design page states: "Ark is not thread-safe". Concurrent queries are only allowed when they
  don't touch the same entities.
- Batch ops cost about 5 ns per entity; query iteration about 1 ns per entity.

How that maps onto this server:
- Today an NPC's state is mutated on its own `sim.Queue`, and also read and written from attackers'
  queues on any pool worker (stats, threat/hate, `attackedBy`).
- Code holds `*Hostile` and component pointers everywhere.
- Each client's packet order follows queue order.

Options:

| Option | What changes | Memory beyond P2+P3 | Verdict |
|---|---|---|---|
| A. Whole world in ark | Per-actor queues become phased systems (move → AI → combat → effects → network), parallelised by spatial partition. Region entities and ark relations split archetypes so partitions never touch the same entity. | ~1–1.5 KB/NPC: no per-object allocation headers or size-class rounding, no ~16 per-object mutexes, dense columns. About 35–50 MB at 35k NPCs. | **No.** Rewrites sim, AI, scripts and network sinks. Every send-order guarantee would need re-proving against Java. The gain is about 10% of what P1–P3 deliver. |
| B. ark as a single-owner dormant-NPC store, with P7 | One goroutine owns a World of dormant NPCs: components `{Slot, Template, Position, Heading, ObjectID}` plus a relation to their region entity. When a region activates, query by relation target and batch-hydrate into full actors. Single ownership sidesteps the thread-safety issue. | ~9 KB → ~150–200 B per dormant NPC | **Planned:** [dormant-npc-ark-plan.md](dormant-npc-ark-plan.md). It resolves the P7 blockers: timers run against a dormant handle with an escape hatch, lookups go through one resolver, and a measurement go/no-go gate comes first. |
| C. Plain struct-of-arrays columns for the sweeps, no library | AI tick and regen sweep read `{region, sleep latch, territory decision, regen phase}` from dense slices indexed by a slot id. | small | **Optional, for CPU** (#4388, #4390) more than memory. |

Conclusion: ark does not fit as the core model. It is the right tool if P7 is ever done (option B),
because there one owner, batch operations and relations are exactly what's needed.

## 4. Libraries considered

| Library | Use | Verdict |
|---|---|---|
| `golang.org/x/sys/unix` (already indirect) | `Mmap`, `Madvise` for P1 | **Adopt** (promote to direct) |
| stdlib `unique` | string interning (P6) | **Adopt** |
| stdlib `sync.Pool` | broadcast scratch (P4) | **Adopt** |
| `github.com/edsrzf/mmap-go` | mmap wrapper | Skip: x/sys covers it |
| `golang.org/x/exp/mmap` | read-only mmap | Skip: `ReaderAt` only, no `[]byte` view |
| `github.com/klauspost/compress` (zstd/s2) | compress geodata | Reject: no O(1) cell access; the lossless bit-packed encoding (P1b) already gets −61% |
| `github.com/RussellLuo/timingwheel` | timers (P5) | Reject: a goroutine per expiry, a `Timer` allocation per `AfterFunc`, no `Reset`, unsynchronized re-arm write. Any single-ticker wheel also rounds deadlines (client-visible), see P5. |
| `github.com/thinkgos/timer` | timers (P5) | Reject: no `Reset`, a new entry per add, adds the `ants` dependency and a goroutine hand-off per expiry, `Periodic` rounds up to 1 s |
| `github.com/mlange-42/ark` (ECS) | NPC storage | Core model: reject (not thread-safe, structural moves, ~10% of the P1–P3 gain for a full rewrite). As a single-owner dormant-NPC store with P7: candidate (§P8 option B). |
| `github.com/RoaringBitmap/roaring` | compressed int sets | Skip: no large id sets found on the heap |
| stdlib `weak` | caches | Skip: no cache that may drop entries |
| `GOEXPERIMENT=arenas` | bulk allocation | Reject: abandoned upstream |

## 5. Expected result

| | Now | After P0–P6 (estimate) |
|---|---|---|
| Live heap (idle) | 1.38 GB | ~0.4–0.5 GB |
| Heap target (GOGC=100) | 2.5 GB | ~0.9–1.0 GB, capped by GOMEMLIMIT |
| Per idle NPC | ~10 KB | ~3–4 KB |
| Geodata | 693 MB heap | 0 heap; ~254 MB evictable page cache (P1 + P1b) |
| Idle allocation | ~3 MB/s | <1 MB/s |
| Startup | 20–30 s (geodata decode) | geodata mmap is near-instant once the cache exists |

Estimates for P2–P6 come from heap-profile sites and struct sizes. Re-profile after each phase, with the
same 0-player baseline plus a bot-swarm run (#260).

## 6. Issues (fatal10110/acis_golang, milestone M14)

| Phase | Issue |
|---|---|
| P0 | fatal10110/acis_public#13 GOMEMLIMIT (moved from #4393) |
| P1 | #4394 geodata mmap cache (supersedes #4391) |
| P1b | #4395 compact geodata encoding |
| P2 | #4396 template passive mods / copy-on-write calculators; #4397 `skill.Definition` by value; #4398 per-NPC config/logger/wiring copies |
| P3 | #4392 lazy combat components (scope comment added) |
| P4 | #4399 per-NPC `KnownBuffer` → per-region player lists |
| P5 | #4389 script timer re-arm in place + index cost (scope comment added; no timing wheel) |
| P6 | #4400 spawn slot string keys; #4401 eager region index maps |
| CPU (same study) | #4388 AI territory checks, #4390 regen backstop |

Not filed: P6 string interning (no measurement yet), P7 dormant NPCs (not recommended).
