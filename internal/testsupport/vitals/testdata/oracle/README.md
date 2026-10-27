# Vitals oracle

`../vitals.golden` is the expected output behind `internal/testsupport/vitals`, whose lookups
the door and pet behavior tests (`tests/skills/door_hp_test.go`,
`tests/pets/summon_revive_test.go`, ...) assert against.

`run.sh` cuts these blocks **verbatim** out of the reference (only CRLF line ends are normalized)
and compiles them unchanged inside the harness `VitalsCopy.java.tmpl`:

- `model/actor/instance/Door.java`: `getDamage`,
- `model/actor/status/CreatureStatus.java`: `getHpRatio`, both `setHp` (the cap at the maximum and
  the start/stop of the regeneration task), `getRegenHp`, `doRegeneration`,
- `skills/Formulas.java`: `getRegeneratePeriod`, its period constant, and the attribute bonus
  tables with their static initializer,
- `model/actor/Creature.java`: the HP statement of `doRevive`,
- `skills/funcs/FuncMaxHpMul.java`: the maximum HP function (the constitution bonus),
- `model/actor/status/PetStatus.java`: `getMaxHp`,
- `model/actor/instance/Pet.java`: `restoreExp`,
- `Config.java`: the `RESPAWN_RESTORE_HP`, `HP_REGEN_MULTIPLIER` and `RAID_HP_REGEN_MULTIPLIER`
  assignments, and `CreatureTemplate.java`: the `hpRegen` default.

The harness only supplies what those blocks read: the configuration holders answer the default
the cut line gives (the properties files do not override them in the test boot), a status with
a maximum HP, current HP and a regeneration flag, and the MAX_HP stat function. A door carries no
stat functions (its own `addFuncsToNewCharacter` adds none), so its regeneration is the base
value. The script fails if a block is no longer found.

Line formats (`VitalsProbe.java`):

- `damage <maxHP> <hp>:<stage>...`: the door damage stage from that remaining HP (a half-point
  grid from 0 to the maximum) until the next run,
- `period <door|other> <ms>`,
- `regen <HP regen multiplier> <start HP> <hp>:<running 0|1>...`: the state after each
  regeneration tick of a door of 600 HP that has just dropped to the start HP, until the task
  stops,
- `revive <maxHP> <HP>`: HP right after a plain revive,
- `petmaxhp <base> <CON> <HP>` (a summon's maximum HP runs the same MAX_HP function),
- `petexp <before death> <exp now> <power> <exp added>`.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/testsupport/vitals/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
