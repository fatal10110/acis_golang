# Castle manor oracle

`../manor.golden` is the expected output of the `*MatchesReferenceProbe` tests in
`../../manor_oracle_test.go`.

`run.sh` cuts these blocks **verbatim** out of the reference (only CRLF line ends are
normalized) and compiles them unchanged inside the harness `ManorCopy.java.tmpl`:

- `data/manager/CastleManorManager.java`: the constructor's mode selection (the lines between its
  "Set mode and start timer" and "Schedule mode change" comments), `scheduleModeChange`,
  `changeMode` (all three modes), `getManorCost`, `getSeed`, `getSeedByCrop`, `getSeedProduction`
  and `getCropProcure`,
- `model/itemcontainer/ClanWarehouse.java`: `validateCapacity`,
- `model/manor/SeedProduction.java` and `CropProcure.java`: the whole files,
- `Config.java`: the `MANOR_*` assignments and `WAREHOUSE_SLOTS_CLAN` (the properties files do not
  override them in the test boot; the shipped values are refresh 20:00, approve 06:00, maintenance
  6 minutes).

The harness supplies what those blocks read: a clock the probe sets (a `Calendar` and a `System`
that answer it, in UTC), the scheduler (it records the delay it is given), `Rnd` (answers the roll
the probe sets), the castles, clans, warehouse and seeds, and `Castle.editTreasury` /
`riseSeedIncome` without their taxes, caps and stores. The script fails if a block is no longer
found.

Line formats (`ManorProbe.java`):

- `mode <hour> <minute>:<mode>...`: the mode a manor starts in, by minute of that hour, as runs,
- `next <mode> <hh:mm:ss.mmm> <ms>`: milliseconds from that time until a manor in `mode` changes
  it,
- `cost <seeds>|<crops> => <cost>`: `getManorCost`; seeds are `<reference price>x<start amount>`
  (`none ` for a seed the table does not hold), crops `<price>x<start amount>`, each a 32-bit
  product,
- `rollover <start/left/price>[+...] <treasury> <roll> => warehouse <item:count,...|-> income <n>
  next <empty|lists> running <seeds> | <crops> mode <mode> delay <ms>`: an approved manor changing
  mode at 20:00 (Gludio owned by a clan). The running period buys the listed crops (alternating
  Dark Coda and Red Coda); the next period sells 10 Dark Coda seeds and buys 100 Dark Coda at 30,
  so it costs 3050 once running.

Not covered: `changeMode` from MODIFIABLE (the approval), which takes the next period's cost from
the castle's treasury when the owner's warehouse lacks room for the crops' mature items and empties
the next period when the treasury cannot pay it; `CastleManorManager` is not yet modeled that way
(tracked in #4361).

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/castlemanor/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
