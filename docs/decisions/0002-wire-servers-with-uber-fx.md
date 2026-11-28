# 0002. Wire the servers with Uber Fx

- **Status:** Accepted
- **Date:** in place before the 2026-09-05 infrastructure review
- **Sources:** [agents/go-style.md](../agents/go-style.md) (Packages, files, and wiring;
  Dependencies), [infra-review-2026-09-05.md](../infra-review-2026-09-05.md),
  `cmd/gameserver/main.go`, `internal/gameserver/app/app.go`

## Context

The game server is built from well over a hundred services: config, logger, database pool, data
tables, stores, the world, the actor pool, and one service per game system. Many of them start
background work, and almost all of them must stop in the right order. On shutdown, for example,
players must be saved before the persistence lanes close, and the lanes must be flushed before the
database pool closes. Each step also needs a time limit, so a slow database cannot stop the process
from exiting.

The test harness must boot the same server, with a few parts swapped (database, clock, listener,
logger).

## Decision

Each binary's `main` package is its composition root. It builds the process as an
[Uber Fx](https://github.com/uber-go/fx) graph:

- constructors (`fx.Provide`) build services from their dependencies;
- `fx.Invoke` functions start them, in a fixed order, and register stop hooks;
- Fx runs the stop hooks in reverse order, under one overall deadline
  (`gameServerStopTimeout`), which is the sum of each step's own limit.

The providers that both production and tests need live in `internal/gameserver/app`, so a new
provider reaches both with one edit. Domain packages never import Fx; they take plain constructor
arguments.

## Consequences

- Start and stop order is written down in one place and enforced by Fx.
- The test harness (`internal/gameservertest`) boots the production graph and replaces only its
  seams, so tests exercise the real wiring.
- Wiring mistakes show up when the graph is built, not at compile time. Tests catch them early:
  `TestGameServerGraphValidates` checks the graph without a database, a test checks that the
  shutdown deadline covers every stop hook, and a divergence test lists the providers not yet
  moved into `internal/gameserver/app`.
- Contributors must learn Fx's provide/invoke model to add a service.

## Alternatives considered

- **Hand-written wiring in `main`.** It is explicit and checked by the compiler, but with this many
  services the start order, the reverse stop order and the per-step limits would be hand-kept, and
  the test harness would need a second copy of the wiring.
- **A home-grown dependency container.** The style guide rules this out: when an adopted library
  already owns a problem, we do not build our own framework for it.
