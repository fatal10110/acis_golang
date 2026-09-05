# De-Java the acis_golang actor model

Design doc for a three-phase refactor of the actor model. Self-contained: an implementer with no
access to the conversation that produced it must be able to execute it from this file plus the
repository. All paths are relative to `acis_golang/` unless prefixed with `../`. Shell commands
follow the working-directory contract in `AGENTS.md` (run from the outer `acis_public/` root with
`-C acis_golang`, or use `rtk go -C acis_golang`).

## Context

`acis_golang` is a Go rewrite of a Java game server (aCis). The port kept Java's *structure*, not
just its behavior, in three places. Each one is an idiom Go replaces with something simpler:

| # | Java shape carried over | Where | Size (measured on `main` @ `0d32d37a`) |
|---|---|---|---|
| A | `WorldObject` universal base + downcast after every lookup | `internal/gameserver/model/worldobject` (1 interface, 1 method), `internal/gameserver/world/{state,registry}.go` | registries store `worldobject.Object`; ~20 `obj.(*T)`/`obj.(world.Tracked)` casts after `State.Object/Player/Summon`; **80** `x.(interface{ Foo() })` anonymous asserts; marker interface `world.Player{ WorldPlayer() }` |
| B | Hand-rolled vtable: abstract "send/broadcast/notify" methods overridden by injecting `func` fields | `model/actor/player/character.go:124-181` (63 func fields), 67 `Set*` setters, all wired from `network/character_flow.go:430-830`; `summon.Actor` 11; `npc.Hostile` 3 + `FrameBuilder` (12 methods); `cast.Controller` 3; `attack/move.Controller` 3 | 53 `if hook != nil { hook() }` sites in `player` alone; every setter takes `stateMu` |
| C | `instanceof` disguised as optional one-method interfaces + runtime assertion | `skill/effect/interfaces.go` (90 unexported ifaces, 92 distinct methods, 117 asserts in pkg), `handler/target/*.go` (~20 ifaces), `handler/skill/*.go` (~15), `creature/formula.go`, `attack/controller.go` | only three live actor kinds ever implement them: `*player.Character`, `*npc.Hostile`, `*summon.Actor` |
| C′ | Same smell at the root: the domain's "creature handle" types are too narrow, so every consumer widens them inline | `attackable.Combatant` (3 methods, **198** uses), `creature.DeathActor` (1 method), `summon.Owner`; consumers in `npc` (17), `summon` (11), `player` (11), `creature` (5), `cast` (4), `ai` (4), `attack` (3), `move` (2) | **80** `x.(interface{ Foo() })` anonymous asserts repo-wide, 55 of them in actor packages; the asserted method union is 30 names, `Position()` alone 13× (e.g. `ai/attackable.go:835 combatantPosition`) — plus 52 named unexported one-method optional interfaces in `model/actor/*` |

Also noted, **explicitly out of scope** (user decision): naming churn (`npc.Instance`/`InstanceKind`,
`staticobject.Object`, `door.Object`, `data/manager`), the embedding chain
`world.Presence → *creature.Live → player.Character → network.livePlayer` (accepted composition), and
mutex removal (owned by `docs/concurrency-refactor-plan.md`; this work reduces lock sites as a side
effect and is a prerequisite-friendly step for it, see "Interplay" at the end).

User decisions already made:
- Do A, B, C. Order A → B → C.
- B replacement shape: **domain describes what happened as typed events; network maps events to
  packets.** Not "one fat Client interface per actor".
- Amend `docs/agents/server-initiated-updates.md` and `docs/agents/go-style.md` (they currently
  *mandate* the `Set<Thing>Updater` pattern being removed).
- Delivery model is the implementer's/user's call; this file is the deliverable. Each phase must
  independently keep `go test -race ./...` green and can be its own branch/PR.

Non-negotiable invariants (from `AGENTS.md`, apply to every phase):
- Observable behavior is frozen: wire bytes, packet order per recipient, formulas, persistence,
  rejection paths. The behavior suites under `tests/` are the oracle. When a hook closure body
  moves, its send sequence moves verbatim.
- Domain packages (`internal/gameserver/model/...`, `skill/...`, `handler/...`) never import
  `network/serverpackets`. After B they also never import `commons/wire`.
- Production identifiers and comments describe this Go system only; no Java class/file citations.
- Keep Lineage domain terms (Npc, Playable, Servitor, Cubic, Henna …) verbatim.
- No new one-implementation interfaces, no speculative abstraction.

---

## Phase A — typed world registries, delete `worldobject`

### Target design

```go
// internal/gameserver/world/visibility.go
type Tracked interface {
	ObjectID() int32
	presence() *Presence
}

// Player is a tracked object that is an online player character. It is the
// only kind whose presence keeps a Region active.
type Player interface {
	Tracked
	CharacterName() string
}
```

- `worldobject` package deleted. `attackable.Combatant` (`model/actor/attackable/combatant.go`)
  declares `ObjectID() int32` inline instead of embedding `worldobject.Object`; same for
  `effect.Participant` (`skill/effect/interfaces.go:18`). After this `attackable` and `world` import
  no model packages.
