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

Deliverable of this plan: the design and phased migration. Umbrella issue #2268; Phase 0 #2285;
phases #2269–#2274.

Revision 3 (#2284) adds, on top of #2280: Phase 0 prerequisites (non-blocking outbox, world lock
collapse, perf baseline, standalone fixes), the cross-actor **command** class (53 sites that mutate
queue-owned state and become posts), worker fairness, a real-pool test gate next to `Inline`,
sequencing with `docs/actor-model-degoifying-plan.md`, and two corrections (player send path
blocks today; `hitLocked`'s `ReduceHP` is the caster's own cost).

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
  effect list. *Landed (Phase 3) as several per-actor leaf locks — vitals, progression, state,
  stat slots, controllers, AI brains, hate, the effect list — not one `vitalsMu`; see Phase 3's
  Landed as notes and #2273 for the list.* Other actors mutate these **synchronously** — `target.ReduceHP` still returns, the
  caller still reads `died`, reflect/counter still land in the same call stack, exactly as the
  reference. `vitalsMu` is never held across a call-out; side effects (Die sequence, stat
  recalculation, effect-icon broadcast, StatusUpdate) are posted to the owner's queue after
  unlock. Hot scalars (HP/MP/CP, position, dead, target id) are also readable off-queue via
  atomics; derived combat stats and abnormal-effect flags are an `atomic.Pointer` snapshot
  republished by the owner queue on recalculation (equip/buff/level), not per hit.
- **Containers** (mutex, short critical section, no call-outs): `world.State` (collapsed from 5
  lock kinds to `State.mu` plus leaf `Region.mu`/`Presence.latch`, see Phase 0),
  `zone.Index`/`Zone`, `task.GroundItems`, `trade.Book`, **`Inventory`**
  (it crosses actors: trade, pickup, pet, warehouse, persistence), `idfactory`,
  `activeRegistry`/`deadlineRegistry`, `ClientRegistry`, `Session`/`Conn` send path, `LoginLink`,
  `netutil.FloodGuard`, `logging`. Target: ~135 → ~25.
