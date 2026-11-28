# 0007. Test behavior end to end against a real database and a virtual clock

- **Status:** Accepted
- **Date:** consolidated by issues #1677 and #1682
- **Sources:** [agents/test-strategy.md](../agents/test-strategy.md),
  [agents/verification.md](../agents/verification.md), `docker-compose.test.yml`,
  `internal/gameservertest`, `internal/testsupport`

## Context

A game server's correctness is mostly visible from outside: which packets a client gets, in what
order, what changes in the world, and what is saved. Early tests were mostly unit tests with fakes.
Fakes drifted from the real code. In one case a fake reimplemented the charge cap of a player's
skill charges, and the real logic changed while the fake did not, so the test kept passing.

Timing is the other problem. Much of the game runs on timers (regeneration, buffs, respawns, boss
phases). Tests that sleep on the wall clock are slow and flaky.

## Decision

Most coverage comes from **behavior suites** in `tests/<domain>/`. Each test:

1. boots a real game server in the test process (`gameservertest.Boot`), using the production Fx
   graph ([0002](0002-wire-servers-with-uber-fx.md));
2. drives it with a scripted client over the real wire protocol (`testsupport.ScriptedClient`);
3. checks three things: the packets the client receives, the world state, and the database rows.

The database is a real MariaDB. One shared instance (`make test-db-up`) serves every test package;
each package or test gets its own uniquely named database on it.

Actor queues run on `sim.Inline`, a single goroutine whose clock moves only when the test says so
(`srv.Advance`, `srv.AdvanceUntil`). Tests never sleep. CI also runs every suite on the real worker
pool and wall clock, to catch problems the inline run would hide.

Only pure functions (codecs, ciphers, formulas, config parsing) get direct unit tests, in
`<pkg>_core_test.go` files. A fake may replace a real type only when no real type exists yet, or for
a deliberate seam such as the clock or random numbers.

## Consequences

- Tests check what players and operators actually see, so refactors that keep behavior keep the
  tests green.
- Tests are deterministic, and scenarios that wait out long timers still run quickly.
- Running most tests needs Docker for the test database.
- A behavior that cannot be triggered through a client packet or a domain entry point cannot be
  tested this way. That is treated as a gap to fix, not a reason for a white-box test.
- Expected packet bytes must come from known-good fixtures, never from the encoder under test.

## Alternatives considered

- **Unit tests with fakes for each package.** Fast, but the fakes drift, and they do not prove that
  the packets and database rows are right.
- **A test container per package.** Each package would start its own MariaDB. One shared instance
  with a database per test is much faster and lets several worktrees share it.
- **Real-time tests with sleeps.** Simple to write, but slow and flaky. The virtual clock makes long
  timers testable in milliseconds.