- `world.Player` loses the `WorldPlayer()` marker (pure type-marker methods are the Java marker
  interface). `CharacterName()` is the discriminator: `State` already needs it for `PlayerByName`, so
  the private `namedPlayer` interface (`state.go:52`) and its two asserts disappear. Remove
  `WorldPlayer()` from `player.Character` (`character_runtime.go:239`), `worldtest.Player`, and the two
  local copies `handler/skill/damage.go:391` (`worldPlayerTarget`) and
  `model/actor/creature/formula.go:233` (`worldPlayerMarker`) — those two need "is this a player" and
  are handled in Phase C (they get `Kind()`); in Phase A change them to assert
  `interface{ CharacterName() string }` so A ships green.
- `registry` (`world/registry.go`) becomes generic:

```go
type registry[T any] struct {
	mu      sync.RWMutex
	entries map[int32]T
}
```

  `State` fields: `objects registry[Tracked]`, `players registry[Player]`, `summons registry[Tracked]`.
  Public API returns concrete element types: `Object(id) (Tracked, bool)`, `Objects() []Tracked`,
  `Player(id) (Player, bool)`, `PlayerByName(name) (Player, bool)`, `Players() []Player`,
  `Summon(ownerID) (Tracked, bool)`, `AddPlayer(Player)`, `AddSummon(ownerID, Tracked)`,
  `RemoveSummon(ownerID)`. Delete the alias pairs `AddPet/RemovePet/Pet` (keep the `Summon` names; the
  registry is keyed by owner id — keep that comment).
- `Spawn(t Tracked, …)` already registers `t`; `AddObject` stays for the door/static-object path in
  `data/manager/worldobjects.go`.

### Call-site rules

1. **Never assert to an anonymous interface literal** (`x.(interface{ Foo() })`). Phase A clears
   the ones in packages that can import the concrete actor types (`network` 4, `gameservertest` 13,
   `data/manager` 2, `cmd` 1): assert or type-switch on the concrete type instead —
   `o.(*summon.Actor)`, `o.(*grounditem.Item)`, `o.(*livePlayer)`. The 55 inside actor/domain
   packages are the C′ cluster and are cleared in Phase C by widening the handle types (they cannot
   import the concrete actors). The 7 `BroadcastFrame` ones die in B. Repo-wide gate = 0 after C.
2. `network/targeting.go:resolveTarget` returns `world.Tracked` straight from `State.Object/Player`
   with no assert.
3. `SetTarget` parameters: `player.Character.SetTarget(worldobject.Object)` +
   `SetTargetTracked(world.Tracked)` merge into one `SetTarget(world.Tracked)`
   (`character_target.go:19,49`); `summon.Actor.SetTarget(worldobject.Object)`
   (`live_accessors.go:551`) and `summon.CommandContext.Target` become `world.Tracked`;
   `summon.Actor.target` field likewise. This removes the 9 `.(world.Tracked)` asserts in
   `player/character_runtime.go:273`, `player/character_target.go:53,88`, `npc/hostile.go:825`,
   `summon/live_accessors.go:455,556`, `handler/target/target.go:162`, `network/targeting.go:199`,
   `move/controller.go:131` (widen `Controller.self` to `world.Tracked`).
4. Delete the no-op assert `active.(interface{ ObjectID() int32 })` in
   `summon/live_accessors.go:59` (`Summon()` now returns `Tracked`).
5. `internal/gameservertest/boot.go` has 15 anonymous asserts on `s.World.Player(id)` results
   (lines ~400-671). Collapse into one helper that resolves an online player to
   `*player.Character` through a single seam (one exported accessor on the live player type in
   `network` is acceptable), then call concrete methods.
6. Other typed-return consumers to fix (measured list, 20 files): `network/{taskeffects,
   summon_spawn, summon, inventory, attack_stance_effects, trade, playerclock, pet_name, pet,
   restartpoint, relations, lifecycle, item_shot_use, client_loop, magic_skill, character_flow}.go`,
   `data/manager/{npcs_respawn, npcs_rewards, worldobjects}.go`, `task/{npc_regen,effects}.go`,
   `model/actor/npc/hostile_escort.go:152`, `cmd/gameserver/{tasks,network}.go`.

### Verification (A)

```bash
rg -n "worldobject" acis_golang/internal acis_golang/cmd            # expect 0
rg -n "\.\(interface\{" acis_golang/internal --type go -g '!*_test.go'   # expect 0 (or only the 2 effect/helpers.go ones if C is deferred)
rg -n "WorldPlayer\(\)" acis_golang/internal                         # expect 0
rtk go -C acis_golang build ./... && rtk go -C acis_golang vet ./...
rtk go -C acis_golang test ./internal/gameserver/world/... ./internal/gameserver/model/... ./internal/gameserver/network/...
make -C acis_golang test-race        # MariaDB must be up: make -C acis_golang test-db-up (container acis-test-mariadb)
```

---

## Phase B — events out, packets in the network layer

### Why a sink and not only return values

