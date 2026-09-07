# Infrastructure review (2026-09-05)

Review of `acis_golang` composition root, networking, scheduling, persistence, config, logging,
world/task registries, test harness, and CI/ops — done to check the base is strong before more
gameplay work builds on it. Reviewed at `main` @ `5665794e`.

Two structural refactors already have their own design docs and are correct in direction; this
review does not redo them, only gives sequencing advice:

- [`concurrency-refactor-plan.md`](concurrency-refactor-plan.md) — 116 mutexes → per-actor owner queues.
- [`actor-model-degoifying-plan.md`](actor-model-degoifying-plan.md) — typed registries, event sink
  replacing ~67 hook setters, drop 80 anonymous interface asserts.

## Verdict

Base is solid: fx lifecycle with ordered stop hooks and per-hook budgets, single-writer `Conn` with
vectored batching and explicit frame ownership, `Session` encrypt-order lock, panic-isolated
tickers, draining accept loop, boot-time DB repair, per-test databases on one shared MariaDB,
behavior suites through the production boot path, race-enabled CI, strong agent rules. Nothing here
blocks building on it.

What is missing is mostly small and stdlib-sized: one real persistence race, one unbounded periodic
DB call, zero runtime observability, no format gate, and a composition root that has grown 23
one-value providers.

## Findings

### P1 — correctness

1. **Lost item update during flush.** `internal/gameserver/task/iteminstances.go:94-107`. `Save`
   snapshots pending → flushes (DB I/O) → deletes the snapshotted ids. `addToBatch` calls
   `inst.Snapshot()` at batch build, so an item mutated during the flush is re-`Add`ed (container
   persister hook, `network/character_flow.go:732`, `network/pet.go:57`) and then deleted by the
   same `Save`; its newest state is not persisted until the next mutation. Window = flush duration.
   Fix: swap the map at snapshot time (`inflight := i.pending; i.pending = make(...)`), flush
   `inflight`; on error merge it back with `if _, ok := i.pending[k]; !ok { i.pending[k] = v }`
   (both sides hold the same `*item.Instance`, so which wins is moot). Known interplay:
   `RemoveItems` (only caller `network/lifecycle.go:234`, after a successful logout flush) cannot
   reach `inflight`, so on a flush error the merge re-pends items the logout path already saved —
   one redundant write next tick, harmless; leave a comment so nobody "fixes" it. Add one test
   asserting an `Add` during `Flush` survives. Land with finding 2 as one commit: the deadline error
   from 2 must take this merge-back path, not the old delete path.

2. **Periodic item flush has no deadline.** `iteminstances.go:52` uses `context.Background()`.
   Autosave (`network/taskeffects.go:214`) and seven signs (`sevensigns.go:197`) already use
   timeouts. A hung DB wedges this ticker; `StopAndWait` at shutdown then blocks until fx's 30s
   kills the process, and the final `Save` hook never gets its 10s budget. Fix: wrap the periodic
   call in `context.WithTimeout(…, itemInstanceShutdownSaveTimeout)` or a new const.

3. **Boot I/O runs in fx constructors with no deadline.** `cmd/gameserver/infra.go:45`
   (`idfactory.New`, full-table scans + ~50 repair statements), `tasks.go:64-77` (`store.Load/Clear`),
   `world.go:51` (`LoadSpawns`) all pass `context.Background()`. These run inside `fx.New`, before
   `Run` installs signal handling and outside any start timeout. Fix: create one `bootCtx` with a
   timeout in `main`, `fx.Supply` it, thread it to those three providers.

### P2 — tooling and hygiene

4. **No gofmt gate.** 10 files currently unformatted (`gofmt -l internal cmd`). CI runs
   build/vet/test only. Fix: add a `gofmt -l` step to `.github/workflows/go.yml` that fails on
   output, and format the 10 files.

