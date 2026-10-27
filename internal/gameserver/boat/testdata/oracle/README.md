# Boat dock oracle

`../dock.golden` is the expected output of `TestDocksMatchReferenceProbe`
(`../../dock_oracle_test.go`).

`run.sh` compiles these reference classes **unmodified** from `aCis_gameserver/java/net/sf/l2j/`
(`javac -sourcepath stub:/ref`):

- `gameserver/enums/boats/BoatDock.java` (the dock table, `getBoardingPoint`,
  `getAdjustedBoardingPoint`, both coordinate conversions),
- `commons/geometry/basic/Line2D.java`, `gameserver/model/location/{Point2D,Location}.java`,
  `commons/random/Rnd.java`.

Stand-ins (`stub/`): `BoatLocation` (the tie-up point, only its position is read) and
`StatSet` (a `Location` constructor argument never used here).

Line formats (`DockProbe.java`; the walks come from a fixed xorshift, so they are stable):

- `dock <NAME> at <x y z> oust <x y z> entrance <x1 y1 x2 y2> exit <...> axis <...> exclusive <bool>`,
- `board <NAME> <inBoat 0|1> <ox oy dx dy> => <entrance point> | <exit point> | <adjusted entrance point>`
  (`none`: the walk does not cross the line); includes the Rune walks the boat passenger
  tests use,
- `convert <NAME> <deck x y> <world x y> => <deck to world> | <world to deck>`.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/boat/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
