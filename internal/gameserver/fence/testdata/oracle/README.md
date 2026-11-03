# Fence sizing oracle

`../fence.golden` is the expected output of `TestSizingMatchesReferenceProbe`
(`../../oracle_test.go`).

`run.sh` cuts these blocks **verbatim** out of
`aCis_gameserver/java/net/sf/l2j/gameserver/data/manager/FenceManager.java` (only the CRLF line
ends are normalized) and compiles them unchanged inside the harness
`FenceCopy.java.tmpl`:

- the private `FenceSize` enum (offset and geodata size per 100-unit step),
- `getFenceSize(int)`,
- the alignment statement `x = x & 0xFFFFFFF0 + fsx._offset;` (the precedence quirk: the
  offset is added to the mask),
- the inner-description loop of `addFence` (the lines between `// create inner description`
  and `calculateGeoObject`), with `type`, `sx`, `sy` as parameters.

The script fails if a block is no longer found. `FenceProbe.java` drives them.

Line formats:

- `size <n> <offset> <cells>|none`: from fence length `n` up to the next `size` line the
  reference answers that placement (`none`: refused),
- `align <value> <offset> <aligned>`: the statement applied to `value` for that offset,
- `outline <type> <cellsX> <cellsY> <row>...`: the inner description, one `#`/`.` string per x cell.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/fence/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