Pure "return `[]Event` from every method" cannot cover today's code: ~40% of hook firings originate
on goroutines with no network caller to return to (effect ticks in `skill/effect`, regen/charge/
short-buff timers inside `Character`, AI tick for NPCs, cross-actor hits where the *target's*
status must be delivered from the *attacker's* handler). Until the owner-queue refactor lands,
the domain needs one push point. So:

- **One** field per actor: `sink event.Sink`, set once at attach, immutable afterwards (no mutex —
  the attach happens before the actor is published into `world.State`, whose registry mutex gives
  every other goroutine a happens-before edge; the code already relies on this for
  `summon.Actor.frames`, see `summon/live.go:101-108`).
- Domain code calls `c.emit(event.LevelUp{…})`. It never names a packet or a verb like
  "broadcast"/"send". Nil sink = drop (domain tests need no network).
- **Synchronous handler paths additionally return typed outcomes** where the handler branches on
  them (e.g. `AddExp` returning whether a level-up happened). Rule: *if the only caller is a
  network handler, prefer a return value; if the origin is a timer, effect tick, AI, or another
  actor, emit an event.* Both are allowed on the same method.
- Network implements `Sink` with one type switch per actor kind (`livePlayer.Emit`,
  `summonSink.Emit`, `npcSink.Emit`, `doorSink.Emit`). This is the `tea.Msg` / `fsnotify.Event` /
  `ast.Node` sum-type idiom, not an observer with 67 methods.

### New leaf package `internal/gameserver/model/actor/event`

Imports allowed: `time`, `model/location`, `model/skill`. Nothing else — verify with
`go list -f '{{join .Imports "\n"}}' ./internal/gameserver/model/actor/event`. (`attack` imports
`creature` → `effect`, and `move` imports `world`, so `event` must not import them; the payload
structs move *into* `event` instead.)

```go
// Event is one thing that happened to an actor which something outside the
// domain must react to. The set is closed: every type lives in this package.
type Event interface{ event() }

// Sink receives an actor's events. Implementations must be safe to call from
// any goroutine and must not block.
type Sink interface{ Emit(Event) }

// Recorder is the test Sink: it appends every event under a mutex.
type Recorder struct{ mu sync.Mutex; events []Event }
func (r *Recorder) Emit(e Event)
func (r *Recorder) Events() []Event
func Count[T Event](r *Recorder) int
```

Payload types relocated here (keep the old name as a type alias for one phase to shrink the diff,
delete the alias at the end of B): `attack.Snapshot`+`SnapshotHit` → `event.Attack`,
`event.AttackHit`; `move.Event` → `event.Move`; `creature.MagicSkillUse` → `event.MagicSkillUse`;
`player.Stance` → `event.Stance`; `player.ShortBuffUpdate` → `event.ShortBuff`.

Event catalogue, derived 1:1 from the existing hooks (merge duplicates while implementing; names
are suggestions, payloads are the current hook parameters):

| Existing Character hook (setter in `player`) | Event |
|---|---|
| `SetStatusBroadcaster` / `SetMPStatusBroadcaster` | `VitalsChanged{IncludeMP bool}` |
| `SetRegenMaxSender(count, period, hpRegen)` | `RegenMax{Count, Period int32; HPRegen float64}` |
| `SetLackHPNotifier` / `SetLackMPNotifier` | `EffectDroppedLackHP{}` / `EffectDroppedLackMP{}` |
| `SetHealRestoredNotifiers(hp, mp)` / `SetCPRestoredNotifier` | `Restored{Resource; HealerName string; Amount int; ByOther bool}` |
| `SetEffectExpiryNotifiers(wornOff, disappeared, aborted)` | `EffectEnded{Reason; SkillID skill.ID; Level int}` |
| `SetSpoilNotifiers(already, success)` | `SpoilResult{Already bool}` |
| `SetOverHitNotifier`, `SetServitorVanishedNotifier`, `SetRelaxHPFullNotifier`, `SetShieldBlockNotifiers(success, perfect)`, `SetMagicFailureNotifiers(...)` | one small struct each (`OverHit{}`, `ServitorVanished{}`, `HPFull{}`, `ShieldBlock{Perfect bool}`, `MagicFailed{Kind…}`) |
| `SetAttackBroadcaster(attack.Snapshot)` | `Attack{…}` (payload type lives here now) |
| `SetMoveBroadcaster(move.Event)` / `SetStopBroadcaster` / `SetPositionBroadcaster` | `Moved{…}` / `Stopped{}` / `PositionChanged{}` |
| `SetAutoAttackStopBroadcaster` | `AutoAttackStopped{}` |
| `SetDieBroadcaster` / `SetFakeDeathReviveBroadcaster` | `Died{}` / `FakeDeathRevived{}` |
| `SetStanceBroadcaster(Stance)` | `StanceChanged{Stance}` |
| `SetAbnormalEffectUpdater` / `SetAbnormalEffectBroadcaster` | `AbnormalEffectChanged{Mask int32}` (one event; network decides self+broadcast) |
| `SetMagicSkillUseBroadcaster(MagicSkillUse)` | `MagicSkillUse{…}` |
| `SetFlightBroadcaster(loc, flight)` | `Flight{Dest location.Location; Flight skill.Flight}` |
| `SetShortBuffBroadcaster(ShortBuffUpdate)` | `ShortBuff{…}` |
| `SetBowDrawNotifier(gaugeMs)` | `BowDrawn{GaugeMs int}` |
| `SetChargesUpdater` / `SetChargeMessageSender(charges, maxed)` | `ChargesChanged{Charges int; Maxed bool}` |
| `SetExpSpGainNotifier` / `SetExpSpLossNotifier` / `SetLevelUpBroadcaster` / `SetLevelRefresher` / `SetUserInfoUpdater` | `ExpSPChanged{Exp int64; SP int; Gain bool}`, `LevelChanged{Old, New int}`, `UserInfoChanged{}` |
| `SetKarmaChangeNotifier` / `SetRelationBroadcaster` / `SetPvPFlagHook(useFlagged)` | `KarmaChanged{Karma int}`, `RelationChanged{}`, `PvPFlagged{UseFlaggedDuration bool}` |
| `SetDeathPenalty{Raised,Reduced,Skill}Updater` | `DeathPenaltyChanged{Old, New int}` |
| `SetGradePenaltyUpdater`, `SetItemStatsRefresher`, `SetWeightPenaltyUpdater` | `GradePenaltyChanged{}`, `ItemStatsChanged{}`, `WeightPenaltyChanged{Level int}` |
| `SetSummonConfirmSender(...)` | `SummonConfirmRequested{CasterName string; CasterID int32; X,Y,Z int; Timeout time.Duration}` |
| `SetTeleportHook(x,y,z,radius)` | `TeleportRequested{location.Location; Radius int}` |
| `SetZoneRevalidator(previous)` | `Relocated{Previous location.Location}` |
| `SetAttackTargetHook(Tracked)` / `SetRetargetHook(Tracked)` | `AttackRequested{TargetID int32}` / `Retargeted{TargetID int32}` — or return values if the only callers are handlers; check with `rg` |
| `SetHerbConsumer(itemID)` | `HerbConsumed{ItemID int32}` or return value (same rule) |
| `SetFrameSender` / `SetBroadcastFrameSender` (`character_attack.go:128-140, 365-385`) | **delete.** `Character.SendFrame/BroadcastFrame` are transport pass-throughs used by `network` (`targeting.go:41`); they move onto `livePlayer`. After B: `rg "wire\." internal/gameserver/model/actor/player` → 0. |

Config/dependency setters are **not** events. They collapse into two structs passed once:

```go
// player.Rules is the server-configuration slice a character's own rules read.
type Rules struct {
	RateKarmaExpLost, WeightLimitMultiplier float64
	PerfectShieldBlockRate, MaxBuffsAmount, DeathPenaltyChance int
	AllowDelevel, RaidCursesDisabled, AwardPKKillPVPPoint, CanGiveDamage bool
}

// player.Runtime is everything a persisted Character needs to act in the live world.
type Runtime struct {
	World  *world.State
	LOS    LineOfSight
	Zones  PeaceZoneQuery
	Skills SkillLookup      // today's unexported skillDefinitions
	Levels *LevelTable
	Log    zerolog.Logger
	Roll   func(int) int     // nil → math/rand
	FloatRoll func(float64) float64
	Sink   event.Sink
	Rules  Rules
}

func (c *Character) Attach(live *creature.Live, rt Runtime)   // replaces AttachLive + ~20 setters
```

Setters replaced by `Attach`: `SetWorld, SetLineOfSight, SetZones, SetSkillDefinitions,
SetLevelTable, SetLogger, SetRollSource, SetFloatRollSource, SetRateKarmaExpLost, SetAllowDelevel,
SetRaidCursesDisabled, SetPerfectShieldBlockRate, SetMaxBuffsAmount, SetWeightLimitMultiplier,
SetDeathPenaltyChance, SetAwardPKKillPVPPoint, SetCanGiveDamage`. Keep as ordinary state mutators
(not hooks): `SetHP/SetCP/SetXYZ/SetRunning/SetStanding/SetInCombat/SetFlying/SetHero/
SetOperating/SetFishing/SetTransformed/SetSpawnProtection/SetInPvPZone…/SetAutoSoulShot/
SetChargedShot/SetSkillLevel/SetSkillReuse/SetGroundTarget/SetCastModifiers/SetLastKnownPosition/
SetOnlineTime/SetResourceValues/SetHeadingTo/SetDeathPenaltyLevel/SetTarget`. `SetCastController`
and `SetSummonSpawner` are consumer-defined interfaces for network-owned controllers (legitimate
seams); pass them through `Runtime` or a second `AttachControllers(cast, spawner)` call instead of
two setters — implementer's choice, but no more per-hook setters.

### Network side (player)

`network/live_player.go`: `livePlayer` gains `link *GameClientLink` and implements
`event.Sink`. The 67 closure bodies in `character_flow.go:430-830` move into
`func (p *livePlayer) Emit(ev event.Event)`'s `case` arms (or one small method per arm when a body
exceeds ~10 lines). **Move each body verbatim** — the sequence of `SendFrame`/`TrySendFrame`/
`ForEachKnown` calls is the observable packet order. `attachLivePlayer` shrinks to: build
`Runtime`, `c.Attach(live, rt)`, construct controllers.