5. **11 MB untracked binary in tree.** `ops/admin-panel/admin-panel` (shows as `?? ops/admin-panel/`
   in `git status`). One accidental `git add .` away from permanent repo bloat. Fix: `.gitignore`
   entry.

6. **Accept loop spins on transient errors.** `internal/commons/netutil/acceptloop.go:62` uses the
   deprecated `net.Error.Temporary()` and retries with no backoff; on EMFILE this is a hot loop.
   Fix: `errors.Is(err, net.ErrClosed)` → return, otherwise sleep with the net/http-style 5 ms→1 s
   backoff.

7. **Zero runtime introspection.** No pprof, expvar, health, or tick-duration reporting anywhere.
   Production auto-deploys on every green `main` push, so the only diagnostic today is the log
   file. Fix, stdlib only: optional `-debug-addr 127.0.0.1:6060` flag on both binaries serving
   `net/http/pprof` + `expvar`; publish players online, connections, and per-ticker last/max tick
   duration. `scheduler.Ticker` has no name and no timing today (`scheduler/ticker.go:44`, `tick`
   just calls `fn`) and `Start` has 21 call sites. Cheapest: derive the key inside `Start` with
   `runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()` trimmed of the module prefix (gives
   `task.(*Door).Tick-fm`, `task.(*Walker).Start.func1`), keep one `expvar.NewMap("tickers")` in
   `scheduler`, wrap `fn` with `time.Since` in `tick`. Zero call-site change. Per-summon tickers
   (`summon/live_lifecycle.go:99,155`) share one key each; acceptable. Upgrade path: explicit
   `name` parameter on `Start` if derived names prove unreadable.

### P3 — composition root, config, DB

8. **Config sprawl.** `cmd/gameserver/main.go:56-79` has 23 one-value providers with named scalar
   types; `provideGameClientLink` (`network.go:50-99`) takes 48 parameters.
   `network.PlayerConfig` already exists as the target shape. Fix: one `loadGameplayConfig` provider
   returning a struct (reuse the existing `loadX` functions as its body), drop the named scalar
   types.

9. **Five mutable package globals set from the root.** `formulas.SetMagicFailures`,
   `effect.SetCancelLesser`, `effect.SetGameClock`, `creature.SetNightSource`,
   `npc.SetMaxGeoPathFailCount`. Hidden coupling, test-order sensitivity. Defer: the actor-model
   plan Phase B rewires domain hooks anyway; fold these in then rather than touching domain
   packages twice.

10. **Two packages log through the global zerolog logger.** `config/properties.go:198,370-388`
    (missing key, malformed pair, non-numeric value) and `geo/reader/l2off.go:59` (trailing-bytes
    warning in `decodeL2OFF`) are the only `zerolog/log` importers. Their warnings bypass the
    configured `logging.Runtime` (file sinks, levels). Fix for config: follow the existing
    `UnsupportedKeys` pattern (`logging.Config`, `task.PvPFlagOptions`): collect warnings on
    `Properties`, let the root log them with the real logger. Fix for l2off: `ReadL2OFF(path)` has
    no logger; return the trailing-byte count alongside the region (or as a typed warning the
    geodata loader logs) and drop the import.

11. **DB pool: hard-coded 8 conns, no `SetConnMaxLifetime`.** `commons/db/pool.go:45-47`. 21
    tickers plus per-connection synchronous saves share 8 conns with no config knob. Hygiene, not a
    live bug: `pool.go:47` already sets `SetConnMaxIdleTime(10m)`, MariaDB's default `wait_timeout`
    is 8h so idle conns close client-side long before the server drops them, and `database/sql`
    retries `driver.ErrBadConn` on a fresh conn. `SetConnMaxLifetime` still bounds connection age
    across server-side restarts and failover, where the idle timer does not help. Fix:
    `SetConnMaxLifetime(5*time.Minute)` and an optional `MaxConnections` config key with default 8;
    the config key is the real deliverable.

