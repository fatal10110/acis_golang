# Test strategy: behaviour-first, fakes vs. real boundaries

Tests in this repo are **behavior-first**: a feature's primary coverage is a full-flow scenario
driven through the real wire protocol against a real MariaDB, not per-function unit tests. This
doc records the tier structure, the shared harness, and the remaining decision rule for when a
fake may stand in for a real type.

Current state (post #1682): the scripted client lives
in `internal/testsupport`, the server boot harness in `internal/gameservertest`, and the MariaDB
container helpers in `internal/gameserver/data/sql/sqltest`. The legacy per-package fixtures and
the integration build tag is gone: #1677 and its children deleted flow-covered unit tests,
and #1682 consolidated the pure-function survivors into `<pkg>_core_test.go` files.

## Test tiers

### Tier 1 — behavior suites (`tests/<domain>/`) — the tier that matters

One package per player-facing domain (`character`, `items`, `skills`, `combat`, `trade`, `pets`,
`lifecycle`). Each file drives one full player-facing flow end to end through
`gameservertest.Boot` and asserts on three surfaces:

1. client-visible packets (opcode sequence + key payload fields),
2. world state (live actors, positions, flags),
3. persisted DB rows (`characters`, `items`, `character_skills`, ...).

New features land here. If a behavior can't be triggered through a client packet or an explicit
domain entry point, that is a product gap to raise — not a reason to write a white-box unit test.

Harness usage:

```go
func TestPlayerPicksUpDroppedItem(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	// ... read SSQInfo / CharSelected, send EnterWorld, consume the burst ...

	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, item.AdenaID, 1000)
	srv.InventoryUpdates.Tick() // drive batching deterministically
}
```

Rules:

- Setup goes through `testsupport.ScriptedClient` and `gameservertest` fixtures only — no
  per-suite reimplementations of the handshake, no struct-literal surgery on production types.
- Use `SyncBarrier` when a triggering request has no synchronous reply and you need ordering
  before driving a task tick.
- Suites need Docker (MariaDB testcontainer); each suite package calls `sqltest.Main(m)` from
  `TestMain`.

### Tier 2 — pure-function core tests (`<pkg>_core_test.go`)

The only unit tests allowed outside `tests/`: packet encode/decode round-trips, damage/stat
formulas (`skill/formulas`, `stat*`), config/property parsing, crypt primitives, container slot
arithmetic. Table-driven, zero shared fixtures, no build tag. A new file outside `tests/` must fit
one of these categories.

### Legacy in-package tests

Existing per-package tests remain until their domain's behavior suite covers them, then get
deleted wholesale (#1678–#1682). Do not add new members to them; extend the behavior suite instead.

## Fakes vs. real types

Replace a fake with the real type only when **both** hold:

1. A real, already-existing production type satisfies the same method set the fake stands in for.
   Verify with `gopls` or a direct `rg` *before* converting — do not assume.
2. Building the real type is not disproportionate to the behavior under test. A DB-store boundary
   reachable via `sqltest` always qualifies. A heavy actor requiring full `world.State`/geodata
   wiring may not, unless the test is about that wiring — but prefer moving such coverage into a
   behavior suite over keeping a bespoke fake.

Keep a double as-is when:

- It serves a tier-2 pure-algorithm test (`skill/formulas`, packet byte fixtures,
  `internal/commons`) — there is no boundary to integrate against.
- It satisfies a package-local interface whose real implementation doesn't exist yet (e.g.
  `skill/conditions`' clan-runtime interfaces). Leave a comment pointing here so nobody "fixes" it
  by adding stub methods to production types. Revisit when the real implementation lands.
- It's a narrow sanctioned infra seam (clock, timer, RNG source) where determinism is the point:
  e.g. `fakeCastClock`/`fakeCastTimer` in `model/actor/cast`.
- It's part of the shared harness itself: `testsupport.FrameCapture` and the world-presence doubles
  used by direct frame-sender assertions are sanctioned; they observe rather than reimplement.

Worked example of the risk: `fakeChargesTarget.IncreaseCharges`
(`skill/effect/hooks_buff_test.go`) once reimplemented the cap/overflow logic that already existed
on `(*player.Character).IncreaseCharges` and silently drifted. When a fake duplicates real logic,
delete the fake and call the real type.

## Naming and tagging

- New behavior suites: `tests/<domain>/<flow>_test.go`, no build tag.
- New pure-core tests: `<pkg>_core_test.go`, no build tag.
- No file carries the integration build tag (removed tree-wide in #1682). Default `go test ./...`
  runs everything; Docker is a hard dependency of the default tier.
- A plain core test must not duplicate a scenario a behavior suite already covers without a
  documented reason.

## Actor construction in core tests

Tier-2 tests construct `&player.Character{ID: N}` directly and rely on zero-value safety, adding
`creature.NewLive(loc, heading, geoStub, ch)` + `ch.Live = live` only when live-actor behavior is
exercised (see `persistenceTestGeo` in `internal/gameserver/skill/persistence_test.go`). Behavior
suites never hand-build actors — they enter the world through packets and use `gameservertest`
handles. Apply the Rule of Three before extracting any new shared builder.

## DB-backed tests

Two entry points, both untagged:

- `sqltest.SharedDB(tb)` — one MariaDB container per test binary, tables truncated between tests;
  pair with `TestMain(m) { os.Exit(sqltest.Main(m)) }`. This is what behavior suites and
  `gameservertest.Boot` use.
- `sqltest.NewDB(t)` — a dedicated container per call; reserved for store-level tests that want
  full isolation.

Construct the real store (`sql.NewCharacterStore(db)`, ...) and assert on what it reads back, not
on captured in-memory state.

## Checklist before writing any test

1. Is this a player-facing flow? → tier 1: extend `tests/<domain>/` via `gameservertest`.
2. Is this a pure function (codec/formula/config/crypt)? → tier 2 `<pkg>_core_test.go`.
3. Neither, and no behavior suite covers the domain yet? → write the behavior suite first
   (or extend an adjacent one), then decide whether anything remains worth a core test.
4. Converting an existing fake? Follow "Fakes vs. real types" above; confirm the converted test
   asserts the same-or-superset of what the fake-based version proved.
