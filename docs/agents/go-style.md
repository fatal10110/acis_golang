# Go style and design guide

This guide is loaded when a task needs detailed Go design guidance. Repository-wide behavioral
requirements remain in the outer `acis_public/CLAUDE.md`.

## Naming

- Package names are short, lowercase, and describe a domain or responsibility. Avoid stutter at call
  sites.
- Types are nouns; functions and methods are verbs or value names. Keep initialisms consistent:
  `ID`, `HP`, `NPC`, `URL`.
- Receivers are short and consistent. Do not use `this` or `self`.
- Treat `Manager`, `Holder`, `Impl`, `Base`, `Abstract`, `Helper`, `Util`, and `Info` as design-smell
  suffixes when they hide an unclear responsibility. Use a precise domain name instead.
- Name interfaces for behavior (`Reader`, `Attacker`, `Persister`), never with an `I` prefix.
- Avoid getter/setter walls. Export plain data when appropriate or expose a method such as `Name()`
  when access has behavior.

## Data and type shape

- Prefer composition to simulated inheritance. Embed or hold only genuinely shared concerns; do not
  build chains of base structs.
- Make the zero value useful where practical. Use a constructor when validation, dependencies, or
  initialization are required.
- Small immutable data is usually a value. Identity-bearing mutable entities are usually pointers.
  Do not make every value a pointer by habit.
- Use named IDs and enums when mixing raw values would be error-prone. Preserve the concrete integer
  width required by the external contract.
- Use `(T, bool)` for ordinary in-memory lookup absence and `(T, error)` for fallible work. Nil is
  idiomatic when it has one clear meaning; do not create nullable object graphs by default.
- Keep structs cohesive. Split independent responsibilities into fields or neighboring types when
  the aggregate becomes difficult to reason about.

## Interfaces

- Define interfaces in the consuming package, with only the methods that consumer calls.
- Accept interfaces and return concrete values.
- Do not add an interface for one implementation unless a real consumer needs a seam, such as a
  focused test double. A hypothetical future implementation is not enough.
- Prefer direct function parameters to service locators, registries of arbitrary values, or
  reflection-driven discovery.

## Errors and panic isolation

- Return `error` for expected failure and wrap it with useful operation context using `%w`.
- Use sentinel or typed errors only when callers must branch on the category; inspect with
  `errors.Is` or `errors.As`.
- Panic only for programmer errors and impossible invariants. Invalid packets, files, or database
  data return errors to the appropriate isolation boundary.
- Recover at infrastructure isolation boundaries such as connection loops, worker dispatch,
  schedulers, and script or plugin execution. Log the stack and terminate the affected unit of work.
  Do not recover merely to continue after an invariant violation, and do not require recovery in
  every short-lived goroutine.

## Concurrency

- State the owner of shared mutable state on the owning type or field: one goroutine, one mutex, or
  another explicit synchronization mechanism.
- Use channels to transfer ownership or coordinate work; use a mutex for a short critical section
  around shared state. Choose the simpler correct model.
- Prefer `sync.Mutex`. Use `sync.RWMutex` only for genuinely read-heavy state where contention or
  critical-section cost makes it useful; the existence of both readers and writers is not enough.
- A read path that mutates, including lazy eviction, takes the write lock.
- One goroutine writes a connection. Long-running work takes a `context.Context`, stops on
  cancellation, and has no orphaned goroutines.
- Keep periodic callbacks short. Move blocking database or network work out of a scheduler's critical
  path while preserving lifecycle ownership.

### Lock order

Actors are written from other actors' queues (a killer's queue applies the victim's death loss, a
caster's queue lands a buff), so a lock taken in the wrong order deadlocks two queues. The rules:

- **Leaf by default.** A leaf lock's critical section touches only the fields it guards, atomics,
  pure helpers, immutable identity reads of another actor (`ObjectID`, `Kind`), and the owning
  queue's scheduler (arming or stopping a task). It calls no other
  object, takes no other lock, and emits no event. Only the locks marked "outer" below are held
  across calls.
