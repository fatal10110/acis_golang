# Attribute bonus table oracle

`../statbonus.golden` is the expected output of
`TestBonusTablesMatchReferenceProbe` (`../../oracle_test.go`): one row per
attribute value 0..99 with STR, CON, DEX, INT, WIT and MEN bonuses and the
base evasion/accuracy term.

`StatBonusProbe.java` copies, unchanged, aCis 409
`aCis_gameserver/java/net/sf/l2j/gameserver/skills/Formulas.java` lines
49-117 (`MAX_STAT_VALUE`, the `*_COMPUTE` constants, the bonus arrays and the
static initializer). Only the `main` that prints the rows is added.

## Regenerating

From the repository root:

```
docker run --rm -v "$PWD/internal/gameserver/skill/statbonus/testdata/oracle:/p" -w /p \
  eclipse-temurin:21-jdk-alpine java StatBonusProbe.java \
  > internal/gameserver/skill/statbonus/testdata/statbonus.golden
```
