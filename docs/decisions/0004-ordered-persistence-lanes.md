# 0004. Write to the database through ordered persistence lanes

- **Status:** Accepted
- **Date:** 2026-09 (phase 4 of the concurrency refactor)
- **Sources:** [concurrency-refactor-plan.md](../concurrency-refactor-plan.md) (Phase 4, #2272),
  `internal/gameserver/persist`

## Context

Players and game systems save state all the time: autosave, logout, pet saves, item batches, and the
game-wide services (Olympiad, heroes, raid points, lottery). Two rules matter:

1. **A save must not block game work.** After [0003](0003-per-actor-queues.md), game logic runs on a
   small pool of workers. If a task waited on a slow database, it would hold a worker, and every
   actor behind it would stall.
2. **Saves for one owner must land in order.** If a player's logout save could overtake an earlier
   autosave, the older data would win. Before this decision a per-player save mutex (`saveMu`)
   enforced the order.

## Decision

Game code does not write to the database itself. It builds a snapshot of what to save and enqueues
a job on `persist.Worker`.

The worker has five lanes, each a goroutine with an unbounded FIFO:

- four lanes for owners (characters, items, clans, ...), chosen by the owner id, so all jobs for
  one owner go to the same lane, in order, without a goroutine per player;
- one lane of its own for the game-wide services (`persist.ServiceOwner`), so a slow service write
  never delays a player's save.

`Enqueue` never blocks the caller. At shutdown the lanes are flushed before the database pool closes.

## Consequences

- Game tasks never wait on the database for writes.
- A later save for an owner can never land before an earlier one, so the per-player save mutex is
  gone.
- Because the queue is unbounded, a database that falls behind shows up as a growing backlog, not
  as stalled players.
- A save is not on disk when `Enqueue` returns. Code that must know the write finished (logout,
  shutdown) waits for the lane explicitly.
- Tests can hold a lane (`HoldPersistenceLane`) to test what happens while a write is still
  outstanding.

## Alternatives considered

- **Write inline from game code.** Simple, but it blocks a pool worker for the length of a database
  round trip, which breaks the queue model.
- **One goroutine per player for saves.** It keeps order, but costs a goroutine for every player
  online. Hashing owners onto a few lanes keeps the order with a fixed cost.
- **A bounded queue that blocks when full.** It would push database slowness back into the game
  loop, which is what this decision avoids.
