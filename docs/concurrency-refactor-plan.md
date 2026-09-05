# Concurrency refactor: per-actor owner queues instead of per-object mutexes

## Context

Audit of `acis_golang` on current `main` (see issues #2258–#2263 and the closed #769 series):
135 mutex fields, ~920 lock sites, ~30 lock objects per online player. No lock-order inversion
exists inside any function, and every controller already fires hooks after unlock — but
deadlock-freedom rests on comments ("callbacks must not reposition", "build must not re-enter")
and on the 4 goroutine kinds that mutate one actor (connection read loop, 21 ticker goroutines,
15 raw `time.AfterFunc` sites, login-link reader) never colliding in a new way.

Goal: remove the sea of locks from actor/domain packages without losing multi-core performance
for many concurrent players in the same or different locations, without introducing races, and
without changing the reference's synchronous call semantics.

Deliverable of this plan: the design and phased migration. Umbrella issue #2268; phases #2269–#2274.

## Target model: keyed serial executors ("owner queues")

Every live player and NPC owns a FIFO **queue**. All mutation of an actor's own state runs on
its queue — its packet handlers, its timers, its periodic ticks. Summons, pets and cubics run on
their owner's queue (pet inventory lives in the owner inventory; `summon/` calls into the owner at
23 sites; the reference runs both in the owner's context). Queues are drained by a fixed
**worker pool** (`GOMAXPROCS` goroutines); a queue is never drained by two workers at once.

State splits into three kinds:

- **Queue-owned** (no lock): progression, controllers (move/attack/cast/cubic), AI, stat
  calculators, shortcuts, quests, henna, macros, item instances reachable only through the owner's
  inventory, effect timers. Touched only on the owner queue; enforced by `sim.AssertOwner`.
  Replaces ~80% of today's lock sites.
- **Cross-mutable subset** (one small mutex per actor, `vitalsMu`): HP/MP/CP, dead flag, hate list,
  effect list. Other actors mutate these **synchronously** — `target.ReduceHP` still returns, the
  caller still reads `died`, reflect/counter still land in the same call stack, exactly as the
  reference. `vitalsMu` is never held across a call-out; side effects (Die sequence, stat
  recalculation, effect-icon broadcast, StatusUpdate) are posted to the owner's queue after
  unlock. Hot scalars (HP/MP/CP, position, dead, target id) are also readable off-queue via
  atomics; derived combat stats and abnormal-effect flags are an `atomic.Pointer` snapshot
  republished by the owner queue on recalculation (equip/buff/level), not per hit.
- **Containers** (mutex, short critical section, no call-outs): `world.State` (collapsed from 5
  lock kinds to 1), `zone.Index`/`Zone`, `task.GroundItems`, `trade.Book`, **`Inventory`**
  (it crosses actors: trade, pickup, pet, warehouse, persistence), `idfactory`,
  `activeRegistry`/`deadlineRegistry`, `ClientRegistry`, `Session`/`Conn` send path, `LoginLink`,
  `netutil.FloodGuard`, `logging`. Target: ~135 → ~25.

Parallelism across actors is preserved and improved: N players in one town = N queues drained in
parallel by the pool. Today's world `RWMutex`/region locks, which serialize every
spawn/move/visibility update in a hot spot *while running observer callbacks*, become one short
critical section with no callbacks inside. Blocking work never runs on a queue: DB I/O goes to
the persist worker; sends from a queue use `Session.TrySendFrame` (already the visibility policy —
a slow client is kicked).

Enforced rules (`sim.AssertOwner(q)` under the `simdebug` build tag; CI runs suites with it):
1. Queue-owned state is touched only from its owner's queue.
2. No mutex (`vitalsMu` or container) is held while calling into an actor or another container.
   Posting to a queue is non-blocking, so posting under a lock is allowed.
3. Mutexes do not nest. The single documented exception: trade commit takes both parties'
   inventory locks in object-id order.
4. Nothing blocking (DB, `Session.SendFrame`, channel receive) runs on a queue. The pool logs any
   task over 50 ms as a bug.

## Cross-actor calls stay synchronous

Call sites: `target.ReduceHP` ×4 in `handler/skill/damage.go`, `caster.ReduceHP` reflect at
`damage.go:364`, `target.Die` ×2, `target.ReduceHPByDOT`, `target.Revive`, effect application onto
a target, hate updates. Every one keeps its return value; `applyLethalHit` still reads the target
right after `ReduceHP` (`damage.go:141–143`). No reply messages, no spike, no decision point.

What changes is *where side effects run*: `ReduceHP` under `vitalsMu` sets HP/dead and returns;
if the target died, the caller (already on its own queue) does kill reward / aggro clear
synchronously, and the target's Die sequence (drops, decay, observer broadcast) runs as a post on
the target's queue. Per-client packet order is unchanged (each client's stream is produced by its
own queue; Attack goes out before the target's StatusUpdate because the post lands after the send).

Two-actor item flows use the inventory container, not snapshots:
- **Trade** — `trade.Book` holds the offer; commit locks both inventories in object-id order,
  re-validates against live contents, moves items, unlocks, then posts inventory-changed hooks to
  both owners' queues. (No item reservation exists today; `Book.locked` is the confirm-lock on the
  offer, so validate-then-post would be a double-spend window.)