### Summon, NPC, door

- `summon.Actor`: `frames FrameBuilder` + `statusUpdater, ownerInfoRefresher, damageNotifier,
  expNotifier, onDespawn, broadcastAutoAttackStop, onAbnormalUpdate` → `sink event.Sink`. Events:
  `Attack, Moved, MoveToPawn{TargetID, Distance int; Origin}, Stopped, SkillUse, StatusChanged,
  OwnerInfoChanged, Damaged{AttackerName string; Damage int32}, ExpGained{int64}, Despawned,
  AutoAttackStopped, AbnormalEffectChanged`. Constructor: `NewPet(cfg)`/`NewServitor(cfg)` take
  `cfg.Sink`. `SetAI`, `SetZones`, `SetLineOfSight` → config fields.
- `npc.Hostile`: `FrameBuilder` (`npc/frames.go:28`, 12 methods) + `rewards`, `roll`, `log`,
  `world`, `los`, `weapon` setters → `npc.Runtime{World, LOS, Log, Roll, Items *item.Table, Sink}`
  passed at construction in `data/manager/npcs_spawn.go:190-193`. Events add `Info/ObjectInfo
  {npcinfo.Snapshot}`, `SkillLaunched{TargetIDs}`, `SkillCanceled`, `Died{Sweep bool}`, `FlyTo`,
  `Status{Attrs}`. **Fan-out moves out of the domain**: today `hostile*.go` and
  `summon/live_broadcast.go` call `world.ForEachKnown` + `BroadcastFrame` themselves (37 sites in
  `model/`); the network sink does that. After B: `rg "wire\.|ForEachKnown|BroadcastFrame\(" internal/gameserver/model` → 0.
