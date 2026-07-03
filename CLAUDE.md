# CLAUDE.md — how to write code in this repository

This is a Go game server. **Write idiomatic Go.** Not another language's design expressed with Go
syntax — Go.

There is a separate behavioral specification (wire protocol, formulas, data formats, game rules). Your
job is to **reproduce the required *behavior*** — the bytes on the wire, the numbers a formula returns,
the contents of a saved row. Your job is **not** to reproduce the *structure* of any reference
implementation. Same observable behavior, native Go shape.

If a rule here ever seems to conflict with matching behavior, re-read: behavior of **observable
output** is non-negotiable; **internal structure** is 100% yours to make idiomatic. These never
actually conflict — a byte-identical packet can be produced by clean Go.

---

## Reference docs — consult before you build

The behavioral specification lives in the reference repository, at
**`../aCis_gameserver/docs/go-rewrite/`** (start at its `README.md`, which indexes the set). Before
implementing any area, read the doc(s) that cover it — they pin down the required behavior precisely,
down to bytes and constants:

- **Protocol & transport** — framing, the cipher algorithms, opcode tables, and the exact
  connect → character → enter-world packet sequence.
- **Engine** — world model, AI/intention model, combat & stat formulas, skills & effects,
  items & economy.
- **Data & persistence** — data-file formats and their loaders, the database schema, and the geodata
  format & pathfinding.
- **Systems & content** — the major game systems, the quest/script model, and lifecycle &
  scheduled work.
- **Execution** — the milestone playbook: what to build in what order, and how each piece is verified
  against known-good behavior.

Use these docs to learn **what** to produce — the bytes, numbers, sequences, and rules. They are a
specification of behavior, and they describe an existing implementation as the authority for that
behavior. That is precisely the material §0 tells you **not** to echo into this repository: read them,
extract the behavior, then write native Go. Never carry their naming, structure, or provenance into
your code, comments, or commit messages.

---

## Tools

- **grepai** — use for semantic/code search across the codebase when plain `grep`/text search isn't
  enough (finding behavior by meaning, locating where a rule is implemented).
- **openmemory** — use to store and recall cross-session context (decisions made, milestone progress,
  gotchas discovered) so later sessions don't re-derive it.
- **ponytail skill** — apply for the laziest solution that works: YAGNI, stdlib first, shortest diff.
  Matches §9's dependency ladder — use it when tempted to over-build.
- **superpowers skills** — use the process skills (brainstorming before new features,
  systematic-debugging before fixes, test-driven-development for implementation) as the default
  workflow for non-trivial changes.

Reach for them when needed; don't invoke them ritually on every task.

---

## 0. The prime directive

> Model the behavior. Re-express it in Go. Never transliterate.

When you look at how something works elsewhere, extract the *what* (the rule, the sequence, the
formula) and throw away the *how* (the class graph, the naming, the control flow). Then write the Go
that a Go engineer would write to get that *what*.

Behavior parity is mandatory; code-shape parity is not. If the required behavior is already provided
by the Go standard library or by a small, focused package, use that instead of rebuilding a foreign
framework by hand. Cross-cutting behavior such as logging belongs in one Go package with a narrow API
and tests, not scattered ad hoc calls or a copy of another runtime's logging architecture.

Optimize for the least custom code we must own, not for the fewest dependencies at any cost. If the
standard library covers only the primitive and the change would require us to build levels, hooks,
formatters, routing, rotation, retries, parsing, pooling, or other subsystem machinery, stop and use a
small proven package unless there is a concrete incompatibility.

**Absolute rule on references:** the Go source you write — its identifiers, comments, and commit
messages — describes Go code and the behavior it implements, nothing else. Do not name or allude to
any reference implementation, its language, its types, its methods, or its packages in the code you
produce. (The spec docs under `../aCis_gameserver/docs/go-rewrite/` are the one place that material
belongs; consult them, never echo them.) Name every function, type, and constant for **what it does in
this system**, never for where the idea came from.

---

## 1. Naming

- **Packages:** short, lowercase, single word, no underscores, no plurals-for-the-sake-of-it:
  `player`, `world`, `skill`, `geo`, `packet`. The package name is part of every call site —
  `world.Region`, not `worldmodel.WorldRegion`.
- **No stutter:** in package `player`, the main type is `player.Player` only if unavoidable; prefer
  `player.Entity` / `player.State` so call sites read `player.New(...)`. Never `player.PlayerData`.