- **Ground pickup** — on the player's queue: capacity check as today, claim under `GroundItems.mu`,
  add under `inv.mu`; a failed add after a claim puts the item back on the ground.

## Phases (one sub-issue and one PR per phase; every PR keeps behavior and `go test -race ./...` green)

Numbering matches issues #2269–#2274. **Execution order is 1 → 4 → 2 → 3 → 5 → 6**: the persist
worker (Phase 4) lands before handlers move onto a cores-sized pool (Phase 2), so no queued task
can block on the DB.

### Phase 1 — `internal/gameserver/sim` (new package, no wiring) — #2269
- `Pool` (worker goroutines, `Start(ctx)`/`Stop(ctx)`: stop accepting posts, drain in-flight
  tasks, return), `Queue` (per-actor FIFO: `Post(fn) bool`, `After(d, fn) Timer`,
  `Every(d, fn) Ticker`, `Close()`), `Clock` (`Now`, injectable), `AssertOwner(q)`.
- Lifecycle: `Close()` cancels armed timers and tickers; `Post` on a closed queue drops the task
  and returns false (attacking a mob that just died is the common case — callers ignore it).
- `AssertOwner`: worker goroutine id (`runtime.Stack` parse, ~1 µs) → draining queue map,
  compiled in only under `simdebug`; release builds get a no-op.
- Slow-task watchdog: log any task over 50 ms with the queue id.
- `Inline` mode for tests: a single-threaded run loop with one global FIFO. `Post` appends and
  never runs the task inline, even from the test goroutine; `Run()` drains until idle;
  `Advance(d)` moves the clock, fires due timers/tickers in deadline order, then drains. Same
  "later, FIFO" ordering as production, only deterministic — so ordering bugs cannot pass in
  Inline and fail on the pool.
