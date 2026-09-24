# Verification policy

Verification has two levels: fast feedback while editing and complete gates before claiming an
implementation change is done.

## Focused checks during implementation

Choose the smallest command that exercises the changed behavior:

```bash
rtk go -C acis_golang test ./internal/gameserver/<affected-package>
rtk go -C acis_golang test ./internal/gameserver/<affected-package> -run '<FocusedTest>'
rtk go -C acis_golang test -race ./internal/gameserver/<affected-concurrent-package>
```

Use the equivalent `internal/loginserver/...`, `internal/commons/...`, or `cmd/...` package when that
is the actual scope. Do not repeatedly run the full repository suite after each small edit.

Contract-specific checks supplement unit tests:

- packets: opcode, byte layout, state gate, send order, rejection response, and `packetdiff` or
  committed byte fixtures;
- formulas: independent oracle vectors, including boundary, rounding, and overflow cases;
- loaders: same-file load counts and representative field dumps;
- geodata: `geoprobe` comparisons for movement, line of sight, and paths;
- persistence: integration tests against the expected schema and transaction effects;
- concurrency: focused `-race` coverage and lifecycle cancellation.

## Server-initiated update checks

Rules for this class of change live in
[`server-initiated-updates.md`](server-initiated-updates.md); these are its checks. Unit tests assert
the changed value, not its delivery, so check delivery separately whenever a change touches rewards,
tasks, effect actions, AI, or any other server-driven path.

Confirm the new state has a live consumer:

```bash
go run golang.org/x/tools/cmd/deadcode@latest ./cmd/gameserver | rg 'gameserver/task/'
rg -n --multiline 'func \([a-zA-Z ]*[Ee]ffects\) [A-Z][A-Za-z]*\([^)]*\) *\{\}' cmd/gameserver
```

The first command lists ticking subsystems no production path constructs; the second lists
composition-root adapters whose methods are empty stubs. Both mean the subsystem runs, or appears to
run, while doing nothing. `deadcode` reports a method of an `fx`-provided type as reachable through
reflection even when nothing calls it, so for those types also confirm a real caller with `rg` before
concluding the subsystem is wired.

Then confirm the change is delivered: trace the mutating call to a `SendFrame`, a broadcast, or a
queue that a running task drains, and cover it with a domain test that counts runtime-hook calls.

## Complete implementation gates

Before claiming completion of a source-code change, require no output from the formatting check and
successful completion of every repository gate:

```bash
find acis_golang -name '*.go' -type f -exec gofmt -l {} +
rtk go -C acis_golang vet ./...
rtk go -C acis_golang build ./...
rtk go -C acis_golang test -race ./...
```

There is no separate integration tier: `go test ./...` is the only run and needs Docker (the
persistence suites boot MariaDB via testcontainers). The integration build tag was removed
tree-wide in #1682; never reintroduce it.

### Colima: the default test run needs an explicit Docker host and Ryuk disabled

