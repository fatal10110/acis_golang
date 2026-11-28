# Development guide

How to set up a working copy, build it, run the tests, and get a change through CI. For how the
code is organized, read [architecture.md](architecture.md) first.

## Prerequisites

| Tool | Version | Used for |
| --- | --- | --- |
| Go | the version in `go.mod` (1.26) | building and testing |
| Docker with Compose v2 | any recent | the shared test MariaDB (`make test-db-up`) and the Docker stack |
| golangci-lint | v2 (CI uses v2.14) | formatting (gofumpt) and lint, in the pre-commit hook and in CI |
| MariaDB client | optional | loading the schema by hand when you run the servers outside Docker |

## Workspace layout

The repository works on its own: it carries a copy of the datapack in `aCis_datapack/` (XML, HTML,
SQL, but no geodata), and the test suites use it. Some tasks need more, so the usual layout puts
the checkout in a parent directory next to the full datapack and the aCis Java sources:

```text
acis_public/
├── acis_golang/        this repository
├── aCis_datapack/      full datapack, with geodata; servers run from this one
└── aCis_gameserver/    aCis Java server: reference sources and its config/ directory
```

| You want to | You need |
| --- | --- |
| build, run unit tests, run behavior suites | this repository and Docker |
| run the servers | a full datapack with geodata, and a `config/` directory (the reference one or `ops/docker/config/`) |
| run geodata tests | geodata, found above the module or in `ACIS_GEODATA_DIR` |
| run reference tests, regenerate `scriptfp` goldens | the `aCis_gameserver/` sources |

Do not run servers from the in-repo `aCis_datapack/`. Servers write into `data/` (crests,
announcements), and that copy is checked against `ops/datapack.sha256` in CI.

## Build

```bash
go build ./...
```

```bash
go build ./cmd/loginserver ./cmd/gameserver ./cmd/gsregister ./cmd/accountmgr
```

[tools.md](tools.md) describes every binary. [run-servers.md](run-servers.md) and
[ops/docker.md](ops/docker.md) describe how to run the servers.

## Tests

Tests are behavior-first. Most coverage comes from suites in `tests/<domain>/`. Each one boots a
real game server in the test process (`internal/gameservertest`), drives it with a scripted client
over the real wire protocol (`internal/testsupport`), and checks the packets the client receives,
the world state and the database rows. Only a small set of pure functions is unit-tested directly.
[agents/test-strategy.md](agents/test-strategy.md) has the full policy.

### Start the test database

Most tests need one shared MariaDB on `127.0.0.1:3307`. Each test package creates its own database
on it, so parallel runs and several worktrees can share the instance.

```bash
make test-db-up
```

```bash
make test-db-down
```

`docker-compose.test.yml` uses host networking. On Docker Desktop, enable host networking, or point
the tests at another server with `ACIS_TEST_MARIADB_DSN` (default `root:test@tcp(127.0.0.1:3307)/`).

### Make targets

| Command | What it runs | Database |
| --- | --- | --- |
| `make test-unit` | the pure-function packages (codecs, ciphers, config, formulas) | no |
| `make test-internal` | every package outside `tests/` | yes |
| `make test-one PKG=tests/items [RUN=TestName]` | one behavior suite, optionally one test | yes |
| `make test` | everything (`go test ./...`) | yes |
| `make test-race` | everything with the race detector; this is the full gate | yes |
| `make fuzz [FUZZ=Target] [FUZZTIME=60s]` | the inbound framing, decryption and decoder fuzz targets | no |

Plain `go test` works too. For example:

```bash
go test ./tests/skills -run TestLearnSkillRejectsInsufficientSP -count=1
```

### Test environment variables

| Variable | Effect |
| --- | --- |
| `ACIS_TEST_MARIADB_DSN` | root DSN of the test MariaDB |
| `ACIS_SIM_EXECUTOR` | `inline` (default): actor queues run on one goroutine with a virtual clock. `pool`: the production worker pool and the wall clock. CI runs every suite both ways. |
| `ACIS_REQUIRE_DATAPACK` | `1`: a test that cannot find the datapack fails instead of skipping (CI sets it) |
| `ACIS_REQUIRE_REFERENCE` | `0`: reference-source tests skip when `aCis_gameserver/` is missing (CI sets it); unset, it follows `ACIS_REQUIRE_DATAPACK` |
| `ACIS_DATAPACK` | datapack location for tools and tests that do not find one on their own |
| `ACIS_GEODATA_DIR` | geodata location for the geodata tests |
| `ACIS_PERF_*` | sizes and duration for the `tests/perf` scenarios |