- Sink implementations live in `network` (`npcSink{h *npc.Hostile; world; …}`). `data/manager`
  must not import `network`; give `manager.Npcs` a `NewSink func(*npc.Hostile) event.Sink`
  dependency supplied from the composition root (`cmd/gameserver`, Fx already wires `Npcs`). Same
  for `door.Object`'s `DoorFrameBuilder` (`data/manager/worldobjects.go:186`).
- `creature.Rewarder` stays (it is a real strategy with one production impl plus tests; but
  re-check the one-implementation rule when touching it).

### Controllers

`cast.Controller` (`onAbort, onFinish, onStopAck`, `controller.go:171-173`), `attack.Controller`
(`SetFinished, SetStarted`), `move.Controller` (`SetArrived`): each takes `sink event.Sink` in its
constructor and emits `CastAborted{Interrupted}`, `CastFinished{Interrupted; Skill skill.Definition;
TargetID int32}`, `CastStopAck{}`, `AttackStarted{}`, `AttackFinished{}`, `Arrived{}`. The network
closures they currently receive (`live_player.go:348-369`, `character_flow.go:~505-530`) become
`case` arms on `livePlayer.Emit`. `move.Controller.SetPositionUpdates(l.positions)` is a
dependency, not a hook — constructor param. Internal closures (`fusionEnd`, `afterFunc`,
`scheduledTimer`) are implementation details, leave them.

### Tests

- Domain tests that today install a counting hook (12 test files:
  `rg -l "Set[A-Za-z]*(Broadcaster|Notifier|Updater|Sender|Hook|Refresher)\(" -g '*_test.go'`)
  switch to `rec := &event.Recorder{}` + `event.Count[event.VitalsChanged](rec)`.
- Behavior suites in `tests/` are unchanged and must pass byte-for-byte.

### Docs to amend (part of B)

`docs/agents/server-initiated-updates.md`, "Rules" bullet 2–4: replace the `Set<Thing>Updater` +
`Update<Thing>` instruction with: *emit an `event.Event` from the code path that makes the change;
the actor's `event.Sink` (set once at attach, nil in domain tests) carries it; the network layer
maps it to packets in that actor kind's single `Emit` type switch; cover delivery with a domain
test asserting on `event.Recorder`.* Add: *never add a func-typed hook field or a `Set<Thing>Hook`
setter to an actor; add an event type instead.*

`docs/agents/go-style.md`, anti-pattern table, add rows:
| `func`-typed callback field + `Set<Thing>Hook` setter per notification | one `event.Sink`; a small `event.X` struct per fact; network maps in one type switch |
| optional-capability interface asserted at runtime (`t.(fooTarget)`) | a required method on the consumer's actor interface, `Kind()` for the closed set of actor kinds, at most one `asPlayer/asNPC` helper per package |
| universal base interface (`ObjectID()` only) stored in registries + downcast | registries typed by what they hold (`registry[T]`), `world.Tracked`/`world.Player` |

`AGENTS.md` "Always-loaded engineering rules": the sentence "network handlers only decode, resolve
context, call domain behavior, and map outcomes to packets" already states the rule — append
"; server-initiated changes reach the network as `event.Event` values, never as callbacks".

### Verification (B)

```bash
rg -n "Set[A-Za-z]+(Broadcaster|Notifier|Updater|Sender|Hook|Refresher|Spawner)\(" acis_golang/internal --type go -g '!*_test.go'   # expect 0
rg -n "^\s+[a-zA-Z]+\s+func\(" acis_golang/internal/gameserver/model/actor/{player,summon,npc}/*.go | rg -v "roll|Roll"   # expect ≈0 (roll sources allowed)
rg -n "wire\.|ForEachKnown|BroadcastFrame\(" acis_golang/internal/gameserver/model                                            # expect 0
go -C acis_golang list -f '{{join .Imports "\n"}}' ./internal/gameserver/model/actor/event | rg internal                       # only location, skill
go -C acis_golang run golang.org/x/tools/cmd/deadcode@latest ./cmd/gameserver | rg 'model/actor|network'                     # no orphaned emitters/arms
rtk go -C acis_golang test ./internal/gameserver/model/... ./internal/gameserver/skill/... ./internal/gameserver/network/...
make -C acis_golang test-race && make -C acis_golang test
```

