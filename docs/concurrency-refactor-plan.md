# Concurrency refactor: per-actor owner queues instead of per-object mutexes

## Context

Audit of `acis_golang` on current `main` (see issues #2258–#2263 and the closed #769 series):
116 mutex fields, ~840 lock sites, ~30 lock objects per online player. No lock-order inversion
exists inside any function, and every controller already fires hooks after unlock — but
deadlock-freedom rests on comments ("callbacks must not reposition", "build must not re-enter")
and on the 4 goroutine kinds that mutate one actor (connection read loop, 21 ticker goroutines,
11 `time.AfterFunc` timer callbacks, login-link reader) never colliding in a new way.

Goal: remove the sea of locks from actor/domain packages without losing multi-core performance
for many concurrent players in the same or different locations, and without introducing races.

Deliverable of this plan: the design and phased migration. No code in this turn.

## Target model: keyed serial executors ("owner queues")

Every live actor (player, NPC, summon) owns a FIFO **queue**. All mutation of an actor's state
runs on its queue — its own packet handlers, its timers, its periodic ticks. Queues are drained
by a fixed **worker pool** (`GOMAXPROCS` goroutines); a queue is never drained by two workers at
once. This gives:

- **Zero mutexes for intra-actor state** (Character's 11, livePlayer's 7, Hostile's 6, summon's 5,
  move/attack/cast/cubic's 4, effect list + calculators, item instances): one writer per actor,
  enforced structurally. Replaces ~80% of today's lock sites.
- **Parallelism across actors** is preserved and improved: N players in one town = N queues drained
  in parallel by the pool. Today's world `RWMutex`/region locks, which serialize every
  spawn/move/visibility update in a hot spot *while running observer callbacks*, become one short
  critical section with no callbacks inside.
- **Cross-actor mutation** becomes a message to the target's queue (async, FIFO per target).
  Cross-actor **reads** use published immutable snapshots (`atomic.Pointer[T]`, republished by the
  owner queue after each mutation). Both are lock-free.
- **Blocking work never runs on a queue**: DB I/O goes to a persist worker (FIFO per owner id,
  which also fixes the autosave/detach ordering that `saveMu` exists for); sends from a queue are
  non-blocking (`Session.TrySendFrame`, already the visibility policy — slow client is kicked).

What **stays a mutex** (genuinely shared containers, short critical sections, no call-outs):
`world.State` (collapsed from 5 lock kinds to 1), `zone.Index`/`Zone`, `task.GroundItems`,
`trade.Book`, `idfactory`, the generic `activeRegistry`/`deadlineRegistry`, `ClientRegistry`,
`Session`/`Conn` send path, `LoginLink`, `netutil.FloodGuard`, `logging`. Target: ~116 → ~15.

Enforced rules (debug build/tests, via `sim.AssertOwner(q)`):
1. Actor state is touched only from its queue.
2. A container mutex is never held while calling into an actor; posting to a queue is
   non-blocking, so posting under a container lock is allowed.
3. Container locks do not nest (world ↔ zone ↔ ground items), so no ordering to document.
4. Nothing blocking (DB, `Session.SendFrame`) runs on a queue.

## Semantic change and the decision point

Cross-actor mutation lands asynchronously (microseconds, FIFO). Call sites: `target.ReduceHP` ×4,
`target.ReduceMP` ×4, `target.Die` ×2, `target.ReduceHPByDOT`, `target.Revive`, `target.SetTarget`,
plus effect application onto a target and hate updates (the latter already run inside the target's
`ReduceHP`, so they land on the target's queue for free). Per-client packet order is unchanged
(each client's stream is produced by its own queue; a post-after-send keeps Attack before the
target's StatusUpdate). What changes is control flow: "did the target die?" becomes a reply
message to the attacker (kill reward, aggro clear) instead of a return value.

Decision point after Phase 3's spike: if the combat behavior suites (`tests/combat`, `tests/skills`)
show an ordering regression that cannot be expressed as a reply message, the fallback is to keep
**one** mutex per actor guarding only the cross-mutable subset (HP/MP/CP, dead, hate) with
synchronous calls — still one lock per actor instead of ~19, and everything else lock-free.

## Phases (one umbrella issue, one sub-issue and one PR per phase; every PR keeps behavior and
`go test -race ./...` green)

### Phase 1 — `internal/gameserver/sim` (new package, no wiring)
- `Pool` (worker goroutines, `Start(ctx)`/`Stop`), `Queue` (per-actor FIFO: `Post(fn)`, `After(d, fn) Timer`,
  `Every(d, fn) Ticker`, `Close()`), `Clock` (`Now`, injectable), `AssertOwner(q)` (debug: worker
  goroutine id → draining queue map; no-op in release), and `Inline` mode for tests (Post runs
  synchronously; re-entrant posts append to a local FIFO drained on return; manual `Advance(d)`).
- Reuse: `commons/scheduler.Ticker` API shape and its panic-recover per callback
  ([ticker.go](internal/commons/scheduler/ticker.go)); the `scheduledTimer`/`afterFunc` seam the
  controllers already expose ([attack/controller.go:118](internal/gameserver/model/actor/attack/controller.go#L118),
  [cast/controller.go:145](internal/gameserver/model/actor/cast/controller.go#L145),
  [move/creature.go:89](internal/gameserver/model/actor/move/creature.go#L89)).
- Tests: FIFO order, one-drainer-per-queue under `-race`, timer cancel, Inline determinism.

### Phase 2 — route all actor work onto queues (locks untouched, now uncontended)
- Create a queue at live-actor construction, close at despawn/detach:
  players in `network/character_flow.go` (~L477–494, where `creature.NewLive`/controllers are built),
  NPCs in `data/manager/npcs_hostile.go` (~L136–183), summons in `network/summon_spawn.go` (~L300–332).
- Packet handlers: `client_loop.go` keeps decode + gates on the connection goroutine, then
  `live.queue.Post(handler)` for in-world opcodes; pre-world handlers (character list/select/restore
  DB reads at `character_flow.go:59–158`, `client_loop.go:265`) stay on the connection goroutine.
- Timers: pass `queue.After` as the `afterFunc` of every controller; replace the raw
  `time.AfterFunc` at `character_shortbuff.go:60`, `character_charges.go:108`, `hostile.go:965`,
  `cubic/runtime.go:59`, `network/dispatch.go:371,435` (`scheduleAfter`, `cubicAfterFunc`).
- Tickers: the 21 `scheduler.Start` sites (`cmd/gameserver/tasks.go`, `task/*.go`,
  `summon/live_lifecycle.go`, `ai/summon.go`) keep one ticker goroutine each, but a tick fans out
  `actor.queue.Post(actor.Tick)` instead of calling the actor inline; registries keep their
  container mutex.
- Login-link reader (`network/loginlink.go:132`) posts to the affected client's queue or stays on
  `Client.mu` (pre-world state only).
- Composition root: `cmd/gameserver/tasks.go` (`startTicker`), `cmd/gameserver/network.go`,
  `internal/gameservertest/boot.go` (use `sim.Inline` so behavior suites are deterministic).
- Gate: unit + behavior suites, `-race`; no packet-byte change.

### Phase 3 — cross-actor boundary (spike first, then decision point above)
- Publish snapshots: `Character`/`Hostile`/`summon.Actor` republish an immutable status snapshot
  (`atomic.Pointer`) after mutation on their queue; reuse existing value types
  (`ResourceValues`, `ProgressionValues`, `NPCInfoSnapshot`, `item.Instance.Snapshot`).
- Convert the cross-actor mutation call sites to `target.Queue().Post(...)`; attack-hit/kill-reward
  and effect-apply flows become reply messages to the caster/attacker queue.
- Observers: `world.Observer.Discover/Forget` implementations post to the observer's queue
  (contract at [world/visibility.go:18](internal/gameserver/world/visibility.go#L18) already
  forbids blocking); zone enter/exit watchers and region `OnActive/OnInactiveRegion` likewise.
- Two-actor transactions: trade — `trade.Book` (mutex) validates against both parties' snapshots,
  then posts remove/add to each party's queue (items are already reserved while a trade is open);
  ground pickup — `GroundItems` claims under its mutex, then posts "add to inventory" to the player
  queue (claim-then-deliver, no double pickup).
- Gate: `tests/combat`, `tests/skills`, `tests/trade`, `tests/items` green under `-race`.

### Phase 4 — persistence worker
- `internal/gameserver/persist`: one goroutine per FIFO keyed by owner id; queues receive
  snapshots built on the owner queue. Autosave (`network/taskeffects.go:194`) and detach
  (`network/lifecycle.go:35`) enqueue in order → `saveMu` deleted (closes #2263). Item batching
  (`task/iteminstances.go`) and `flushItemPersistence` route through it.

### Phase 5 — mutex deletion sweeps (one PR per group; each site becomes `sim.AssertOwner`)
1. `model/actor/player` + `network/live_player.go` (18 locks; closes #2258 by construction).
2. `model/actor/npc` + `ai` + `summon` (16).
3. `move`/`attack`/`cast`/`cubic` (6; closes #2259 — `hitLocked`'s ReduceHP is now a post).
4. `skill/effect` (List, Calculator, Effect schedule) (4).
5. `model/itemcontainer` + `model/item` (3; closes #2261 — `BuildAndDrainUpdates` runs on the owner queue).
6. `world`: collapse `transitionMu`/`Presence.mu`/`Region.mu`/`Region.activityMu`/`regionActivityMu`
   to one `State.mu` with no callbacks inside (closes #2260). `zone` actor/flags (2).
7. `task`: registries stay as containers; delete per-task locks that only guarded actor calls.
8. Remaining single-field guards from #2262 that survive to this point.

### Phase 6 — test cleanup
- Replace the 34 `afterFunc` test seams and the 73 `time.Sleep`/`Eventually` waits in
  `tests/` and `internal/gameservertest` with `sim.Inline` + `Clock.Advance`.

## Performance notes (why this does not regress the hot spot)
- Pool size = cores; per-actor queues are independent, so contention is bounded by the container
  mutexes only, each now a map op with no callbacks — strictly less than today's world/region
  RWMutex sections that run observer hooks inside.
- Per-message cost is a closure allocation (~100 ns–1 µs), below packet encode cost.
- Long inline compute (geopath) stays on the queue initially; if profiling shows worker starvation,
  it moves to a compute pool with a reply message — the queue model makes that a local change.
- Queues are unbounded with a high-water log; inbound volume is already capped by the read loop's
  flood protection (`client_loop.go`), and internal posts are not client-controlled.

## Verification
- Every PR: `gofmt`, `go vet ./...`, `go build ./...`, `go test -race ./...` with the shared MariaDB
  up (`make test-db-up`; container `acis-test-mariadb` is currently healthy).
- Phase 2+: a `sim`-level test that runs two workers over 1k queues under `-race` to prove
  one-drainer-per-queue; a debug-mode test that `AssertOwner` panics when actor state is touched
  off-queue.
- Phase 3: behavior suites in `tests/combat`, `tests/skills`, `tests/trade`, `tests/items`; add
  a scenario per converted flow that asserts per-client packet order (attack → target
  StatusUpdate → kill reward) using `sim.Inline` determinism.
- End state check: `rg -c "sync\.(RW)?Mutex" internal/gameserver/model internal/gameserver/skill`
  reports zero; container packages keep their documented mutex.
- Manual: run two clients in one region, trade, fight an NPC, relog mid-fight (autosave/detach
  ordering) per `docs/run-servers.md`.

## Out of scope
- Region sharding across processes/loops (possible later on top of queues; not needed now).
- Behavior or wire changes; any observed packet-order difference is a bug to fix, not to accept.
