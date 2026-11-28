# Development guide

This guide takes you from a fresh clone to a tested change. If you have not read
[architecture.md](architecture.md) yet, read it first: it explains how the code fits together.
[CONTRIBUTING.md](../CONTRIBUTING.md) covers how to propose a change.

## Your first build and test

You need Go (the version in `go.mod`), Docker with Compose v2, and `make`.

```bash
git clone https://github.com/fatal10110/acis_golang.git
cd acis_golang
```

Build everything:

```bash
go build ./...
```

Run the fast tests. They need no database:

```bash
make test-unit
```

Start the test database, then run one behavior suite:

```bash
make test-db-up
```

```bash
make test-one PKG=tests/items
```

If all of that passes, your setup works.

## Tools you need

| Tool | Version | Used for |
| --- | --- | --- |
| Go | the version in `go.mod` (1.26) | building and testing |
| Docker with Compose v2 | any recent version | the test database, and the Docker stack |
| golangci-lint | v2 (CI uses v2.14) | formatting and lint, in the pre-commit hook and in CI |
| MariaDB client | optional | loading the schema by hand to run the servers without Docker |

## The checkout and its neighbors

This repository works on its own for building and testing. It carries a copy of the game data in
`aCis_datapack/` (XML, HTML and SQL, but no geodata), and the tests use it.

A few tasks need more. Put this repository in a parent folder next to the full datapack and the
aCis Java sources:

```text
acis_public/
├── acis_golang/        this repository
├── aCis_datapack/      the full datapack, with geodata: run servers from this one
└── aCis_gameserver/    the aCis Java server: reference sources and its config/ folder
```

