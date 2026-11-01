# Fly geodata oracle

`../fly.golden` is the expected output of `TestFlyMatchesReferenceProbe` in
`../../fly_oracle_test.go`. (#4341)

The committed geo probe (`cmd/geoprobe`) needs licensed real geodata. This one runs the
reference's fly queries on a synthetic block grid instead.

`run.sh` compiles these **unmodified** (only CRLF line ends are normalized):

- `geoengine/geodata/`: `ABlock`, `BlockFlat`, `BlockComplex`, `BlockMultilayer`, `BlockNull`,
  `IGeoObject`, `GeoStructure`,
- `enums/`: `GeoType`, `MoveDirectionType`,

and cuts these blocks **verbatim** into the harness `GeoCopy.java.tmpl`:

- `geoengine/GeoEngine.java`: `getGeoX`, `getGeoY`, `getBlock`, `getHeightNearest(geoX, geoY,
  worldZ)`, `canFly` and `getValidFlyLocation` (the `debug` branches stay, the probe passes
  `null`),
- `model/World.java`: the geodata window constants (`TILE_*`, `WORLD_*`) and both `isOutOfWorld`
  overloads, into `stubs/World.java.tmpl`.

The engine itself cannot be constructed (its constructor reads region files and exits on a
failure), so `GeoCopy` only holds the `_blocks` grid the cut methods read. `stubs/` holds the debug
packet and location value types, reduced to what the cut methods call. The script fails if a block
is no longer found.

## Synthetic geodata

`FlyProbe.java` fills the grid by grid block coordinates (a block is 8x8 cells of 16 units; the
window's first block is `(0,0)` at the world origin); every other block is `BlockNull`.
`fly_oracle_test.go` builds the same blocks in a Go engine. Heights are multiples of 8, which the
cell format stores exactly.

| block | content |
| --- | --- |
| `(0,0)` | complex: floor 0 on cell columns 0-3, floor 200 on columns 4-7 |
| `(1,0)` | flat, floor 50 |
| `(2,0)` | complex: floor 0, with a trench (floor -200) on columns 3-4 |
| `(0,2)` | complex ramp: floor `8*(x+y)` |
| `(1,2)` | multilayer: floor 0 plus a ceiling at 152 on cells whose `x+y` is even; column 7 has layers 0, 80 and 400 |
| `(last,0)`, `(0,last)`, `(last,last)` | flat, floor 30: the other three corners of the grid |

## Line format

One line per query, tab separated:

```
fly <name> <ox oy oz> <oheight> <tx ty tz> <canFly> <stop x y z>
```

`canFly` is `canFly(..., null)`; the stop is `getValidFlyLocation(..., null)`. The queries cover
the cases of `fly_test.go` (traced by hand before this probe), diagonals in every direction, steep
and shallow slopes, vertical and horizontal lines, cell borders, multilayer ceilings and middle
layers, the corridor height truncated to an int (fractional, negative, huge and not-a-number),
lines long enough to wrap the 32-bit squared-delta sum, and walks that leave the geodata grid on
each side and corner.

The reference `canFly` has no bound check on its origin: an origin outside the grid indexes
outside `_blocks` (an exception), so the probe never asks for one; only the target is gated by
`isOutOfWorld`.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/geo/engine/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
