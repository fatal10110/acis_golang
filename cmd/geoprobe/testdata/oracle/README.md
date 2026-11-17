# geoprobe Java oracle capture

Fixture answering #1970: a Java `GeoEngine` oracle dump for `cmd/geoprobe -expected-dump`, captured
against a real Interlude L2OFF geodata set instead of a Go-generated or synthetic one.

## Contents

- `query_ids.txt` — 1000 query IDs, `go run ./cmd/geoprobe -queries 1000 -seed 1` against the same
  geodata (IDs only; a fresh run reproduces the identical sample from the seed).
- `java_oracle.dump` — the same queries evaluated by the Java `GeoEngine`/`PathFinder`, in
  `datadiff`'s dump format. This is the file to pass as `-expected-dump`.
- `GeoProbeOracle.java` — the standalone capture tool: reads a query-ID file, evaluates each against
  `net.sf.l2j.gameserver.geoengine.GeoEngine`, and writes a dump.
- `report.txt` — `go run ./cmd/geoprobe -expected-dump java_oracle.dump` output at capture time.

## Provenance

- Java reference revision: `aCis_gameserver` commit `0d32d37adc94128a820ce51df3bce5d7655e25e2`.
- Geodata: a real Interlude L2OFF set (139 loaded region files, matching every region enabled in
  `aCis_gameserver/config/geoengine.properties`). Not committed here — it's a large licensed asset;
  point `-geodata` at your own copy to reproduce.
- Pathfinding weights: `aCis_gameserver/config/geoengine.properties` defaults (also Go's
  `pathfind.DefaultOptions()`): MoveWeight=10, MoveWeightDiag=14, ObstacleWeight=30,
  HeuristicWeight=12, MaxIterations=10000.

## Reproducing the capture

```bash
# 1. Generate a reproducible query sample (IDs only matter; this repo's copy already has them).
go run ./cmd/geoprobe -geodata <geodata-dir> -geotype L2OFF -queries 1000 -seed 1 -dump /tmp/q.dump
cut -f1 /tmp/q.dump > cmd/geoprobe/testdata/oracle/query_ids.txt

# 2. Compile the capture tool against a JDK-21-built aCis_gameserver checkout.
(cd aCis_gameserver && JAVA_HOME=<jdk21> ant compile)
javac -cp aCis_gameserver/build/classes -d /tmp/oracle cmd/geoprobe/testdata/oracle/GeoProbeOracle.java

# 3. Evaluate the same queries against the Java GeoEngine.
java -cp /tmp/oracle:aCis_gameserver/build/classes GeoProbeOracle \
  <geodata-dir>/ cmd/geoprobe/testdata/oracle/query_ids.txt cmd/geoprobe/testdata/oracle/java_oracle.dump

# 4. Compare.
go run ./cmd/geoprobe -geodata <geodata-dir> -geotype L2OFF \
  -expected-dump cmd/geoprobe/testdata/oracle/java_oracle.dump | tee cmd/geoprobe/testdata/oracle/report.txt
```

The tool sets `Config.GEODATA_PATH`/pathfinding weights directly to the same values above rather than
loading `geoengine.properties`, so it needs no gameserver boot beyond `GeoEngine.getInstance()`.

`GeoProbeOracle`'s `path` case reads `cost` off the last returned path node's `getCostG()` — the
reference `PathFinder`'s own accumulated A* cost — rather than recomputing a cost formula, so a `cost`
mismatch reflects the reference engine's real search cost, not an approximation.

## Result

`queries=1000 agreement=100.00%` (see `report.txt`), re-run after the multilayer layer-order, door-lift
and `findPath` pre-gate fixes. The capture itself first scored 98.10% (6 `los`, 2 `path` and 11
`validlocation` disagreements, tracked then as #1830, #1764 and #1832).

`TestOracleDumpReplaysAgainstShippedGeodata` (`oracle_replay_test.go`) replays this dump against the
shipped geodata on every local run (`ACIS_REQUIRE_DATAPACK=1`; CI has no geodata and skips it).

## Not covered by this capture

All 200 `path` queries have `found=false` (random far-apart endpoints), and no query uses a non-zero eye
height, a closed-door overlay or a midpoint/non-monotonic cell. Extending the capture for those needs a
JDK 21 build of the reference engine (tracked in #4789 and #4793).
