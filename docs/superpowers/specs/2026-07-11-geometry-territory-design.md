# Geometry package & unified Territory shape

## Goal

`internal/gameserver/model/zone` (shape forms: Cuboid, Cylinder, Polygon) and
`internal/gameserver/model/spawn` (Territory: name + Z range + point list, no
containment/area/intersection methods) independently reimplement 2D/3D shape
math. Unify both onto one composable primitive so a shape is built once and
reused everywhere a region needs to be described.

## Package: `internal/gameserver/model/geometry`

```go
type Shape interface {
    Contains(x, y int) bool
    Area() float64
    IntersectsRect(x1, x2, y1, y2 int) bool
    Intersects(other Shape) bool
}
```

Concrete primitives: `Rectangle`, `Triangle`, `Polygon`, `Circle`.

Rectangle/Triangle/Polygon share a vertex-list representation and route
pairwise intersection through one generic polygon-vs-polygon
separating-axis-theorem (SAT) test — avoids 10 hand-written pairwise
combinations for 4 shape kinds. Circle gets two special-cased tests:
circle-vs-circle (center distance vs. radius sum) and circle-vs-polygon
(point-to-edge distance).

## Territory (3D composite)

```go
type Territory struct {
    MinZ, MaxZ int
    Shapes     []Shape
}

func NewTerritory(minZ, maxZ int, shapes ...Shape) (*Territory, error)
func (t *Territory) Contains(x, y, z int) bool
func (t *Territory) IntersectsRect(x1, x2, y1, y2 int) bool
func (t *Territory) LowZ() int
func (t *Territory) HighZ() int
func (t *Territory) Area() float64
func (t *Territory) Intersects(o *Territory) bool
```

`Contains` = z within `[MinZ, MaxZ]` AND any shape contains `(x, y)` — union
semantics across shapes, matching existing multi-region containment
behavior. `Area` sums per-shape area (overlap between shapes in the same
`Territory` is not deduplicated — not needed by any current caller).

`*Territory`'s method set (`Contains`, `IntersectsRect`, `LowZ`, `HighZ`)
structurally satisfies `zone.Form` — no change to the `zone.Form` interface
itself.

## Migration: zone package

Replace the `NewCuboid` / `NewCylinder` / `NewPolygon` constructors so each
returns a `*geometry.Territory` wrapping the matching primitive:

- `NewCuboid(x1,x2,y1,y2,z1,z2)` → `Territory{Shapes: [Rectangle], MinZ: z1, MaxZ: z2}`
- `NewCylinder(x,y,z1,z2,rad)` → `Territory{Shapes: [Circle], MinZ: z1, MaxZ: z2}`
- `NewPolygon(nodes,z1,z2)` → `Territory{Shapes: [Polygon], MinZ: z1, MaxZ: z2}`

The concrete `zone.Cuboid` / `zone.Cylinder` / `zone.Polygon` types are
deleted; call sites (the zone XML loader) construct `*geometry.Territory`
directly through the updated constructors.

## Migration: spawn package

```go
type Territory struct {
    Name string
    *geometry.Territory
}
```

The existing `Nodes []Node` point list converts to a `geometry.Polygon` at
construction time. This gives spawn's `Territory` working `Contains` /
`Area` / `Intersects` for the first time — it currently has none.

Out of scope for this migration: geodata-aware random-point-in-shape spawn
placement (tracked separately, needs the geo engine).

## Explicitly deferred (future improvement, not built now)

`Visualize` (2D outline) and `Visualize3D` (colored-face 3D render) methods
on `Territory`, plus the machinery to actually show them to a GM in-client.
Blocked on two prerequisites that don't exist yet in this codebase and are
out of scope for the geometry work itself:

- Admin/GM command dispatch — `internal/gameserver/handler/admin/` is
  currently an empty stub; there is no `//command` routing to hang a new
  debug command off of.
- A drawing/debug packet type — no packet exists today that sends arbitrary
  point/line/face data to a client for visualization.

Tracked as a follow-up issue; not blocking the core geometry/Territory work.

## Testing

Table-driven tests per primitive (`Contains`/`Area`/`Intersects` against
known geometric fixtures: inside/outside/edge cases). `Territory` composite
tests: union containment across multiple shapes, Z-range boundary
inclusion/exclusion, cross-shape-kind `Intersects` (Rectangle vs Circle,
Polygon vs Circle, etc). No mocks — pure math, no external dependencies.

## Milestone note

This is infrastructure supporting `zone`/`spawn` (M4 — World, spawns,
movement), not a game feature of M12 ("Territory: sieges, castles, clan
halls, manor" — castle/clan-hall siege ownership, an unrelated use of the
word "Territory"). Filed without a milestone to avoid the naming collision.