The persistence suites spin up mariadb via testcontainers-go. On a machine running Colima instead of
Docker Desktop, the default socket discovery fails with `rootless Docker not found`, and even after
pointing at the Colima socket, testcontainers' Ryuk reaper sidecar fails to start (`error while
creating mount source path .../docker.sock: operation not supported`) because it bind-mounts the
socket the way Docker Desktop's layout expects, which Colima's does not match. Set both before
running the suite:

```bash
export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock
export TESTCONTAINERS_RYUK_DISABLED=true
rtk go -C acis_golang test -race -count=1 ./...
```

Confirm the actual socket path with `docker context ls` first — `default/docker.sock` assumes the
default Colima profile. Disabling Ryuk only skips its container-cleanup sidecar; testcontainers still
tears down each container it starts.

### `sqltest.SharedDB`: pooled databases, one per running test

`sqltest.NewDB(t)` creates a fresh database and applies the schema for that single test. For a
package with many persistence tests, repeating the schema dominates wall-clock time.
`sqltest.SharedDB(tb)` instead checks a database out of a per-test-binary pool (Go compiles each
package's tests into its own binary, so the pool is per package). The test holds it until its
`tb.Cleanup`, which truncates the tables and returns it. Sequential tests therefore reuse one
database, and each test running at the same time under `t.Parallel()` gets its own. Repeated
calls with the same `tb` return the same database.

- **Subtests:** a `t.Run` subtest is a different `tb`, so it gets a different database from its
  parent. Seed rows and call `gameservertest.Boot` on the same `t`.
- **Gameplay settings:** each `Boot` carries its own (`WithCancelLesserEffect`,
  `WithMagicFailures`, ...), so a test that overrides one may still call `t.Parallel()`. A new
  setting reaches its consumer through a constructor or option, never a package global.

The pooled databases outlive individual tests, so a package using `SharedDB` must add a
`TestMain` that drops them once, after every test in the package has run:

```go
func TestMain(m *testing.M) { os.Exit(sqltest.Main(m)) }
```

Without it, the package's databases leak on the shared MariaDB instance and pile up across local
runs. Add it to any new package that adopts `SharedDB`.

Use `NewDB` instead of `SharedDB` for a test that mutates the schema itself (e.g. dropping a table to
force a downstream failure), because the database goes back to the pool for the package's next test.

### Datapack oracle tests are local-only — CI cannot run them

`aCis_datapack` is a separate checkout that is never pushed, and `.github/workflows/go.yml` checks
out only `acis_golang`. Every test that reads the shared datapack resolves its path through a helper
that calls `t.Skip` when the datapack is absent, so **all datapack oracle tests silently skip in CI
and a green CI run proves nothing about them.**

This makes the local run the only gate for them. Before every push:

- run the full suite from the outer `acis_public/` root, with `aCis_datapack/` present, so the
  oracle tests actually execute;
- confirm the run did not skip them — a skip reading `aCis_datapack not checked out near the module
  root` means the gate did not run and completion cannot be claimed;
- never treat a green CI check as covering a datapack-backed acceptance criterion.

```bash
# from acis_public/, with aCis_datapack/ checked out alongside acis_golang/
rtk go -C acis_golang test -race ./...
rtk go -C acis_golang test -run 'Datapack|Oracle|Shipped' -v ./... | rg -i 'SKIP|FAIL'
```

An issue whose acceptance criteria name real datapack content is not verified until its oracle test
is observed passing locally.

The installed `rtk` version exposes compact `go test`, `go build`, and `go vet` wrappers.
`golangci-lint` is not currently installed or configured as a repository gate. Run
`rtk err golangci-lint run` only when the task or CI requires it and the binary is available;
otherwise report it as unavailable, never as passed.

If an external integration service is unavailable, run all independent gates and report the exact
blocked test; do not convert the missing service into a passing result.

## Documentation and agent configuration

When only Markdown, TOML, JSON, YAML, or agent configuration changes, the full game-server suite is
not required unless another repository policy says otherwise. Instead:

- parse every changed TOML, JSON, and YAML file;
- validate installed-tool configuration with the tool's strict/local parser where available;
- check Markdown links and required headings;
- search for stale paths and contradictory duplicate rules;
- verify no source or shared data file changed;
- compare instruction line, word, and byte counts before and after.

## Completion evidence

Report the commands actually run and their exit status. Do not claim an unrun gate passed. Preserve
the exact-contract fixtures, packet-impact check, silent-rejection check, concurrency ownership, and
documented deferrals in the final review. For each shipped gap or out-of-scope item, report the live
follow-up issue number, confirm its comments were checked, and confirm that its body and milestone
cover the deferred work. For issue implementation, report the draft pull-request URL and confirm it
targets `main`, contains the issue-closing keyword, and includes only the scoped commit; omit this
only when the user explicitly requested local-only work. Before staging or opening the pull request,
audit the final diff for gaps and require `Gap audit: no shipped gaps` or one verified open issue,
checked comments, and milestone for every shipped gap.