- **Types:** nouns. **Functions:** verbs or verb phrases. Exported identifiers get doc comments
  starting with the identifier name.
- **Banned name-shapes** (they signal foreign structure, not Go):
  - suffixes `Manager`, `Holder`, `Impl`, `Base`, `Abstract`, `Helper`, `Util`, `Info` used as a
    dumping ground. A "manager" is usually just a package, or a struct named for what it owns
    (`registry`, `pool`, `store`, `table`).
  - interface names prefixed `I` (`IPlayer`). Interfaces are named for behavior: `Reader`,
    `Attacker`, `Persister`.
  - getter/setter walls (`GetName()`/`SetName(...)`). Export the field, or expose a single method
    named for the value (`Name()`), only when access needs logic.
- **Acronyms keep case:** `ID`, `HP`, `NPC`, `URL` — `npcID`, `MaxHP`, `parseNPC`.
- **Receivers:** short, consistent, 1–2 letters (`p *Player`, `w *World`). Never `this` or `self`.

## 2. Types & data modeling

- **Composition, not inheritance.** There is no inheritance in Go and we do not simulate it. Share
  behavior by **embedding** a smaller type or by holding a field, and by satisfying **interfaces**.
  Do not build a tower of "base" structs each embedding the last to fake a class hierarchy — that is
  transliteration. Model each concrete thing as its own struct that embeds only the genuinely shared
  pieces (e.g. a `spatial` for position, a `combat` for HP/attack) and implements the interfaces its
  callers need.
- **Make the zero value useful** where you can, so callers can write `var x T` or rely on struct
  literals without a mandatory constructor. When construction needs work, provide `New...` returning a
  concrete `*T` (or `T`), and an `error` if it can fail.
- **Value vs pointer:** small immutable data → value types, copy freely. Entities with identity and
  mutable state (a `Player`, a `World`) → one `*T`, passed around. Don't make everything a pointer out
  of habit; don't make a shared-mutable thing a value.
- **Typed IDs and enums.** Use `type NpcID int32`, `type SkillID int32` so the compiler catches mixups.
  Enumerations are `iota` constants of a named type with a `String()` method — not bare `int`s, not
  strings.
- **Keep types small and focused.** If a struct grows past a screen or two of fields covering unrelated
  concerns, split it: separate structs per concern held as fields, in separate files, behind one
  aggregate. A 500-line type is a design smell here, not a goal.
- **No nil-as-normal.** Prefer `(T, bool)` or `(T, error)` over returning a nil pointer that every
  caller must remember to check. Reserve nil for genuine absence with a documented meaning.

## 3. Interfaces

- **Define interfaces where they are consumed, not where types are defined.** The `combat` package
  declares the `Target` interface it needs; `player` and `npc` just happen to satisfy it.
- **Keep them small** — one to three methods is the sweet spot. Big interfaces are a foreign habit.
- **Accept interfaces, return concrete types.** Functions take the narrow interface they use and return
  the real struct, so callers keep full access and mocking stays trivial.
- **Do not declare an interface for a type that has exactly one implementation** unless a consumer
  genuinely needs the seam (a test double, a second impl on the roadmap you can point to). Speculative
  interfaces are dead weight — delete them.

## 4. Errors & panics

- **Errors are values.** Return `error` as the last result for anything that can fail for expected
  reasons (bad input, missing row, validation). Do not signal expected failure by throwing/panicking.
- **Wrap with context:** `fmt.Errorf("load skill %d: %w", id, err)`. Inspect with `errors.Is` /
  `errors.As`. Define sentinel errors (`var ErrNotFound = errors.New(...)`) or typed errors where
  callers branch on the kind.
- **Panic only for programmer bugs** (impossible state, violated invariant) — never for control flow
  and never in response to bad external input. A malformed inbound packet disconnects that one client
  with a logged error; it must never take down a goroutine that matters or the process.
- **Recover at goroutine boundaries.** Every long-lived goroutine and every scheduled/ticked callback
  begins with a deferred recover-and-log, so one panic is contained and logged, not fatal.

## 5. Concurrency

This is a concurrent server. Concurrency is a first-class design concern, not an afterthought.

- **Decide ownership before you write the type.** For every piece of shared mutable state, document —
  in a comment on the struct — exactly what guards it: which mutex, or which single owning goroutine.
  If you can't state the ownership in one sentence, the design isn't ready.
