# Skill formula oracle

`../formulas.golden` is the expected output of
`TestFormulasMatchReferenceProbe` (`../../oracle_test.go`). Each line is
`<formula> <inputs...> = <result>`, printed with Java's `String.valueOf`.

## What the probe is

`FormulasProbe.java` copies from aCis 409
`aCis_gameserver/java/net/sf/l2j/gameserver/skills/Formulas.java` the bonus
tables (lines 49-117, unchanged) and the arithmetic of `calcCastBreak`,
`calculateTimeBetweenAttacks`, `calcAtkSpd`, `calcHitMiss`, `getPosMul`,
`calcMagicSuccess`, `calcPhysicalAttackDamage`, `calcPhysicalSkillDamage`,
`calcBlowRate`, `calcBlowDamage`, `calcLethalRate`, `calcMagicDam` (both
overloads), `calcCubicSkillSuccess` (the magic attack modifier),
`calcManaDam`, `calcMagicAffected`, `calcShldUse`, `calcSkillSuccess`,
`calcRevivePower`, `calcSowSuccess`, `doorUnlock`, `chestUnlock` and
`calcHealAmount`; `L2Skill.getPower(Creature)`; `Cancel.calcCancelSuccess`
(`handler/skillhandlers/Cancel.java`); `EffectCancel.calcCancelSuccess` and
`EffectCancelDebuff.calcCancelSuccess` with their callers' `(int)` casts
(`skills/effects/`); and `Harvest.calcSuccess`
(`handler/skillhandlers/Harvest.java`).

The copies keep every arithmetic line, operand order, cast and clamp. The
substitutions are:

- Creature, skill and item lookups (`getPAtk`, `getPDef`, `calcStat(...)`,
  levels, `isBehind`, `getActiveWeaponItem`, the race and element
  modifiers, `HealSpsData.calculateHealSps`) are parameters. Defence
  parameters already hold any shield defence the caller adds on
  `ShieldDefense.SUCCESS`; skill power parameters already hold any SoulShot
  boost; the blow critical-position multiplier is passed already halved.
- `Rnd.get(n)` comparisons return the threshold instead of a boolean (the Go
  test checks the Go roll predicate around each threshold), or take the
  roll as a parameter (`castBreaks`, `shieldUse`); `Rnd.nextGaussian()` is a
  parameter.
- The magic failure branch is split out (`magicFailure`: 0 none, 1 kept
  damage for a non-player caster, 2 halved, 3 set to 1) and `magicDamage`
  takes its outcome; PvP multipliers are 1 outside PvP.
- Logging, system messages and the `Config.DEVELOPER` blocks are dropped.

`Math.pow` results (`magicSuccessRate`, `skillPower`) are compared within 4
ulp: the reference only promises 1 ulp for `Math.pow`, and Go's `math.Pow`
lands up to 4 ulp away on these inputs.

## Regenerating

From the repository root:

```
docker run --rm -v "$PWD/internal/gameserver/skill/formulas/testdata/oracle:/p" -w /p \
  eclipse-temurin:21-jdk-alpine java FormulasProbe.java \
  > internal/gameserver/skill/formulas/testdata/formulas.golden
```