- **Cross-actor commands** (posts): 53 call sites in `skill/` + `handler/` mutate the *target's*
  queue-owned state from another actor's goroutine — `StopCast`, `InterruptCast`, `AbortAll`,
  `StopMove`, `StopAttack`, `TeleportTo`, `Think`, `TryToAttack`/`TryToIdle`, `FleeFrom`,
  `AddAttackDesire`, `Sit`/`SetStanding`, `SetTarget`/`ClearTarget`, `StartFakeDeath`,
  `AddExpAndSP` to party members, `IncreaseCharges`, `ReduceDeathPenaltyLevel`, … Every one is
  fire-and-forget today (no caller reads a result; the three `Think()` results are discarded with
  `_ =`), so each becomes `target.Queue().Post(func() { … })` with an on-arrival re-check (still
  alive, still the same live object). This is not the rejected async-mutation design: nothing a
  caller reads back is deferred, only commands nobody reads back, and by one hop. *Superseded
  (#2271 slice 3):* these commands stay synchronous under their existing leaf locks; see Phase 3.
  The reference does not defer them (`AbstractAI.doIntention` is `synchronized`, `notifyEvent`
  runs inline, nothing in `ai/type` uses `ThreadPool.execute`). Status flags set by effects
  and read by formulas (`invul`, `paralyzed`, `immobilized`, `teleporting`, sitting) are atomics
  (#2262 item 9), settable from any goroutine. New multi-actor state ported later (party, clan,
  duel, olympiad, siege) is a container with its own short mutex, never a field on one actor that
  others reach into.

Parallelism across actors is preserved and improved: N players in one town = N queues drained in
parallel by the pool. Today's world `RWMutex`/region locks, which serialize every
spawn/move/visibility update in a hot spot *while running observer callbacks*, become one short
critical section with no callbacks inside. Blocking work never runs on a queue: DB I/O goes to
the persist worker, and sends never block. Today only the broadcast hook uses
`Session.TrySendFrame`; the player's own `Character.SendFrame` → `Session.SendFrame` → `Conn.send`
blocks on a 64-slot channel ([conn.go:200](internal/gameserver/network/conn.go#L200)) until the
writer drains — on a queue that would stall a pool worker. Phase 0 replaces the channel with an
unbounded per-connection outbox and a byte high-water kick, after which every send path is
non-blocking and `TrySendFrame` (with #2254's spin) is deleted.

Enforced rules (`sim.AssertOwner(q)` under the `simdebug` build tag; CI runs suites with it):
1. Queue-owned state is touched only from its owner's queue.
2. No mutex (`vitalsMu` or container) is held while calling into an actor or another container.
   Posting to a queue is non-blocking, so posting under a lock is allowed.
3. Mutexes do not nest. The single documented exception: trade commit takes both parties'
   inventory locks in object-id order.
4. Nothing blocking (DB, a send that can wait on the client, channel receive) runs on a queue. The
   pool logs any task over 50 ms as a bug.
5. A worker drains at most `drainSlice` (64) tasks from one queue before requeueing it at the back
   of the run queue, so a boss under 50 attackers cannot starve the other queues.

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

## Phase 0 — standalone prerequisites (#2285; not blocked on `sim`, one PR each, start now)

- **Non-blocking per-connection outbox.** Replace `Conn.out`'s 64-slot channel with an unbounded
  FIFO drained by `writeLoop`, aborting the connection at a byte high-water mark (the policy
  `TrySendFrame` applies today at 64 frames). `SendFrame` never blocks, `TrySendFrame` and the
  `trySendLockAttempts` spin go away (retires #2254), and an EnterWorld burst over 64 frames cannot
  kick a client. No wire change; a slow client is kicked at N bytes instead of 64 frames.
- **World lock collapse** (was Phase 5 sweep 6; closes #2260). One `State.mu`; `Discover`/`Forget`
  and activity callbacks are collected under the lock and fired after unlock in every path. Today
  `regionActivityMu` is held across them for players
  ([visibility.go:243–337](internal/gameserver/world/visibility.go#L243)) and `DespawnAll` sorts
  and holds N `transitionMu` at once ([visibility.go:126–137](internal/gameserver/world/visibility.go#L126)).
  Needs nothing from `sim`.
  *Landed (#2324) as:* `State.mu` serializes every change of region membership and activity, and
  every callback runs after it is released. Two per-object/per-region locks remain, both leaves
  held only around a few stores or a slice update: `Presence.latch` (a CAS) lets a `Move` that stays
  in its region skip `State.mu`, and `Region.mu` lets known-list scans read a region without it.
  A single world lock measured 4–14x slower at 8 cores on same-region `Move` and `AppendKnown`.
- **Perf baseline harness.** N scripted clients through the `tests/` boot path (real packets, real
  MariaDB) in one region doing move + attack for a fixed duration; record CPU, p50/p99 handler
  latency, GC pause on #2268. Run on `main` before Phase 2; re-run after Phases 2, 3 and 5.
  Without it "no regression" is unverifiable.
- **Already-filed standalone fixes, do now:** #2259 (move `ReduceHP`/`ReduceMP`/`ConsumeItem`
  after `cast.Controller.mu` unlock — this is the caster paying its own skill cost, intra-actor,
  the same lock-across-re-entrant-call class as #2258/#2261) and #2262 (13 single-field atomics).

## Sequencing with `docs/actor-model-degoifying-plan.md` (#2276)

Both plans rewrite `model/actor/{cast,attack,move}/controller.go` and
`network/{character_flow,live_player,summon_spawn}.go`. Phases 0, 1 and 4 any time; **Phase 2
waits for #2278** (Phase B, `event.Sink`) — one `Emit` per actor is where the per-actor outbox
lands, and routing ~67 hook fields onto queues only to delete them is waste. #2277 and #2279 are
independent of this plan.

## Phases (one sub-issue and one PR per phase; every PR keeps behavior and `go test -race ./...` green)

Numbering matches issues #2269–#2274. **Execution order is 0 ∥ 1 → 4 → 2 → 3 → 6 → 5**: Phase 0
and Phase 1 in parallel; the persist worker (Phase 4) lands before handlers move onto a cores-sized
pool (Phase 2), so no queued task can block on the DB; the test cleanup (Phase 6) runs before the
lock sweeps (Phase 5) so the sweeps' dual-executor gate is deterministic.

*Status (2026-09-24):* Phases 0–4 landed (#2323–#2325, #2322, #2344, #2365/#2366, #2446–#2449).
Phase 3 landed with cross-actor **leaf locks** instead of command posts and a single `vitalsMu`
per actor (see its *Landed as* notes); Phase 5 is rescoped accordingly on #2273.

### Phase 1 — `internal/gameserver/sim` (new package, no wiring) — #2269
- `Pool` (worker goroutines, `Start(ctx)`/`Stop(ctx)`: stop accepting posts, drain in-flight
  tasks, return), `Queue` (per-actor FIFO: `Post(fn) bool`, `After(d, fn) Timer`,
  `Every(d, fn) Ticker`, `Close()`), `Clock` (`Now`, injectable), `AssertOwner(q)`.
- Lifecycle: `Close()` cancels armed timers and tickers; `Post` on a closed queue drops the task
  and returns false (attacking a mob that just died is the common case — callers ignore it). A
  relogging player gets a new live object and a new queue; closures holding the old pointer check
  detached/closed on entry and no-op, as stale `time.AfterFunc` callbacks do today.
- Fairness: `drainSlice` (64) tasks per drain, then requeue at the back if still non-empty.
- `AssertOwner`: the queue holds `draining sync.Mutex` while a worker drains it, so
  `if q.draining.TryLock() { q.draining.Unlock(); panic }` is a zero-cost check in every build that
  catches "queue idle, state touched from elsewhere". Under `simdebug` it additionally maps worker
  goroutine id (`runtime.Stack` parse, ~1 µs) → draining queue to catch "queue busy on another
  worker".
- Slow-task watchdog: log any task over 50 ms with the queue id.
- `Inline` mode for tests: a single-threaded run loop with one global FIFO. `Post` appends and
  never runs the task inline, even from the test goroutine; `Run()` drains until idle;
  `Advance(d)` moves the clock, fires due timers/tickers in deadline order, then drains. Same
  "later, FIFO" ordering as production, only deterministic. What `Inline` cannot reproduce is two
  queues draining concurrently against a shared `vitalsMu` or container lock, so from Phase 2 on
  the behavior suites also run on a real `Pool` (`GOMAXPROCS=1` and default).
- Reuse: `commons/scheduler.Ticker` API shape and its panic-recover per callback
  ([ticker.go](internal/commons/scheduler/ticker.go)); the `scheduledTimer`/`afterFunc` seam the
  controllers already expose ([attack/controller.go:118](internal/gameserver/model/actor/attack/controller.go#L118),
  [cast/controller.go:145](internal/gameserver/model/actor/cast/controller.go#L145),
  [move/creature.go:89](internal/gameserver/model/actor/move/creature.go#L89)).
- Tests: FIFO order, one-drainer-per-queue under `-race` (two workers over 1k queues), timer
  cancel, post-after-close drops, `drainSlice` fairness (10k posts on one queue do not delay a second
  queue's single post past one slice), Inline determinism, `AssertOwner` panics off-queue in the
  plain build (idle queue) and under `simdebug` (queue busy on another worker).

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
- Pre-conditions: Phase 4 merged; Phase 0 outbox merged and perf baseline recorded on #2268;
  #2278 merged. Any sync DB call still reachable from a queued task is a bug the watchdog will
  surface.
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
  `internal/gameservertest/boot.go` (mode selectable per run: `sim.Inline` or a real `Pool`).
- Gate: unit + behavior suites, `-race`, `-tags simdebug`, in **both** modes — `sim.Inline`
  (deterministic) and a real `Pool` with `GOMAXPROCS=1` and default (real interleavings); perf
  baseline re-run with no p99 regression; no packet-byte change.
  *Landed (#2270) as:* the queue lives on `creature.Live` (players, NPCs) and `summon.Actor`
  (the owner's queue); `SetQueue` also routes the movement, attack and cast controllers' timers
  and the effect list's ticks onto it, and `Queue()` is part of every `task` actor interface.
  The connection goroutine posts each in-world frame's work to the player's queue and **waits**
  for it before reading the next frame (`network.onLive`), so frames keep read order,
  flood/decode gates and disconnect returns stay on the connection goroutine, and
  Logout/Restart run the exit check and detach on the queue while the persistence wait stays
  on the connection. Ticks post only each actor's share (drain + send, save, flag update);
  registries keep their lock and bookkeeping. The login-link reader needed no change: it
  resolves pre-world auth only. The harness picks the executor with
  `ACIS_SIM_EXECUTOR=pool|inline` (`gameservertest.SimExecutorEnv`); `Server.Settle` waits
  for posted work. Synchronous DB calls still reachable from queue tasks are #2364.
  *Panic policy (#2394):* a task a connection **waits** on (`onLive`/`onQueue`) that panics ends
  that session — the pool still recovers and logs the panic, and `onLive` reports the failure so
  the dispatch loop returns and its deferred detach saves and detaches the character, exactly as a
  fatal decode error does. A half-applied mutation the client was never told about must not be
  replayable against a live session. Fire-and-forget work (`postLive`) and timer/tick expiries keep
  the pool's own recovery alone: nothing waits on them, and dropping a session because a tick
  panicked would be a regression in the other direction.

### Phase 3 — cross-actor boundary (synchronous subset + inventory container) — #2271
- Introduce `vitalsMu` on `Character`/`Hostile`/`summon.Actor` guarding HP/MP/CP, dead, hate,
  effect list. `ReduceHP`/`ReduceMP`/`Die`/`Revive`/effect apply/remove mutate under it and return;
  every hook they fire today moves to a post on the owner's queue after unlock.
- Atomics for HP/MP/CP, position, dead, target id; `atomic.Pointer` stat snapshot republished on
  recalculation (reuse `player.Vitals`, `npcinfo.Snapshot`, `attack.Snapshot`).
  *Landed as (#2271 slice 4):* the cross-mutable subset already sits under per-actor leaf locks
  that other actors take synchronously — `Character.vitalsMu`, `Hostile`'s `creature.Health`,
  `mpMu` and `deathMu`, `summon.Actor`'s `vitals.mu`/`statusMu`, `ai.Attackable.mu` for hate, the
  effect list's own lock — none held across a call-out. Position and heading are the
  `world.Presence` seqlock; `dead`, status flags and abnormal masks are atomics; stat reads go
  through per-stat `effect.Calculator` locks. No read moved to an atomic or `atomic.Pointer`
  snapshot: the per-hit reads are uncontended `RLock`s and the perf re-run shows no regression.
  The audit of every cross-actor surface (`attackable.Combatant`, `creature.FormulaActor`,
  `effect.Actor`, the skill handlers' `Creature`/`Player`/`NPC`/`Summon`, `CharInfo`/`NpcInfo`
  builders) found three unguarded groups, now fixed:
  - progression and karma. A death runs on the killer's queue (or the victim's, for a DOT), so
    the victim's exp, level and karma loss and the killer's PK/PvP counters are written off their
    owner's queue. `Character.progressionMu` now also guards `KarmaPoints`, `PvPKills` and
    `PKKills`; `Level`, `Karma` and `ProgressionValues` read under it, and every progression
    change runs its events and level refreshes after unlock (`progressionHooks`, closes #2258).
    Progression therefore stays a cross-actor lock, not queue-owned state.
  - `item.SpoilPool` and `npc.SeedState`. Spoilers, sowers and harvesters act from their own
    queues while the killer's fills and reads them; both take a mutex, and `Mark`, `Sow` and
    `ClaimHarvest` check and claim in one step.
  - `RaiseDeathPenaltyLevel` read the effect list and the killer while holding `stateMu`; it now
    gathers those first.
- Observers: region `OnInactiveRegion` posts the NPC reset (effects stopped, AI back to peace) to
  the NPC's queue with an on-arrival re-check that the region is still inactive; `OnActiveRegion`
  only clears an atomic latch and stays inline. *Landed as:* `world.Observer.Discover/Forget`
  stay synchronous on the goroutine that drives the transition — the reference sends object info
  inline from the moving thread, and the subject's next updates (move, status) go out from that
  same goroutine, so posting info to the observer's queue would let them overtake it. What those
  callbacks read of the other actor is covered by the atomics/stat-snapshot item below. Zone
  enter/exit watchers have no production registration yet; a script that adds one posts its own
  work.
- Cross-actor commands: convert the 53 sites (list them in the PR with
  `rg -oh 'target\.[A-Z][A-Za-z]+\(' internal/gameserver/skill internal/gameserver/handler | sort | uniq -c`,
  classified into `vitalsMu` subset / atomic flag / container / command post) to
  `target.Queue().Post(...)` with an on-arrival re-check. A site whose result turns out to be read
  by the caller joins the `vitalsMu` subset instead; note each such case on #2271.
  *Landed as:* no command is posted; each one stays synchronous on the caller's goroutine under
  the target's existing leaf lock. Posting changes what clients see. The reference runs an
  effect's `abortAll` → `tryToIdle` → `updateAbnormalEffect` in the caster's call stack, so
  `StopMove`/`MagicSkillCanceled` reach observers before the effect icon. A post also lands
  behind a hit expiry the target's queue already holds, so under backlog a stunned target's cast
  still lands, while a synchronous `sim.Timer.Stop` cancels that expiry the way
  `ScheduledFuture.cancel` does. Every wired command either sends packets (stop/abort/interrupt,
  retarget, stance, charges, death penalty, teleport, flight) or is read back by the next
  aggressor (a summon's target), so none qualifies. The audit found one unguarded pair,
  `summon.Actor` `target`/`intent`, now under `stateMu`. Commands still stubbed on live actors
  (`AbortAll`, `StopMove`, `StopAttack`, `ClearTarget`, `TryToIdle`, `FleeFrom` on player/NPC)
  follow the same rule when they are wired: they run synchronously under the controller's lock.
- Trade commit and ground pickup as described above.
- Gate: `tests/combat`, `tests/skills`, `tests/trade`, `tests/items` green under `-race` and
  `-tags simdebug` in both `sim.Inline` and real-pool modes; a scenario per converted flow asserting
  per-client packet order (attack → target StatusUpdate → kill reward) under `sim.Inline` and
  re-run on the pool; perf baseline re-run.

### Phase 5 — mutex deletion sweeps (one PR per group; deleted locks' writers get `sim.AssertOwner`) — #2273
*Rescoped 2026-09-24.* Phase 3 kept the cross-actor state under per-actor leaf locks, so this
phase deletes only the locks whose every site runs on the owner's queue (handlers via `onLive`,
timers via `queue.After`, ticks via `Post`), and puts `sim.AssertOwner(q)` at each former lock
site so the `simdebug` pool run catches an off-queue writer. It does not fold leaf locks into one
`vitalsMu`: that widens critical sections for no correctness gain. The deletable candidates and
the locks that stay are listed on #2273; `AssertOwner` has no production caller until this phase.
1. `model/actor/player` + `network/live_player.go`: `livePlayer`'s per-connection locks and the
   `Character` locks not named in Phase 3's *Landed as*. `vitalsMu`, `stateMu` and `progressionMu`
   stay: progression is cross-actor (death exp/karma loss and PK/PvP credit run on another actor's
   queue, see Phase 3); #2258 closed with Phase 3 slice 4.
2. `model/actor/npc` + `summon` + `cubic`: shot, disabled-skill and overhit locks where no other
   actor reaches them. `Hostile`'s health/`mpMu`/`deathMu`/`minionsMu`, `summon.Actor`'s
   `statusMu`/`stateMu` (target and intent are read by the next aggressor, Phase 3 slice 3) and the
   `ai` brain locks stay. `npc.SeedState.mu` and `item.SpoilPool.mu` (Phase 3 slice 4) guard
   per-life NPC state that sowers, harvesters, spoilers and the killer change from their own queues
   and stay cross-actor. `Hostile.claimFollowSlot` holds `minionsMu` across the world lock while it
   resolves occupants (breaks rule 2): snapshot ids under the lock, resolve after unlock, re-check
   on claim.
3. `move`/`attack`/`cast`. The move/attack/cast controller locks stay as leaf container
   locks, because other actors stop, abort and interrupt them synchronously (Phase 3, landed).
   The same goes for the AI brains' locks (`ai.Attackable`, `ai.PlayerAttack`, `ai.Summon`) and
   the `stateMu` sections guarding target, stance, charges and summon intent. The sweep still
   moves `Start` and `Hit` onto the caster's queue, where their `ConsumeItem`/`ReduceMP`/`ReduceHP`
   are the **caster's own** skill cost under its own `vitalsMu` (#2259 was fixed in Phase 0). The
   `cubic` locks are sweep 2's.
4. `skill/effect` and stat slots: the `List` lock, `scheduleMu`, the `Calculator` locks and the
   `statMu` that guards each actor's calculator slots (`Character.statMu`, `Hostile.statMu`)
   **stay** — other actors apply and remove effects synchronously (Phase 3 *Landed as*: "the
   effect list's own lock" is one of the cross-actor leaf locks), and every `Stat()` read from an
   attacker's formula goes through `statCalc` under `statMu`. Nothing to delete here beyond
   confirming no lock is held across a call-out.
5. `model/itemcontainer` + `model/item`: `Inventory` keeps `inv.mu` as a container; per-item
   `item.Instance` locks deleted (mutated only under the owning inventory's mutex). Closes #2261:
   `BuildAndDrainUpdates` returns the batch under `inv.mu` and the caller runs its callback after
   unlock, on the owner queue.
   *Landed as:* `BuildAndDrainUpdates` snapshots the items and drains the queue under both
   container locks, runs the build unlocked, and puts the drained updates back on a failed build.
   The `item.Instance` lock stays: the item-persistence flush reads instances on a persistence
   lane, and a relogging session claims pending rows from its connection goroutine, so it is not
   owner-queue state. `Container.mu` and `Inventory.mu` stay as containers.
6. `zone` actor/flags (2) — `network.liveZoneActor.mu` may be dead weight, `zone.Flags` keeps its
   paired-read reason from #776 unless the single-goroutine caller removes it. (The `world` collapse
   moved to Phase 0.)
7. `task`: registries stay as containers; delete per-task locks that only guarded actor calls.
8. Remaining single-field guards from #2262 that survive to this point → atomics or deletion.

### Phase 6 — test cleanup — #2274 (runs before Phase 5)
- Replace the `afterFunc` test seams and the `time.Sleep`/`Eventually`/`waitFor` waits in
  `tests/`, `internal/gameservertest` and the remaining `internal/**/*_test.go` with `sim.Inline` +
  `Clock.Advance` (counts on `main` at ef80df83: 52 `afterFunc` lines, 95 + 41 waits; re-count per
  PR). Retire the `ACIS_SIM_EXECUTOR=inline` pump once a suite drives the clock itself, and move
  the raw `time.AfterFunc` left in `network/summon_spawn.go` (pet-restore hold ceiling) and the
  controllers' nil-`afterFunc` fallbacks onto `queue.After` so `Inline` can advance them. Only
  needs `queue.After` in production (Phase 2), so it does not wait for Phase 5.
  *Slice 1 (`tests/combat`, `tests/skills`, `tests/pets`) landed as:* `gameservertest.DriveClock()`
  in a suite's `TestMain` runs its queues on `sim.Inline` whose clock moves only through
  `Server.Advance`/`AdvanceUntil` and through client reads, which step to the next timer while no
  frame is on its way. Before the clock moves the harness waits until the server has handled every
  client frame (`network.Conn.ObserveReads`, frame counts on both ends) and flushes the persistence
  lanes a test does not hold, so a database round trip takes no virtual time; restore-window tests
  hold the lane instead of slowing the store. Effect periods read the owner queue's clock
  (`sim.Queue.Now`; a signet point runs on its own queue), and the pet-restore
  hold ceiling moved onto `queue.After`. CI keeps these suites' real-pool run as a
  `ACIS_SIM_EXECUTOR=pool` step. Other wall-clock reads (reuse, disabled items, cast interrupt)
  still use `time.Now` (#2482).
  *Slice 2 (`internal/gameserver/network`) landed as:* production always hands the attack, cast
  and move controllers a queue, so their three private `time.AfterFunc` fallbacks collapse onto
  `sim.AfterOr(nil, …)`, reached only by unit tests that build an actor with no queue. The never-set
  `GameClientLink.afterFunc`/`cubicAfterFunc` seams are gone. The package's other waits are not on
  a timer: three 2 s waits were false passes, where `ScriptedClient.ExpectClosed` treated a read
  timeout as a close. They now pin the exact threshold, and a read timeout fails the test. The
  remaining polls (login link, session validator, the item-flush order) wait on a socket or a
  goroutine the test starts, not on a queue timer. Close-path frames are tracked in #2484.
  *Slice 3 (`tests/character`, `tests/items`, `tests/lifecycle`, `tests/trade`) landed as:* every
  behavior suite now runs on the driven clock, so it is the harness default and `DriveClock()` is
  gone; the wall-clock `ACIS_SIM_EXECUTOR=inline` pump is retired (the runner goroutine still runs
  posted tasks, but only a test moves the clock). Pool task-panic recovery tests and `tests/perf`
  pin `WithRealPool()`. A connection whose handler waits for saves on a lane the test holds
  counts as caught up (`network.Conn.ObservePersistWaits` reports the wait's owners), so the clock
  can move past a held database round trip; read loops are bounded on the read's clock
  (`ScriptedClient.Now`), not the wall clock. CI's
  real-pool step covers all seven suites.
  *Slice 4 (`model/actor/*` and the nil-queue fallbacks) landed as:* the attack, cast and move
  controllers and the cubic runtime hold their owner's `*sim.Queue` and arm every timer with
  `queue.After`. The `afterFunc` seams, their loggers and `sim.AfterOr`/`sim.Now` are gone, as are
  network's no-queue branches in `onQueue`/`postLive`: a link's `Queues` is now required. Unit tests
  build actors on a `sim.Inline` queue and drive it with `Advance`, including the NPC wander
  recheck, walker arrival and signet tick tests that used to wait out 1–2 s wall-clock timers.
  #2488 then made a queue required for every live actor: the task registries post to
  `Queue()` unconditionally, the hostile NPC reset and summon offensive-follow ticker have no
  queue-less path, `NewGameClientLink` and `manager.NewNpcs` reject a missing queue source, a
  signet effect point gets its own queue (closed on despawn) instead of ticking on the effect
  task's goroutine, and an effect list reads only its owner queue's clock.

## Performance notes (why this does not regress the hot spot)
- Pool size = cores; per-actor queues are independent, so contention is bounded by the per-actor
  leaf locks (`vitalsMu` and its siblings, a few instructions per hit, no call-outs) and the
  container mutexes, each now a map op with no
  callbacks — strictly less than today's world/region RWMutex sections that run observer hooks inside.
- Per-message cost is a closure allocation (~100 ns–1 µs), below packet encode cost. Stat
  snapshots allocate only on recalculation; per-hit reads/writes are atomics.
- Long inline compute (geopath) stays on the queue initially; if the watchdog shows worker
  starvation, it moves to a compute pool with a reply post — the queue model makes that a local change.
- Queues are unbounded with a high-water log; `drainSlice` keeps one hot queue from starving the
  rest; inbound volume is already capped by the read loop's flood protection (`client_loop.go`),
  and internal posts are not client-controlled.
- The per-connection outbox is unbounded with a byte high-water kick, so a queue task never waits
  on a client; a slow client costs memory up to the mark, then its connection, never a worker.

## Verification
- Every PR: `gofmt`, `go vet ./...`, `go build ./...`, `go test -race ./...` and
  `go test -race -tags simdebug ./...` with the shared MariaDB up (`make test-db-up`).
- Phase 0: perf baseline numbers recorded on #2268 before Phase 2 starts; outbox test that a
  stalled reader is aborted at the high-water mark and a 200-frame burst to a healthy reader lands
  in order.
- Phase 1: two workers over 1k queues under `-race` prove one-drainer-per-queue; `AssertOwner`
  panics when queue-owned state is touched off-queue (plain and `simdebug` builds).
- Phase 2+: behavior suites in both `sim.Inline` and real-pool modes; perf baseline re-run per
  phase.
- Phase 3: behavior suites in `tests/combat`, `tests/skills`, `tests/trade`, `tests/items`; a
  scenario per converted flow asserting per-client packet order under `sim.Inline`; a trade
  scenario where one party drops an offered item between offer and confirm.
- Phase 4: shutdown with online players saves every one before exit; relog mid-fight.
- End state check: `rg -n "sync\.(RW)?Mutex" internal/gameserver/model internal/gameserver/skill`
  reports only the container locks and the cross-actor leaf locks listed on #2273 (per-actor
  vitals/progression/state, stat slots (`statMu`), controllers, AI brains, hate/threat, the effect
  list and calculators, `SeedState.mu`, `SpoilPool.mu`), each commented with the caller or container it serves, and
  every queue-owned field has `sim.AssertOwner` at its writers.
- Manual: run two clients in one region, trade, fight an NPC, relog mid-fight (autosave/detach
  ordering) per `docs/run-servers.md`.

## Out of scope
- Region sharding across processes/loops (possible later on top of queues; not needed now).
- Behavior or wire changes; any observed packet-order difference is a bug to fix, not to accept.
- Async cross-actor mutation with reply messages: rejected — the reference is synchronous
  everywhere and the port's handlers read the target right after mutating it.