- **Defer the follow-ups.** Events, packets and refreshes that a change triggers, and anything that
  reads the state back, run after the lock is released: collect them as closures
  (`progressionHooks`, the effect list's pending hooks) and run them after `Unlock`.
- **Read prerequisites first.** Values a locked step needs from elsewhere are read before the lock
  is taken (the death loss reads the Lucky skill and the level table before `progressionMu`).
- **One actor at a time.** Never hold a leaf lock of one actor while taking any lock of another.
  Read from one, release, then lock the other (`awardKillerPKKarma` reads the victim, then locks
  the killer). The only cross-actor nesting is the one the table names for `reviveMu`.
- **Callbacks under a lock say so.** An interface method called with the caller's lock held
  documents it (`effect.StatOwner.AttachStatFuncs`, `MaxBuffCount`,
  `effect.ActivityRegistry.SetActive`), and its implementations take only leaves below that lock.
- **A new nesting updates this table** and comes with a test that drives both paths concurrently
  and fails on a timeout (see `character_skill_lock_order_test.go`).

`player.Character`, outer to inner. A goroutine takes a lock only while it holds none listed below
it:

| Order | Lock | Held across calls | Why |
| --- | --- | --- | --- |
| 1 | `reviveMu` | outer | A revive is one step: it reads and stops effects, restores exp (`progressionMu`), sets vitals (`vitalsMu`), emits events and restarts the mount feed under it. A resurrection offer for a dead summon reads the summon's effect list under it. Nothing takes it while holding another lock: a pet's revive, which closes its owner's offer, runs after the owner releases it. |
| 2 | `charmMu` | outer, only around one effect-list read | It serializes the Charm of Courage mirror refresh. Taken before, never under, the effect list's lock; the charm's hooks run after the list unlocks. |
| 3 | `effect.List.mu` (`EffectList()`) | only into the owner's `AttachStatFuncs` and `MaxBuffCount`, and the effect-tick registry (`task.Effects.SetActive`) | Start/exit hooks, `StatFuncsAttached`, icon refreshes and messages are queued and run after it is released. |
| 4 | `stateMu`, `skills.mu`, `statMu`, each `effect.Calculator.mu` | leaf | These four are reachable under the effect list's lock: `MaxBuffCount` reads `stateMu` and the Divine Inspiration level (`skills.mu`), and `AttachStatFuncs` takes `statMu` and the calculators. Calling into the effect list while holding one of them (`IsAffected`, `Flags`, `AllSkillsDisabled`, `Stunned`, `Add`) deadlocks against a buff landing from another queue. |
| 5 | `progressionMu`, `vitalsMu`, `locMu`, `summonFriendMu`, `saved.mu`, `boat.mu`, `recommendations.mu`, `mountFeed.mu` | leaf | Never held across a call; follow-ups go through hooks or run after unlock. |

Leaves never nest in one another, so the order within a row does not matter. `stateMu` and
`mountFeed.mu` may arm or stop a task on the character's queue (`afterLocked`, `stopLocked`); the
queue's own lock is below every lock in the table. So is the effect-tick registry's lock
(`task.Effects.mu`), which the effect list takes under its own in `notifyActivityTransition` (a
list gains its first effect or loses its last) and `Untrack`: a registry-side path never reads a list
while holding it (`Effects.Tick` and `Reset` snapshot the entries and unlock before touching a list).

Creature locks follow the same rules. `creature.HPBar.Publish` holds the bar's lock while it reads
HP and hands the value to `send`; `send` must not block and must not take a lock a `Publish`
caller may hold.

## Numeric and contract fidelity

- Use explicit integer widths when overflow, truncation, signedness, or wire size is observable.
- Preserve formula operation order and `float64` precision where the specification requires it.
- Use explicit byte order and framing. Standard-library encoders are preferred when they express the
  exact contract clearly.
- Match random distributions, boundary conditions, and rounding semantics, not merely average
  outcomes.
- Name compatibility algorithms for what they compute and document their contract. Production code
  must not cite the source implementation.

## Packages, files, and wiring

- Organize by domain or responsibility under `internal/`; keep `cmd/<binary>` as the composition and
  lifecycle boundary.
- Put a helper beside the type or concept it operates on. A storage or network package must not own a
  domain codec merely because it was the first caller.
- Keep files cohesive and navigable. Split when responsibilities can be understood independently or
  the file is difficult to work with; do not enforce one concern or one type per file mechanically.
- Avoid import cycles by repairing the boundary rather than adding indirection.
- Prefer explicit constructor wiring. The gameserver and loginserver composition roots already use
  Uber Fx for lifecycle and object-graph wiring; extend that established root wiring when needed,
  while keeping domain packages independent of the container.
- Mutable package globals are a last resort. Constants and pure package functions are fine.

## Dependencies

Stop at the first rung that completely solves the real requirement:

1. The feature is unnecessary: do not add it.
2. The standard library covers it: use it.
3. An existing dependency covers it: reuse it.
4. A small established package owns a non-trivial subsystem better than local code would: evaluate
   it deliberately.
5. Otherwise write the smallest maintainable local implementation.

Use zerolog for repository logging. Do not build custom logging, dependency-injection, parsing,
retry, or pooling frameworks when the adopted library or standard library already owns the problem.

## Comments and documentation

- Exported declarations have doc comments beginning with the declaration name when required by the
  repository's lint policy.
- Comments explain an invariant, unit, ownership rule, compatibility contract, or non-obvious reason;
  they do not narrate syntax.
- Production names, comments, and commit messages describe this Go system only. Reference evidence
  belongs in tests, docs, issues, pull requests, or `.agent-cache` reports.

## Anti-patterns

| Smell | Prefer |
| --- | --- |
| singleton `XManager` | a precisely named value constructed at the composition root |
| getter/setter for every field | plain data or one behavior-bearing accessor |
| one-implementation interface | the concrete type until a consumer needs a seam |
| base-struct inheritance chain | composition of small shared concerns |
| every value is `*T` | values for immutable data; pointers for mutable identity |
| panic for external input | a contextual error handled at an isolation boundary |
| unguarded shared map | one documented owner or mutex |
| `Util` grab bag | functions beside the domain concept they serve |
| domain rules in packet handlers | a domain API returning typed outcomes |
| hidden registration in `init` | explicit composition-root registration |
| `func`-typed callback field + `Set<Thing>Hook` setter per notification | one `event.Sink`; a small `event.X` struct per fact; network maps in one type switch |
| optional-capability interface asserted at runtime (`t.(fooTarget)`) | a required method on the consumer's actor interface, `Kind()` for the closed set of actor kinds, at most one `asPlayer/asNPC` helper per package |
| universal base interface (`ObjectID()` only) stored in registries + downcast | registries typed by what they hold (`registry[T]`), `world.Tracked`/`world.Player` |

## Worked example

This registry has explicit ownership, ordinary absence, and no speculative interface:

```go
type Registry struct {
	mu      sync.Mutex // guards objects
	objects map[ObjectID]Object
}

func NewRegistry() *Registry {
	return &Registry{objects: make(map[ObjectID]Object)}
}

func (r *Registry) Find(id ObjectID) (Object, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.objects[id]
	return o, ok
}
```

If profiling later shows read-lock contention is material and reads dominate, replace the mutex with
an `RWMutex` and add the corresponding race/performance evidence then.