Packet-order regression check: run every suite under `tests/` (they compare client-visible frames);
any diff in `tests/combat`, `tests/skills`, `tests/pets` is a moved closure whose send order changed.

---

## Phase C — required interfaces instead of runtime capability checks

### Why

`effect` cannot type-switch on `*player.Character` (import direction: player → effect). So the
Java `if (target instanceof Player) ((Player) target).foo()` became `if p, ok := target.(fooTarget);
ok { p.Foo() }` — 90 such interfaces in one file. Go's answer for a closed set of three actor kinds:
one required interface (methods that make no sense for a kind return the zero value — the Go
equivalent of a base-class no-op), a `Kind()` enum for the rare genuinely kind-specific branch, and
at most one optional interface per package for a cohesive kind-only cluster.

### `actor.Kind`

`internal/gameserver/model/actor` already exists (only `doc.go`). Add:

```go
type Kind uint8
const (
	KindPlayer Kind = iota + 1
	KindNPC
	KindSummon
	KindDoor
	KindStatic
	KindItem
)
```

Every `world.Tracked` implementer gets `Kind() Kind` (Character, Hostile, Decoration, EffectPoint,
summon.Actor, grounditem.Item, door.Object, staticobject.Object, worldtest.Player). Replaces the
`WorldPlayer()` marker leftovers from A (`handler/skill/damage.go`, `creature/formula.go`), and lets
`handler/target.Category` (`target.go:12-24`, a 3-bit RTTI bitmask) be re-derived: `Playable` =
`Kind() ∈ {Player, Summon}`; `Attackable`/`Folk` are NPC template facts → methods `Attackable()`/
`Folk()` on the target actor interface. Collapse `Category` only if it falls out cleanly; it is
not required.

### C′ — `attackable.Combatant` becomes the required creature surface (do this first in C)

This is what kills the anonymous asserts in the actor packages. Today a `Combatant` is
`ObjectID/SiegeGuard/AlikeDead`; `ai/attackable.go:835`, `npc/hostile_target.go:73-135`,
`npc/hostile_attack.go:76-81`, `summon/attack.go:84-89`, `player/character_runtime.go:281-299`, … each
re-assert `Position()`, `CollisionHeight()`, `InPeaceZone()`, `Karma()`, `SilentMoving()`,
`RecentFakeDeath()`, … because the handle is too narrow. The measured union of anonymously asserted
methods in `model/` (30 names; count = sites):

`Position 13, BroadcastFrame 6 (gone in B), CollisionHeight 4, SilentMoving 2, OwnerCombatant 2,
MovementDisabled 2, InPeaceZone 2, Heading 2, CollisionRadius 2, ActingPlayer 2, SpawnProtected,
RecentFakeDeath, RaidRelated, Playable, ObjectID (gone in A), NpcID, Move, Karma, IsMoving, Guard,
FakeDeath, EnableOverhit, CurrentHeading, CharacterName, Category, CanMoveTo, CanGiveDamage,
AttackType, AllSkillsDisabled, Vertices (zone shape — unrelated, leave)`.

Target:

```go
// attackable.Combatant is any creature that can take part in combat. All three
// live actor kinds implement every method; a method that does not apply to a
// kind returns its zero value (documented at the implementation).
type Combatant interface {
	ObjectID() int32
	Kind() actor.Kind
	Position() (x, y, z int)
	Heading() int
	CollisionRadius() float64
	CollisionHeight() float64
	Level() int
	Karma() int              // 0 for NPCs and summons
	Dead() bool
	AlikeDead() bool
	FakeDeath() bool
	RecentFakeDeath() bool
	IsMoving() bool
	MovementDisabled() bool
	InPeaceZone() bool
	SilentMoving() bool
	SpawnProtected() bool
	CanGiveDamage() bool
	RaidRelated() bool
	SiegeGuard() bool
	Guard() bool
	// Owner returns the controlling player of a summon; (nil, false) for every
	// other kind. The one sanctioned "is this owned" branch.
	Owner() (Combatant, bool)
}
```

Rules:
- `creature.DeathActor` (`death.go:5`, one method) is deleted; its uses take `Combatant`.
  `creature.Mortal` embeds `Combatant`. `summon.Owner` (`summon/live.go:38`) embeds `Combatant`
  (drops its duplicated `Position/InCombat` decls where covered) so `a.owner.(creature.DeathActor)`,
  `a.owner.(attackable.Combatant)`, `a.owner.(interface{ SpawnProtected() bool })`,
  `a.owner.(interface{ CanGiveDamage() bool })` (`summon/live_accessors.go:36,534`,
  `summon/formula.go:238,257`) become direct calls.
- `OwnerCombatant()`/`ActingPlayer()` asserts (`hostile_target.go:73,171`, `character_pvpflag.go:97`,
  `character_karma.go:92`) → `Owner()`.
- `Category()` assert in `character_pvpflag.go:85` → `Kind()` (+ `Guard()`).
- `Move() *CreatureMove`/`CanMoveTo` (`move/controller.go:437`, `ai/attackable.go:338`,
  `data/manager/npcs_hostile.go:126`) and `AllSkillsDisabled`/`EnableOverhit`/`NpcID`/`AttackType`
  are controller- or NPC-specific: add them to the *consumer's* named interface (`ai.MoveController`,
  `cast.Actor`, `attack.CreatureActor`, `creature.FormulaActor`) as required methods, not to
  `Combatant`.
