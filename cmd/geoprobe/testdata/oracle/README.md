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

`queries=1000 agreement=98.10%` (see `report.txt`). 19 disagreements across three categories, each
matching an already-open, milestone-assigned gap rather than a new defect:

- **6 `los` mismatches** — line-of-sight disagreements on blocked multilayer edges. Matches #1830
  (`Multilayer.Above` selects the highest qualifying layer; Java selects the lowest), which this
  package's `CanSee` blocked-edge path depends on for `Above` resolution.
- **2 `path` mismatches** — Go finds a path (`found=true`) where Java's `GeoEngine.findPath` rejects
  outright (`found=false`), both with an endpoint Z far from its nearest geodata layer. Matches #1764
  (`findPath` missing the `hasGeoPos` and `|gtz-tz|>500` reference pre-gates).
- **11 `validlocation` mismatches** — large Z/coordinate divergences once movement crosses a
  multilayer cell. Matches #1832 (post-#804 `PathFinder`/`CanMove` layer-selection reconciliation is
  still open), which the same movement-resolution path (`getHeightNearest`/`getNsweNearest`) as
  `ValidLocation` depends on.

No `height` or `canmove` disagreements: those two categories are already at 100% agreement against
this oracle.

Re-run this capture once #1764, #1830, and #1832 land, to confirm they close the corresponding
disagreements.