- **Channels to transfer ownership and coordinate; mutexes to guard state.** Use whichever is simpler
  for the case. Don't force a channel where a short `sync.Mutex` around a map is clearer, and don't
  share a map across goroutines with no guard at all.
- **One goroutine writes a given connection.** Per connection: a read goroutine and a write goroutine
  draining a channel. Never write the same socket from two goroutines.
- **Context for lifecycle.** Long-running loops take `context.Context` and stop on cancel. Shutdown
  cancels the root context; every goroutine has a clear exit path. No leaks.
- **`go test -race ./...` must stay green.** It runs in CI from the first package. A data race is a
  bug, full stop.
- **No global mutable state without synchronization.** Prefer passing dependencies explicitly. A
  package-level registry is fine when it owns its own mutex and that's documented.
- Scheduled/periodic work uses a ticker goroutine or `time.AfterFunc`; keep the callback body short
  and non-blocking, offloading heavy or blocking work (DB, I/O) to its own goroutine.

## 6. Numeric & behavioral fidelity

Some outputs are exact contracts — packet fields, formula results, hash values, saved data. These must
match the specification bit-for-bit. Reconcile that with idiomatic Go like this:

- **Match integer widths deliberately** because overflow and truncation are part of the contract. Pick
  the concrete sized type the value needs (`int32`, `int64`, `uint16`, `byte`) and keep 32-bit
  wraparound where the algorithm depends on it. Don't silently widen a value that is defined to
  overflow.
- **Preserve operation order** in formulas — floating-point results depend on it. Reproduce the
  sequence of operations; keep `float64` where the spec computes in double precision.
- **Name reproduced algorithms for what they compute**, and document the algorithm inline. A function
  that must produce one specific legacy hash is `legacyStringHash` (with the formula in its doc
  comment), described by its behavior — never by its origin.
- Random-driven mechanics (drop, crit, enchant rates) must reproduce the specified distribution; use
  the project's RNG helper and match its semantics.

Idiomatic and exact are not in tension: `binary.Write` / explicit little-endian byte assembly produces
the exact wire bytes and is perfectly idiomatic Go.

## 7. Package & project layout

```
cmd/<binary>/          entry points; main wires dependencies together explicitly
internal/<area>/       all implementation (unimportable outside this module — deliberate)
```

- Organize packages **by responsibility/domain**, not by layer-type. `player`, `world`, `skill`,
  `item`, `geo`, `packet`, `db` — each a cohesive unit with a small surface.
- **`main` owns composition.** Construct and connect dependencies in `cmd/.../main.go` in explicit
  order. Do not rely on hidden package `init()` side effects to build the object graph; `init()` is for
  trivial, self-contained setup only.
- No cyclic imports — they mean two packages are really one, or a boundary is wrong. Fix the boundary.
- One concern per file; split large packages into focused files rather than one giant file.

## 8. Testing

- **Table-driven tests** with the standard `testing` package. No third-party test frameworks, no
  assertion DSLs, no elaborate fixtures unless a case truly needs them.
- Any non-trivial logic — parsers, formulas, encoders/decoders, money/trade paths, geometry,
  concurrency — ships with tests **in the same change**. Trivial accessors don't need tests.
- For exact-contract code, test against **known-good vectors** committed as data, not against numbers
  re-derived from the same formula you're testing.
- Prefer fast, hermetic unit tests. Anything needing external services is separated and not on the
  default `go test ./...` path.

## 9. Dependencies

Follow the ladder — stop at the first rung that works:

1. Does this need to exist at all? If speculative, don't write it.
2. Standard library fully covers the behavior without building a mini-framework? Use it.
   (`encoding/binary`, `encoding/xml`, `database/sql`, `crypto/*`, `container/heap`, `context`,
   `sync`, `math/rand/v2`, `time`.)
3. A dependency already in `go.mod`? Use it.
4. A small, established package clearly owns the subsystem better than we should? Add/use it.
5. Can it be a few lines of our own code? Write the few lines.

Keep the dependency set tiny, but do not hand-roll a subsystem when a standard Go package or a small,
well-scoped dependency is the simpler, safer implementation. For logging in this repo, use Logrus and
write only the glue needed to map existing config and route project-specific sinks.

## 10. Tooling gates (every change)

- `gofmt` clean (non-negotiable, no discussion).
- `go vet ./...` clean.
- `go build ./...` and `go test -race ./...` green.
- Exported identifiers have doc comments; comments explain **why**, not restate the code.

