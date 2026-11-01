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

- `approve <start/price>[+...] <warehouse> <treasury> => treasury <n> next <lists|empty> sent
  <messages|-> mode <mode> delay <ms>`: a modifiable manor approved at 06:00 (Gludio owned by a
  clan). The next period sells 10 Dark Coda seeds at 5 and buys the listed crops (alternating Dark
  Coda and Red Coda), so it costs 3050 with `100/30`; the owner's warehouse holds `<warehouse>`
  other items, plus the Dark Coda mature item when it ends in `m`. `sent` is what the clan leader
  was told.

The approval's warehouse check counts a slot for each crop whose mature item
`getItemsByItemId(...) == null`; that method returns a list, empty when nothing matches, never
null, so the count is always 0, `validateCapacity(0)` passes, and the approval never charges the
treasury, empties the next period or tells the leader. The harness's `getItemsByItemId` returns a
list the same way; the `approve` lines pin that outcome for full, overfull and empty warehouses and
for short and ample treasuries (#4361).

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/castlemanor/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