| To | You need |
| --- | --- |
| build, and run the unit tests and behavior suites | this repository and Docker |
| run the servers | the full datapack with geodata, and a `config/` folder (the Java server's, or `ops/docker/config/`) |
| run the geodata tests | geodata, found in a parent folder or in `ACIS_GEODATA_DIR` |
| run the reference tests | the `aCis_gameserver/` sources |

Do not run servers from the in-repo `aCis_datapack/`. Servers write into `data/` (crests,
announcements), and CI checks that copy against `ops/datapack.sha256`
([decision 0011](decisions/0011-in-repo-datapack-copy.md)).

To run the servers, follow [run-servers.md](run-servers.md) or [ops/docker.md](ops/docker.md).
[tools.md](tools.md) describes every program under `cmd/`.

## Tests

### How the tests work

Most tests are *behavior suites* in `tests/<area>/`, such as `tests/items` or `tests/clan`. Each
test does what a player would do:

1. it starts a real game server inside the test process;
2. it connects a scripted client that sends real packets;
3. it checks the packets the client receives, the state of the world, and the rows in the database.

The database is a real MariaDB. Time is a *virtual clock* that moves only when the test says so,
which makes tests fast and repeatable. Only small pure functions, such as packet codecs and damage
formulas, have direct unit tests. [Decision 0007](decisions/0007-behavior-first-tests.md) explains
why, and [agents/test-strategy.md](agents/test-strategy.md) has the full rules.

### The test database

All database tests share one MariaDB on `127.0.0.1:3307`. Each test package creates its own
database on it, so parallel runs, and several checkouts, can share it.

```bash
make test-db-up
```

```bash
make test-db-down
```

The database runs with host networking. On Docker Desktop, turn on host networking, or point the
tests at another MariaDB with `ACIS_TEST_MARIADB_DSN` (default `root:test@tcp(127.0.0.1:3307)/`).

### Which tests to run

| Command | Runs | Needs the database |
| --- | --- | --- |
| `make test-unit` | the pure-function packages; use it while you edit | no |
| `make test-one PKG=tests/items` | one behavior suite. Add `RUN=TestName` for one test | yes |
| `make test-internal` | every package outside `tests/` | yes |
| `make test` | everything | yes |
| `make test-race` | everything, with the race detector. **Run this before you open a pull request.** | yes |
| `make fuzz` | feeds random bytes to the packet decoders, 60 seconds per target. `FUZZ=Target` picks one; `FUZZTIME=5m` changes the time | no |

You can also call `go test` directly:

```bash
go test ./tests/skills -run TestLearnSkillRejectsInsufficientSP -count=1
```

### Test settings

| Variable | What it does |
| --- | --- |
| `ACIS_TEST_MARIADB_DSN` | where the test database is |
| `ACIS_SIM_EXECUTOR` | `inline` (default): actor queues run on one goroutine with a virtual clock. `pool`: the real worker pool and the real clock. CI runs every suite both ways. |
| `ACIS_REQUIRE_DATAPACK` | `1`: a test that cannot find the datapack fails instead of skipping. CI sets it. |
| `ACIS_REQUIRE_REFERENCE` | `0`: tests that need the Java sources skip when they are missing. CI sets it. When unset, it follows `ACIS_REQUIRE_DATAPACK`. |
| `ACIS_DATAPACK` | datapack location, for tools and tests that cannot find one |
| `ACIS_GEODATA_DIR` | geodata location, for the geodata tests |
| `ACIS_PERF_*` | number of clients, duration and other sizes for the `tests/perf` scenarios |

CI cannot run the geodata tests or the reference tests, because it has neither geodata nor the Java
sources. If you change geodata code, or something the reference tests cover, run them on your machine
and check in the `-v` output that they ran: a skipped test proves nothing.
[agents/verification.md](agents/verification.md#datapack-geodata-and-reference-tests) lists them.

### Writing a test

- Put a new feature's test in the matching `tests/<area>/` package. Start the server with
  `gameservertest.Boot` and talk to it through `testsupport.ScriptedClient`. Do not write your own
  handshake.
- Never sleep. Move time with `srv.Advance(t, d)` or `srv.AdvanceUntil(t, what, cond)`, or run a
  regular job once with its `Tick()` method.
- Take expected packet bytes from a known-good capture, never from the encoder you are testing.
- A pure function with no game flow around it gets a test in `<package>_core_test.go`.

## Formatting and lint

Code is formatted with gofumpt and checked with golangci-lint, using `.golangci.yml`. CI requires
zero lint issues across the whole repository.

Turn on the pre-commit hook once per clone:

```bash
make hooks
```

The hook formats the Go files you commit and lints the packages you changed. To run the checks by
hand:

```bash
golangci-lint fmt
```

```bash
golangci-lint run ./...
```

## Code rules

These rules keep the code consistent. Most of them come from a decision record, which explains why.

- **Behave like the reference.** Packets, their order, formulas, data-file reading and saved data
  must match the aCis Java server. A deliberate difference needs an issue and a note in the docs
  ([decision 0001](decisions/0001-stay-compatible-with-acis.md),
  [agents/porting-playbook.md](agents/porting-playbook.md)).
- **Keep game rules out of the network layer.** A packet handler decodes, calls a game-rule
  function, and turns the result into packets. Game-rule code reports *events* and never builds
  packets ([decision 0005](decisions/0005-domain-events-to-packets.md),
  [`network/AGENTS.md`](../internal/gameserver/network/AGENTS.md)).
- **Always answer the client.** Every rejected client action must send something back (a message,
  a status packet, or `ActionFailed`), so the client is never stuck waiting
  ([agents/action-response-contract.md](agents/action-response-contract.md)).
- **Touch an actor's state only from its own queue.** Never wait on the database from a queue.
  Send writes to the persistence lanes ([decision 0003](decisions/0003-per-actor-queues.md),
  [decision 0004](decisions/0004-ordered-persistence-lanes.md)).
- **Describe the Go code in comments.** Do not cite Java classes or files in production code.
  `internal/provenance` checks this in every test run
  ([decision 0013](decisions/0013-code-describes-itself.md)).
- **Follow the style guide.** [agents/go-style.md](agents/go-style.md) covers naming, types,
  interfaces, errors and lock order.
- **Wire shared services once.** A service that both production and tests need goes in
  `internal/gameserver/app` ([decision 0002](decisions/0002-wire-servers-with-uber-fx.md)).

### Example: adding a client packet

1. Write the decoder in `internal/gameserver/network/clientpackets`, and add a sample packet to the
   game client fuzz target.
2. Register the handler in `client_dispatch.go`. Wrap it in `whenLive` if it acts on the player in
   the world, so it runs on the player's queue.
3. Allow the opcode in the client states where it is valid, in `allowedOpcodes` in `state.go`.
4. Make every rejection send an answer, and add the cases to `silent_action_test.go`.
5. Test the whole flow in a `tests/<area>/` suite.

`TestEveryAcceptedOpcodeHasAHandler` fails if steps 2 and 3 do not match.

### Example: changing the datapack copy

The in-repo `aCis_datapack/` mirrors the full datapack. Do not edit it on its own. Change the full
datapack first, copy the changed files in, then refresh the checksum file and commit both together:

```bash
ops/datapack-manifest.sh write
```

## Continuous integration

`.github/workflows/go.yml` runs on every pull request and on every push to `main`:

| Job | Checks |
| --- | --- |
| `test` | All tests, split into parallel shards. Each behavior suite runs twice: once with the virtual clock, once on the real worker pool. |
| `lint` | `golangci-lint run` over the whole repository |
| `docker` | the datapack copy's checksums, and a smoke test of the Docker stack |
| `go` | passes only when the three jobs above pass. This is the required check. |

Deploys happen only when a GitHub release is published, and only if the `go` check passed on the
release's commit ([decision 0012](decisions/0012-deploy-on-release.md)).

## Profiling and debugging

- Start a server with `-debug-addr 127.0.0.1:6060` to get `pprof` profiles and `expvar` metrics.
  [profiling.md](profiling.md) shows how.
- Build or test with `-tags simdebug` to make `sim.AssertOwner` stricter: it then also catches state
  touched from the wrong queue while that queue is running on another worker.
- [observability.md](observability.md) lists the log files and what each failure looks like.