Geodata and reference tests are local-only: CI has neither, so they skip there. If your change
touches geodata code or what the reference tests cover, run them locally and check in `-v` output
that they ran. [agents/verification.md](agents/verification.md#datapack-geodata-and-reference-tests)
lists them.

### Writing tests

- A new feature gets a full-flow test in the matching `tests/<domain>/` package. Use the
  `gameservertest.Boot` options and the `testsupport.ScriptedClient`. Do not write a handshake of
  your own.
- Never wait on the wall clock. Move the virtual clock with `srv.Advance` and `srv.AdvanceUntil`,
  or drive a ticker directly (`srv.InventoryUpdates.Tick()`).
- Expected bytes come from known-good fixtures, not from the encoder under test.
- A pure function with no flow to drive gets a `<pkg>_core_test.go` file.

## Formatting and lint

The code is formatted with gofumpt and linted with the configuration in `.golangci.yml`. CI runs
`golangci-lint run` over the whole tree and requires zero issues.

```bash
make hooks
```

That enables `.githooks/pre-commit`, which formats the staged Go files and lints the packages you
touched. To run the same checks by hand:

```bash
golangci-lint fmt
```

```bash
golangci-lint run ./...
```

## Code conventions

- [agents/go-style.md](agents/go-style.md) is the style and design guide: naming, type shape,
  interfaces, errors, concurrency and lock order.
- Production comments describe this Go code: what it does and the rules it keeps. They do not point
  at the Java implementation or its file names. `internal/provenance` checks this in every test
  run.
- Network code decodes, calls a domain API and maps the result to packets. Game rules belong in the
  domain packages. Every rejected client action must answer the client. The rules are in
  [`internal/gameserver/network/AGENTS.md`](../internal/gameserver/network/AGENTS.md),
  [agents/action-response-contract.md](agents/action-response-contract.md) and
  [agents/server-initiated-updates.md](agents/server-initiated-updates.md).
- Actor state is touched only from its own `sim.Queue`. Database writes go through
  `persist.Worker`. See [architecture.md](architecture.md#concurrency-model).
- A new provider shared by production and tests goes in `internal/gameserver/app`, so both boot
  graphs pick it up.
- Behavior must match the aCis Java server: packets, their order, formulas, data-file reading and
  persistence. [agents/porting-playbook.md](agents/porting-playbook.md) describes how to check a
  change against the reference.

### Adding a client packet

1. Add the decoder in `internal/gameserver/network/clientpackets` and a seed for the game client
   fuzz target.
2. Register the handler in `client_dispatch.go` (`register`, wrapped in `whenLive` when it acts on
   the player in the world).
3. Allow the opcode for its client states in `allowedOpcodes` in `state.go`.
4. Answer every rejection branch, and extend `silent_action_test.go`.
5. Cover the flow in a `tests/<domain>/` suite.

`TestEveryAcceptedOpcodeHasAHandler` fails when steps 2 and 3 do not agree.

### Changing the datapack copy

The in-repo `aCis_datapack/` mirrors the full datapack and is not edited on its own. Change the
full checkout first, copy the changed paths in, then refresh the manifest and commit both:

```bash
ops/datapack-manifest.sh write
```

## Continuous integration

`.github/workflows/go.yml` runs on every pull request and on pushes to `main`:

| Job | What it checks |
| --- | --- |
| `test` | the test suites, split into shards. Each behavior suite runs once on the inline executor and once on the real pool, against a MariaDB service, with `ACIS_REQUIRE_DATAPACK=1` |
| `lint` | `golangci-lint run` over the whole tree |
| `docker` | `ops/datapack-manifest.sh check` and the Docker smoke test (`ops/docker/smoke-test.sh`) |
| `go` | passes only when the three jobs above pass; this is the required check |

`.github/workflows/deploy.yml` deploys when a GitHub release is published. The release tag selects
the commit, and that commit must have a green CI run.

## Profiling and debugging

- Start a server with `-debug-addr 127.0.0.1:6060` for `pprof` and `expvar`. See
  [profiling.md](profiling.md).
- Build with `-tags simdebug` to make `sim.AssertOwner` also catch state touched from the wrong
  queue while that queue is busy on another worker.
- [observability.md](observability.md) describes the log files and what each failure looks like.