- Reuse: `commons/scheduler.Ticker` API shape and its panic-recover per callback
  ([ticker.go](internal/commons/scheduler/ticker.go)); the `scheduledTimer`/`afterFunc` seam the
  controllers already expose ([attack/controller.go:118](internal/gameserver/model/actor/attack/controller.go#L118),
  [cast/controller.go:145](internal/gameserver/model/actor/cast/controller.go#L145),
  [move/creature.go:89](internal/gameserver/model/actor/move/creature.go#L89)).
- Tests: FIFO order, one-drainer-per-queue under `-race` (two workers over 1k queues), timer
  cancel, post-after-close drops, Inline determinism, `AssertOwner` panics off-queue under `simdebug`.

### Phase 4 — persistence worker — #2272 (runs second)
- `internal/gameserver/persist`: a fixed small pool (4 lanes); `Enqueue(ownerID, job)` hashes the
  owner id to a lane, so jobs for one owner are FIFO without a goroutine per player. Jobs carry
  snapshots built by the caller (under today's locks now; on the owner queue after Phase 2;
  inventory snapshots under `inv.mu`).
- Autosave (`network/taskeffects.go:194`), detach (`network/lifecycle.go:35`), pet save
  (`lifecycle.go:154`), item batching (`task/iteminstances.go`) and `flushItemPersistence` route
  through it. Detach enqueues the final save then the offline-status write, in order → `saveMu`
  deleted (closes #2263).
- `Flush(ctx)` for shutdown. Composition root (`cmd/gameserver`) shutdown order: stop accepting
  connections → stop tickers → `pool.Stop(ctx)` (Phase 2+) → enqueue final saves for every online
  player → `persist.Flush(ctx)` → close DB.
- Gate: relog mid-fight and autosave-during-detach scenarios in `internal/gameservertest`; `-race`.

### Phase 2 — route all actor work onto queues (locks untouched, now uncontended) — #2270
- Pre-condition: Phase 4 merged. Any sync DB call still reachable from a queued task is a bug the
  watchdog will surface.
- Create a queue at live-actor construction, close at despawn/detach:
  players in `network/character_flow.go` (~L477–494, where `creature.NewLive`/controllers are built),
  NPCs in `data/manager/npcs_hostile.go` (~L136–183). Summons (`network/summon_spawn.go` ~L300–332)
  and cubics receive the owner's queue; no queue of their own. NPC respawn creates a new object
  ([npcs_respawn.go:11](internal/gameserver/data/manager/npcs_respawn.go#L11)), hence a new queue.
- Packet handlers: `client_loop.go` keeps decode + gates on the connection goroutine, then
  `live.queue.Post(handler)` for in-world opcodes; pre-world handlers (character list/select/restore
  DB reads at `character_flow.go:59–158`, `client_loop.go:265`) stay on the connection goroutine.
- Timers: pass `queue.After` as the `afterFunc` of every controller; replace the 15 raw
  `time.AfterFunc` sites (`character_shortbuff.go:60`, `character_charges.go:108`, `hostile.go:965`,
  `cubic/runtime.go:59`, `network/dispatch.go:371,435` (`scheduleAfter`, `cubicAfterFunc`), and the
  rest from `rg -n "time\.AfterFunc" internal cmd`).
- Tickers: the 21 `scheduler.Start` sites (`cmd/gameserver/tasks.go`, `task/*.go`,
  `summon/live_lifecycle.go`, `ai/summon.go`) keep one ticker goroutine each, but a tick fans out
  `actor.Queue().Post(actor.Tick)` instead of calling the actor inline; summon ticks post to the
  owner's queue; registries keep their container mutex.
- Login-link reader (`network/loginlink.go:132`) posts to the affected client's queue or stays on
  `Client.mu` (pre-world state only).
- Composition root: `cmd/gameserver/tasks.go` (`startTicker`), `cmd/gameserver/network.go`,
  `internal/gameservertest/boot.go` (use `sim.Inline` so behavior suites are deterministic).
- Gate: unit + behavior suites, `-race`, `-tags simdebug`; no packet-byte change.

### Phase 3 — cross-actor boundary (synchronous subset + inventory container) — #2271
- Introduce `vitalsMu` on `Character`/`Hostile`/`summon.Actor` guarding HP/MP/CP, dead, hate,
  effect list. `ReduceHP`/`ReduceMP`/`Die`/`Revive`/effect apply/remove mutate under it and return;
  every hook they fire today moves to a post on the owner's queue after unlock.
- Atomics for HP/MP/CP, position, dead, target id; `atomic.Pointer` stat snapshot republished on
  recalculation (reuse `player.Vitals`, `npcinfo.Snapshot`, `attack.Snapshot`).
- Observers: `world.Observer.Discover/Forget` implementations post to the observer's queue
  (contract at [world/visibility.go:18](internal/gameserver/world/visibility.go#L18) already
  forbids blocking); zone enter/exit watchers and region `OnActive/OnInactiveRegion` likewise.
- Trade commit and ground pickup as described above.
- Gate: `tests/combat`, `tests/skills`, `tests/trade`, `tests/items` green under `-race` and
  `-tags simdebug`; a scenario per converted flow asserting per-client packet order
  (attack → target StatusUpdate → kill reward) under `sim.Inline`.

### Phase 5 — mutex deletion sweeps (one PR per group; each site becomes `sim.AssertOwner`) — #2273
1. `model/actor/player` + `network/live_player.go` (18 locks → `vitalsMu`; closes #2258 —
   progression is queue-owned, level-up hooks run on the queue with nothing held).
2. `model/actor/npc` + `ai` + `summon` (16 → `vitalsMu`; summon state is owner-queue-owned).
3. `move`/`attack`/`cast`/`cubic` (6 → 0; closes #2259 — `hitLocked` runs on the caster's queue
   with no controller lock, `ReduceHP` takes only the target's `vitalsMu`).
4. `skill/effect`: `List`'s own mutex goes (guarded by its owner's `vitalsMu`); `Calculator` and
   the effect schedule become queue-owned (timers via `queue.After`).
5. `model/itemcontainer` + `model/item`: `Inventory` keeps `inv.mu` as a container; per-item
   `item.Instance` locks deleted (mutated only under the owning inventory's mutex). Closes #2261:
   `BuildAndDrainUpdates` returns the batch under `inv.mu` and the caller runs its callback after
   unlock, on the owner queue.
6. `world`: collapse `transitionMu`/`Presence.mu`/`Region.mu`/`Region.activityMu`/`regionActivityMu`
   to one `State.mu` with no callbacks inside (closes #2260). `zone` actor/flags (2).
7. `task`: registries stay as containers; delete per-task locks that only guarded actor calls.
8. Remaining single-field guards from #2262 that survive to this point → atomics or deletion.

### Phase 6 — test cleanup — #2274
- Replace the 34 `afterFunc` test seams and the 73 `time.Sleep`/`Eventually` waits in
  `tests/` and `internal/gameservertest` with `sim.Inline` + `Clock.Advance`.

## Performance notes (why this does not regress the hot spot)
- Pool size = cores; per-actor queues are independent, so contention is bounded by `vitalsMu`
  (a few instructions per hit, no call-outs) and the container mutexes, each now a map op with no
  callbacks — strictly less than today's world/region RWMutex sections that run observer hooks inside.
- Per-message cost is a closure allocation (~100 ns–1 µs), below packet encode cost. Stat
  snapshots allocate only on recalculation; per-hit reads/writes are atomics.
- Long inline compute (geopath) stays on the queue initially; if the watchdog shows worker
  starvation, it moves to a compute pool with a reply post — the queue model makes that a local change.
- Queues are unbounded with a high-water log; inbound volume is already capped by the read loop's
  flood protection (`client_loop.go`), and internal posts are not client-controlled.

## Verification
- Every PR: `gofmt`, `go vet ./...`, `go build ./...`, `go test -race ./...` and
  `go test -race -tags simdebug ./...` with the shared MariaDB up (`make test-db-up`).
- Phase 1: two workers over 1k queues under `-race` prove one-drainer-per-queue; `AssertOwner`
  panics when queue-owned state is touched off-queue.
- Phase 3: behavior suites in `tests/combat`, `tests/skills`, `tests/trade`, `tests/items`; a
  scenario per converted flow asserting per-client packet order under `sim.Inline`; a trade
  scenario where one party drops an offered item between offer and confirm.
- Phase 4: shutdown with online players saves every one before exit; relog mid-fight.
- End state check: `rg -n "sync\.(RW)?Mutex" internal/gameserver/model internal/gameserver/skill`
  reports only `vitalsMu` (one per actor type) and `Inventory`/container locks; nothing else.
- Manual: run two clients in one region, trade, fight an NPC, relog mid-fight (autosave/detach
  ordering) per `docs/run-servers.md`.

## Out of scope
- Region sharding across processes/loops (possible later on top of queues; not needed now).
- Behavior or wire changes; any observed packet-order difference is a bug to fix, not to accept.
- Async cross-actor mutation with reply messages: rejected — the reference is synchronous
  everywhere and the port's handlers read the target right after mutating it.