## 11. Comments & documentation

- Doc comments on exported types/functions, starting with the name, saying what it does and any
  contract (units, ranges, concurrency-safety, ownership).
- Inline comments earn their place: state an invariant, a non-obvious reason, or a contract the code
  can't express. Delete comments that narrate the obvious.
- Comments describe the Go code and the behavior it implements. Nothing else. No provenance notes, no
  cross-references to any external codebase, no "this mirrors X" — those rot and violate the reference
  rule in §0.

---

## 12. Anti-pattern quick reference

Seeing the left column in a diff means stop and rewrite as the right column.

| Smell (transliteration) | Idiomatic Go |
|---|---|
| `type XManager struct` with a `GetInstance()` | a package; or a struct named for what it owns, constructed in `main` |
| `GetFoo()` / `SetFoo(v)` on every field | exported field, or a single `Foo()` when access needs logic |
| `IThing` interface with one implementation | use the concrete type; add an interface when a consumer needs it |
| deep `Base` → `Mid` → `Leaf` embedding chain to fake subclassing | flat structs embedding only shared concerns; interfaces for polymorphism |
| everything `*T`, every field nullable | values for small/immutable data; nil only as documented absence |
| throw/catch or panic for expected failure | return `error`, wrap with `%w` |
| one giant class holding unrelated state | small structs per concern behind an aggregate |
| stringly-typed constants, magic ints | typed `iota` enums with `String()` |
| reflection-driven registration | explicit registration in `main` or a package var |
| shared map mutated from many goroutines, unguarded | documented mutex, or channel-owned state |
| `Util`/`Helper` grab-bag package | put the function next to the type it serves |

## 13. Worked example — the same behavior, two ways

❌ Transliterated (banned): fake inheritance, getters, a "manager" singleton, an interface with one impl.

```go
type IActor interface { GetHP() int; SetHP(v int) }

type BaseActor struct { hp int }
func (a *BaseActor) GetHP() int      { return a.hp }
func (a *BaseActor) SetHP(v int)     { a.hp = v }

type ActorManager struct { actors map[int]*BaseActor; mu sync.Mutex }
var instance *ActorManager
func GetInstance() *ActorManager { /* lazy singleton */ return instance }
func (m *ActorManager) GetActorById(id int) *BaseActor { /* ... */ }
```

✅ Idiomatic Go: concrete types, embedding for the shared concern, a real package registry with clear
ownership, no getters, interface only where a consumer needs it.

```go
// package combat
package combat

// Health is a mixin embedded by anything that can take damage.
type Health struct{ HP, MaxHP int32 }

func (h *Health) Damage(n int32) { h.HP = max(0, h.HP-n) }
func (h *Health) Dead() bool     { return h.HP <= 0 }
```

```go
// package npc
package npc

type NPC struct {
	ID   NpcID
	combat.Health          // embedded shared concern, not a base class
	pos  world.Point
}

// New returns a spawned NPC at full health.
func New(id NpcID, tmpl *Template, at world.Point) *NPC {
	return &NPC{ID: id, Health: combat.Health{HP: tmpl.MaxHP, MaxHP: tmpl.MaxHP}, pos: at}
}
```

```go
// package world — owns the live-object registry with explicit synchronization.
package world

type Registry struct {
	mu      sync.RWMutex          // guards objects
	objects map[ObjectID]Object
}

func NewRegistry() *Registry { return &Registry{objects: make(map[ObjectID]Object)} }

// Find returns the object and whether it was present.
func (r *Registry) Find(id ObjectID) (Object, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.objects[id]
	return o, ok
}
```

Same behavior (actors have HP, take damage, are looked up by id, concurrency-safe). Zero fake
inheritance, zero getters, zero singleton, explicit ownership. **This is the bar.**

---

## Definition of done (self-check before every PR)

- [ ] Reads like Go written from scratch — a Go engineer would not guess it was derived from anything.
- [ ] No banned name-shapes (§1), no getter/setter walls, no one-impl interfaces, no fake inheritance.
- [ ] Concurrency ownership documented; `go test -race ./...` green.
- [ ] Exact-contract outputs verified against committed known-good vectors.
- [ ] `gofmt` + `go vet` clean; exported items documented; comments explain why.
- [ ] No reference to any external codebase or language anywhere in the diff.