- `attackable` must stay a leaf: it imports only `model/actor` (for `Kind`). Verify with `go list`.
- Audit the 52 named unexported one-method interfaces under `model/actor/**` (`rg -n "^type [a-z][A-Za-z]* interface" internal/gameserver/model/actor`): keep the ones that are real
  seams with a test double (`scheduledTimer`, `afterFunc`, `LineOfSight`, `peaceZoneQuery`); fold
  every one whose only use is `x, ok := v.(fooer)` into the interface `v` already has.

Then the `effect`/`handler` recipe below is the same move applied to the remaining consumers.

### `effect` package

1. Inventory the union: `rg -n "^\s+[A-Z][A-Za-z]*\(" internal/gameserver/skill/effect/interfaces.go | rg -o "[A-Z][A-Za-z]*\(" | sort -u` (92 names) plus `helpers.go`'s two anonymous ones.
2. After B, the notifier group is already gone (`statusBroadcaster, mpStatusBroadcaster,
   regenMaxSender, lackHPNotifier, lackMPNotifier, abnormalEffectBroadcaster, effectExpiryNotifier,
   spoilNotifier, healRestoredNotifier, relaxHPFullNotifier, …`) — effect emits through
   `Actor.Emit`.
3. Partition the rest into: **common** (all three actors implement; zero-value default where
   meaningless, with a one-line comment stating the default), **player-only** (charges, karma, PvP
   flag, death penalty, hennas, sit/stand, weapon grade penalty, cubics, servitor ownership),
   **NPC-only** (hate/aggro: `hateRaiser, attackDesireRaiser, mostHatedResetter, hateRandomizer,
   nearbyMonsterFinder, aggroHateControl`, raid, spoil/seed corpse state).
4. Define:

```go
// Actor is every cast participant. Effect.Effector and Effect.Effected are Actors.
type Actor interface {
	ObjectID() int32
	Kind() actor.Kind
	Position() (x, y, z int)
	Heading() int
	Level() int
	Dead() bool
	Emit(event.Event)
	Vitals      // HP, MaxHP, MP, ReduceHPByDOT, ReduceMP, …
	Status      // Paralyzed/Immobilized/Invul/Afraid/… + Set*, abnormal mask
	Combat      // AbortAll, StopMove, Target/ClearTarget, Think, …
	Effects     // EffectList, StopEffects(skillID), IsAffected(Flag)
}

type PlayerActor interface { Actor; /* player-only cluster */ }
type NPCActor    interface { Actor; /* hate/aggro/corpse cluster */ }

func asPlayer(a Actor) (PlayerActor, bool) { p, ok := a.(PlayerActor); return p, ok }
func asNPC(a Actor) (NPCActor, bool)
```

   Sub-interfaces are for readability only; if they end up with one consumer, inline them.
5. `Effect.Effector/Effected` typed `Actor` (`effect/effect.go:20-21`). `Participant` deleted (or
   kept as the alias name if `ReduceHPByDOT`/`FleeFrom` contracts in player/npc/summon need the
   narrow type — prefer changing those signatures to `Actor`).
6. Replace every `x, ok := t.(fooTarget); if ok { … }` with a direct call; player/NPC clusters via
   the two helpers. Delete the optional interface declarations as they lose their last use.
7. Compile-time proofs in each actor package:
   `var _ effect.PlayerActor = (*player.Character)(nil)`, `var _ effect.NPCActor = (*npc.Hostile)(nil)`,
   `var _ effect.Actor = (*summon.Actor)(nil)`.

Expected: 117 asserts → ≤ the `asPlayer`/`asNPC` call sites; `interfaces.go` shrinks from ~490
lines to the three interfaces.

### `handler/target`, `handler/skill`, `creature/formula.go`, `attack/controller.go`

Same recipe per package: `target.Creature` + `AttackRules, SightChecker, Summoner, OwnedCreature,
HolyTarget, UnlockableTarget, UndeadTarget, CorpseTarget, MonsterTarget, FolkOrGuardTarget,
DoorTarget, PlayableCastRules, OlympiadCastState, CorpseDeadlineTarget, PeaceZoner, SpoiledCorpse,
SeededCorpse, PetTarget, PartyMate, ClanAllyMate, …` (`target.go`, `group.go`,
`target_predicates.go`) → one `target.Actor` with required methods + `Kind()`; keep at most one
optional interface for door/static targets if the unlock/holy paths genuinely only apply there
(`handler/skill/unlock.go` already type-switches — fine). `handler/skill/{continuous,disablers,
apply}.go` ~15 optional ifaces → required on the actor type they consume. `attack/controller.go`
`raidRelatedTarget, raidCurseTester, damageReceiver` → methods on `CreatureActor`.
`creature/formula.go` `magicFailureWeapon`, `worldPlayerMarker` → `FormulaActor` gains
`WeaponGradePenalty() bool` and `Kind()`.

### Verification (C)

