# Test strategy: fakes vs. integration/behaviour tests

This repo has hundreds of hand-written `fake*`/`recording*`/`spy*`/`stub*` test doubles, one per
file, no shared mocks package, and 19 `//go:build integration` files that already exercise real
boundaries (a disposable MariaDB via `internal/gameserver/data/sql/sqltest`, real TCP/crypto
handshakes in `internal/link` and `internal/loginserver`). This doc is the decision rule for when a
fake should be replaced by the real thing it stands in for, so that judgment call doesn't get
re-litigated per package.

## Decision rule

Replace a fake with the real type only when **both** hold:

1. A real, already-existing production type in this repo satisfies the same method set the fake
   stands in for. Verify this with `gopls` or a direct `rg` for the interface's methods on the real
   type *before* starting the conversion — do not assume. See the counter-example below; the
   assumption has been wrong before.
2. Building the real type for the test is not disproportionate to the behavior under test. A
   DB-store boundary reachable via `sqltest.NewDB` always qualifies. A heavy actor requiring full
   `world.State`/geodata wiring to construct may not, unless the test is specifically about that
   wiring.

Keep a double as-is (do not convert) when:

- It's a pure-algorithm/no-boundary test: `skill/formulas`, `model/geometry`, `geo/pathfind`,
  `internal/commons` (fields/gameduration/statset), packet byte-level encode/decode tests. These
  have no boundary to integrate against and are out of scope for this initiative entirely.
- It satisfies a package-local interface that no real type implements yet. See
  `internal/gameserver/skill/conditions/conditions_test.go`: its `Actor`/`PlayerActor` interfaces
  stand in for a not-yet-built creature/clan runtime — `player.Character` implements none of
  `Actor`'s methods (`Level`, `HPRatio`, `IsMoving`, ...) and none of `PlayerActor`'s clan methods
  (`HasClan`, `ClanCastleID`, `ClanHallID`, `IsClanLeader`, `PledgeClass`) exist anywhere in the
  codebase — there is no `clan` package yet. Converting this now would mean adding throwaway stub
  methods to production types just to satisfy a test, which is scope creep into unbuilt game
  features disguised as test-quality work. Leave it, and add a one-line comment pointing back here
  so the next reader doesn't "fix" it incorrectly. Revisit once the real implementation exists.
- It's a narrow, sanctioned infra seam (a clock, timer, or RNG source) even when a real wall-clock
  equivalent exists, because determinism is the point of the test, not integration coverage. Example:
  `fakeCastClock`/`fakeCastTimer` in `internal/gameserver/model/actor/cast`.

## Worked conversion example (the risk this initiative targets)

`internal/gameserver/skill/effect/hooks_buff_test.go:490` — `fakeChargesTarget.IncreaseCharges`
**reimplements** the same cap/overflow logic that already exists on the real
`(*player.Character).IncreaseCharges` (`internal/gameserver/model/actor/player/character_charges.go:21`).
The fake can silently drift from the real method while its own test keeps passing, since nothing
ties the two implementations together. `hooks_status_test.go:297`'s `fakeCombatant`/`betrayOwner`
similarly stand in for `attackable.Combatant` (`internal/gameserver/model/actor/attackable/combatant.go:8`
— `SiegeGuard() bool`, `AlikeDead() bool`, plus `worldobject.Object`'s `ObjectID()`), which
`*player.Character` already implements. Both convert cheaply to `&player.Character{ID: N}` — no
DB/world wiring required, since these tests only read zero-value-safe accessors. This conversion
stays a plain unit test file; no `//go:build integration` tag, because no DB/socket boundary is
involved (see the tag convention below — "use the real struct" and "hit a real DB/socket" are
different conversions).

## Naming / build-tag convention

- File suffix `_integration_test.go`, package-level `//go:build integration` as the first line,
  blank line after — matches every existing file in the 19-file set.
- A plain unit-test file should not duplicate a scenario an integration-tagged file in the same
  package already covers without a documented reason. `internal/gameserver/data/manager/roster_test.go`
  (real DB) and `roster_create_test.go` (hand-rolled fake stores, same `Roster.Create` behavior) is
  the negative example this initiative fixes.
- Swapping a fake for a real production struct that needs no DB/socket boundary does **not** require
  the `integration` tag or file suffix — it stays a plain, fast unit test. The tag is for a real
  external boundary (DB, socket), not for "uses a real type."

## Real actor fixture pattern

No shared builder/factory exists for constructing a real `player.Character` in tests today.
`internal/gameserver/model/actor/player`'s own 40-file test suite constructs `&Character{ID: N}`
directly and relies on the type being zero-value-friendly, adding
`creature.NewLive(loc, heading, geoStub, ch)` + `ch.Live = live` only when a test needs
`EffectList()`, movement, or other live-actor behavior (see the `persistenceTestGeo` stub pattern in
`internal/gameserver/skill/persistence_test.go`). Document and reuse this pattern rather than
inventing a new builder package now — a factory would be a speculative abstraction ahead of need.
Apply the Rule of Three: only extract a shared helper once three or more packages need the identical
multi-field `Character` setup verbatim.

## DB integration tests

Use `sqltest.NewDB(t) *sql.DB` (`internal/gameserver/data/sql/sqltest/sqltest.go`) to get a real,
schema-provisioned MariaDB instance, then construct the real store type
(`sql.NewSkillSaveStore(db)`, `sql.NewCharacterStore(db)`, etc.) and drive the test through its real
read/write path — assert on what the store reads back, not on an in-memory fake's captured state.

`sqltest.NewDB` boots one container per call, and today's integration files mostly call it once per
test function. If CI wall-clock time becomes a problem as more files convert, a package-level shared
container (e.g. `sqltest.SharedDB(tb testing.TB)`, booted once in `TestMain`, with per-test cleanup
via truncation or a rolled-back transaction) is the fix — build that only once there's evidence it's
needed, not preemptively.

## Checklist before converting a fake

1. Identify every method the fake implements.
2. Confirm with `gopls` or `rg` that a real, already-existing type in this repo implements every one
   of those methods (or the real DB/socket boundary the fake stands in for already has a working,
   integration-tested implementation).
3. If no real type/implementation exists, stop — leave the fake in place and add a comment linking
   back to this doc's "keep as-is" list. Do not add stub methods to a production type just to satisfy
   a test.
4. If a real type/implementation exists, replace the fake, delete it once unused, and confirm the
   converted test asserts the same-or-superset of what the fake-based version proved — not less.
