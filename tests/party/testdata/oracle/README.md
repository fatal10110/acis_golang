# Raid point record oracle

`../raid_points.golden` is the expected output of `TestBossRecordMatchesReference`
(`tests/party/raid_points_test.go`). It is the output of `RaidPointProbe.java`
in this directory, a probe of the reference raid point record.

## What the probe is

`RaidPointProbe.java` copies, from aCis 409:

- `aCis_gameserver/java/net/sf/l2j/gameserver/data/manager/RaidPointManager.java`:
  the `_entries` map, the constructor's row load (`computeIfAbsent` then
  `put`), `getList`, `addPoints` (`computeIfAbsent` then `merge`),
  `getPointsByOwnerId` and `calculateRanking`;
- `aCis_gameserver/java/net/sf/l2j/gameserver/network/clientpackets/RequestGetBossRecord.java`:
  `runImpl`;
- `aCis_gameserver/java/net/sf/l2j/gameserver/network/serverpackets/ExGetBossRecord.java`:
  `writeImpl`, written little-endian into a buffer.

The copies are unchanged except that the database calls are gone: the
constructor's `SELECT` becomes calls to `load`, and `addPoints` no longer
issues its `REPLACE`. Each output line is a player number and the
ExGetBossRecord body (extended opcode included) as hex.

The order a record lists its bosses in is the iteration order of the
reference's `HashMap`: a restored boss is appended to its bucket's chain by
`put`, an added one prepended by `merge`, and the two grow the table at
different moments. The scenario exercises both, a table growing past 12
bosses, and nine bosses in one bucket growing a 16-bucket table.

## Scenario

Player 1 has six stored rows. Points are then added to player 1 (seven new
bosses and one existing), to player 2 (nine bosses 16 apart), to player 3 (a
zero amount) and to player 4 (a negative amount, which adds nothing). Every
player then asks for its record. The `restart` lines are the same requests
after every held total is written back and read again in the table's key
order, as a restart does.

Totals are distinct, so no two players tie: the reference breaks ties by its
hash map order, the Go ranking by object id.

## Regenerating

From the repository root:

```bash
docker run --rm -v "$PWD/tests/party/testdata/oracle:/p" -w /p \
  eclipse-temurin:21-jdk-alpine java RaidPointProbe.java \
  > tests/party/testdata/raid_points.golden
```

A change to the scenario needs the same change on both sides.