12. **Hand-copied test schema, no drift guard.** `data/sql/sqltest/sqltest.go` carries 12
    `CREATE TABLE` copies (310 lines) of `../aCis_datapack/sql/*.sql` (65 files). `characters`
    verified identical today; nothing keeps it so. CI has no datapack checkout, so loading the real
    files in CI is out. Fix: one test that, when `../aCis_datapack/sql` exists (skip otherwise),
    normalizes each copy against its datapack file and fails on mismatch.

13. **idfactory memory ceiling.** `commons/idfactory/idfactory.go:111` — `map[int32]struct{}` over
    every persisted object id (~40–50 B/entry; 5 M items ≈ 250 MB). Not a problem at current scale.
    Note only: upgrade path is a `[]uint64` bitset over `id-First` (1 bit/id) with a word-scan
    `nextFreeFrom`. Defer until item counts matter.

### P4 — test infrastructure (note, defer)

14. Policy says only pure-unit tests live outside `tests/`; reality: 186 test files in
    `internal`/`cmd`, 27 still named `*_integration_test.go`, 26 hit the DB.
    `docs/agents/test-strategy.md` acknowledges the legacy set (#1678–#1682). Not a bug; finish the
    deletion series or drop the `_integration` suffix so the naming stops contradicting AGENTS.md.

15. 55 `time.Sleep` in tests, 65 direct `time.Now()` in non-test `internal`. Owned by concurrency
    plan Phase 6 (`sim.Clock`). Defer.

### Ops (note)

16. `deploy.yml` restarts both services on every green `main` push with no drain or announce; the
    30s stop budget saves state but every merge kicks players. Consider tag- or
    `workflow_dispatch`-gated deploys once there are real players. Not code.

## Sequencing advice for the two refactor docs

- Run actor-model Phase A → B before concurrency Phase 2+. Both docs already say this; B collapses
  67 hook fields into one immutable sink, which removes most of the lock sites Phase 5 would
  otherwise have to migrate.
- For concurrency Phase 3, treat the documented fallback (one mutex per actor guarding
  HP/MP/CP/dead/hate, synchronous cross-actor calls) as the target, not the spike's plan B. Async
  cross-actor mutation turns "did the target die" into a reply message, and the behavior suites are
  the only oracle for two-actor packet ordering. One lock per actor instead of ~19 already delivers
  the win with far less semantic risk.
- Finding 9 (package globals) should ride inside Phase B.
- Batch 3 finding 8 rewrites `cmd/gameserver/{main,network}.go`; actor-model Phase A/B also edit
  `cmd/gameserver/{tasks,network}.go` (anonymous-assert cleanup, sink factory wiring). Serialize:
  land Batch 3 before Phase A opens, or rebase it after B. Not concurrently.

## Batches (tracked as GitHub issues)

- **Batch 1 — persistence correctness** (findings 1–3).
- **Batch 2 — hygiene + observability** (findings 4–7, 11).
- **Batch 3 — config consolidation** (findings 8, 10, 12).

Deferred (no issue yet): 9 (folds into actor-model Phase B), 13, 14, 15, 16.

## Verification

- Every batch: `gofmt -l internal cmd` empty, `go -C acis_golang build ./...`,
  `go -C acis_golang vet ./...`, `make -C acis_golang test-race` with `acis-test-mariadb` up.
- Batch 1: new test fails before the map swap, passes after; `tests/lifecycle` and `tests/items`
  green under `-race`.
- Batch 2: boot gameserver per `docs/run-servers.md` with `-debug-addr 127.0.0.1:6060`;
  `curl localhost:6060/debug/vars` shows ticker durations; `curl localhost:6060/debug/pprof/goroutine?debug=1`
  lists 21 ticker goroutines; CI gofmt step red on an unformatted file, green after.
- Batch 3: behavior suites unchanged (no packet-byte change); provider count in `main.go` drops;
  drift test passes locally against `../aCis_datapack/sql`.