```bash
rg -n "\.\(interface\{" acis_golang/internal acis_golang/cmd --type go -g '!*_test.go'                    # expect 0 repo-wide (zone/form.go Vertices may remain: 1)
rg -n "creature\.DeathActor|\.OwnerCombatant\(\)|\.ActingPlayer\(\)" acis_golang/internal --type go -g '!*_test.go'   # expect 0
go -C acis_golang list -f '{{join .Imports "\n"}}' ./internal/gameserver/model/actor/attackable | rg internal   # only model/actor
rg -c "^type [a-z][A-Za-z]* interface" acis_golang/internal/gameserver/model/actor -g '!*_test.go'          # expect ≤ 15 (from 52), each with a test double or ≥2 impls
rg -n "\.\([a-z][A-Za-z]*(Target|Source|Notifier|Marker|Stopper|Raiser|Owner|Checker|Rules|Mate|Holder|State)\)" acis_golang/internal/gameserver/skill acis_golang/internal/gameserver/handler acis_golang/internal/gameserver/model --type go -g '!*_test.go'   # expect 0
rg -c "^type [a-z][A-Za-z]* interface" acis_golang/internal/gameserver/skill/effect/interfaces.go   # expect 0 (file deleted or holds only Actor/PlayerActor/NPCActor)
rg -n "WorldPlayer|Category\(\)" acis_golang/internal   # expect 0 (Category only if collapsed)
rtk go -C acis_golang test ./internal/gameserver/skill/... ./internal/gameserver/handler/... ./internal/gameserver/model/...
make -C acis_golang test-race && make -C acis_golang test
```

---

## Cross-phase gates (run before declaring any phase done)

```bash
find acis_golang -name '*.go' -type f -exec gofmt -l {} +      # no output
rtk go -C acis_golang vet ./...
rtk go -C acis_golang build ./...
make -C acis_golang test-unit
make -C acis_golang test-race                                   # needs: make -C acis_golang test-db-up
make -C acis_golang test
```

Behavior oracle: `tests/{character,combat,items,lifecycle,pets,skills,trade}` (243 scenarios) drive
real packets against MariaDB through the production boot path. They are the acceptance test for
"nothing observable changed".

## Risks and how each phase contains them

- **Packet order drift (B).** Each moved closure keeps its statement order; a `case` arm that
  previously was two hooks fired in sequence (e.g. `updateAbnormalEffect` then
  `broadcastAbnormalEffect`) becomes one event whose arm performs both in the same order. Diff
  the `tests/` suites, not just unit tests.
- **Happens-before on `sink` (B).** `Attach` must run before `world.State.Spawn/AddPlayer/
  AddSummon`. Add a `-race` test that spawns then emits from a second goroutine.
- **Import cycles (B).** `event` is a leaf; if a payload needs a type from `attack`/`move`/
  `creature`/`player`, move the type into `event`, do not import.
- **Silent no-ops (C).** A zero-value default on the common `Actor` interface for a method that
  the reference treats as kind-specific behavior must be a *deliberate* default; document it in one
  line at the method. When in doubt, put the method on `PlayerActor`/`NPCActor` instead.
- **Scope creep.** No formula, persistence, or handler-logic change rides along. Naming (Phase D)
  is out; if a rename is forced by a collision, keep it local.

## Interplay with `docs/concurrency-refactor-plan.md`

That plan replaces per-actor mutexes with owner queues. This work is compatible and helpful:
after B, an actor has one immutable `sink` instead of ~67 mutex-guarded hook fields (fewer lock
sites for it to remove), and `Emit` is the natural place to later append to a per-actor outbox
drained by the owner queue — event types and network `case` arms stay unchanged. Do not start that
plan's Phase 2+ in the same PRs as this work.

## Files most affected (representative, not exhaustive)

- A: `internal/gameserver/world/{state,registry,visibility,region}.go`, `model/worldobject/*`
  (deleted), `model/actor/attackable/combatant.go`, `network/targeting.go`,
  `model/actor/{player/character_target.go, summon/live_accessors.go, summon/live.go}`,
  `gameservertest/boot.go`.
- B: new `model/actor/event/`, `model/actor/player/character.go` (+ every `character_*.go` with a
  `Set*` hook), `network/{character_flow,live_player,summon_spawn,visibility}.go`,
  `model/actor/summon/{live,live_broadcast,live_accessors}.go`, `model/actor/npc/{hostile*,frames}.go`,
  `model/actor/{cast,attack,move}/controller.go`, `data/manager/{npcs_spawn,worldobjects}.go`,
  `cmd/gameserver/*.go` (sink factory wiring), `docs/agents/{server-initiated-updates,go-style}.md`,
  `AGENTS.md`.
- C: `model/actor/kind.go` (new), `model/actor/attackable/combatant.go` (widened),
  `model/actor/creature/death.go` (`DeathActor` deleted), `model/actor/summon/live.go` (`Owner`),
  `model/actor/ai/attackable.go`, `model/actor/npc/{hostile_target,hostile_attack,hostile_escort}.go`,
  `model/actor/player/{character_runtime,character_pvpflag,character_karma,character_stats}.go`,
  `skill/effect/{interfaces,helpers,core,…}.go`,
  `handler/target/{target,group,target_predicates}.go`, `handler/skill/{continuous,disablers,apply,
  damage}.go`, `model/actor/creature/formula.go`, `model/actor/attack/controller.go`, plus `Kind()`
  on the nine `Tracked` implementers.
