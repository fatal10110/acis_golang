# 0003. Run each actor's work on its own queue instead of locking every object

- **Status:** Accepted. Phases 0 to 4 landed by 2026-09-24; later phases continue on the umbrella
  issue.
- **Date:** 2026-09
- **Sources:** [concurrency-refactor-plan.md](../concurrency-refactor-plan.md), umbrella issue
  #2268, phases #2269–#2274, `internal/gameserver/sim`

## Context

The first version of the port protected game state the way the Java server does: a mutex on almost
every object. An audit counted 135 mutex fields, about 920 lock sites, and about 30 lock objects per
online player.

No deadlock had been found, but nothing guaranteed that. Safety depended on comments such as
"callbacks must not reposition", and on four kinds of goroutine that could change the same actor
(the connection read loop, 21 ticker goroutines, 15 raw `time.AfterFunc` timers, and the login
link reader) never meeting in a new way. Every new feature added risk.

The server must also use every CPU core when many players act at once, in the same place or in
different places.

## Decision

Each live actor (player or NPC) owns a FIFO queue (`sim.Queue`). Everything that changes the actor's
own state runs as a task on that queue: its packet handlers, its timers and its periodic ticks.
Summons, pets and cubics run on their owner's queue.

A fixed pool of worker goroutines, one per CPU (`GOMAXPROCS`), drains the queues. A queue's tasks run
one at a time and in order, and two workers never drain the same queue at once. To stay fair, a
worker runs at most 64 tasks from one queue before moving it to the back of the line.

A few values change across actors: when one actor hits another, the target's HP changes at once and
the attacker reads the result, as in the reference. These values sit behind small per-actor locks
that are never held while calling other code. Values that other actors only read (position, HP,
status flags) are also published through atomics.

`sim.AssertOwner` checks that queue-owned state is only touched from its queue. Building with
`-tags simdebug` makes the check stricter.

## Consequences

- Most actor state has no lock at all, and code that runs on an actor's queue can rely on nothing
  else changing that actor at the same time.
- Work for different actors still runs in parallel on every core.
- Tests run the same queues on `sim.Inline`, a single goroutine with a virtual clock, which makes
  them deterministic ([0007](0007-behavior-first-tests.md)). CI also runs every suite on the real
  pool.
- A task on a queue must never block, for example on the database. That is why database writes go
  through [0004](0004-ordered-persistence-lanes.md), and why handlers that read the database do
  the read off the queue and post the result back.
- Contributors must know which queue owns a piece of state. Code that reaches into another actor
  must use that actor's locked or atomic accessors, or post to its queue.

## Alternatives considered

- **Keep a mutex per object.** It is familiar, but the deadlock risk grows with every feature, and
  the number of locks per player costs memory and time.
- **Change other actors only by sending them messages and waiting for replies.** Rejected: the
  reference is synchronous everywhere, and the port's handlers read the target right after
  changing it. Making those changes asynchronous would change behavior
  ([0001](0001-stay-compatible-with-acis.md)).
- **Split the world into regions with one loop each.** Possible later on top of queues, but not
  needed now. The plan lists it as out of scope.
