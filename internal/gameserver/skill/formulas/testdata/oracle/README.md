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
`calcRevivePower`, `calcSowSuccess`, `doorUnlock` (both branches),
`chestUnlock`, `calcHealAmount`, `calcCrit(double)`, `calcMCrit`,
`calcSkillReflect`, `calcLethalHit` (its tier order and draws),
`calcSkillVulnerability`, `calcSkillStatModifier`, `getLevelModifier`,
`getMatkModifier`, `calcElementalPhysicalAttackModifier`,
`calcElementalSkillModifier`, `calcSkillMastery`, `calcFallDam`,
`calculateKarmaLost` and `calculateKarmaGain`;
`HealSpsData.calculateHealSps` (`data/xml/HealSpsData.java`) over a copy
of the `aCis_datapack/data/xml/healSps.xml` entries in file order (absent
attributes are 0, as the record defaults them);
`L2Skill.getPower(Creature)`; `Cancel.calcCancelSuccess`
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
- `Rnd.get(n)` comparisons return the threshold instead of a boolean, or
  take the roll as a parameter (`castBreaks`, `shieldUse`, `crit`, `mcrit`,
  `skillReflect`, `doorUnlockSpecial`); `lethalHit` takes its two draws in
  order and prints `<tier>/<draws taken>`. For each threshold row the Go
  test replays the matching roll predicate (`Missed`, `MagicSucceeds`,
  `BlowSucceeds`, `lethalSucceeds`, `SkillSucceeds`, `CancelSucceeds`,
  `DoorUnlockSucceeds`) at the draws next to the threshold and at both ends
  of the draw range, against the reference comparison.
  `Rnd.nextGaussian()` is a parameter.
- `skillReflect` reports only the reflect bit; the counter bit is a target
  stat lookup outside the formula.
- The magic failure branch is split out (`magicFailure`: 0 none, 1 kept
  damage for a non-player caster, 2 halved, 3 set to 1) and `magicDamage`
  takes its outcome; PvP multipliers are 1 outside PvP.
- The creature-level modifiers (`skillStatModifier`, `skillVulnerability`,
  `elementalSkillModifier`, `levelModifier`, `matkModifier`,
  `elementalPhysicalAttackModifier`) are replayed against the live-actor
  resolvers by `TestModifiersMatchReferenceProbe`
  (`model/actor/creature/formula_oracle_test.go`). Their stat lookups
  model the target's and attacker's stat funcs as one func per stat: the
  init-seeded `*_VULN` and `<element>_RES` stats multiply their seed (the
  `*_VULN` multipliers are the fixed `VULN_MUL`, distinct per stat so the
  result shows which stat applied), and `<element>_POWER` adds to its seed
  0 before `getAttackElementValue`'s `(int)` cast.
- `skillMasteryRate` returns the value the `Rnd.get(100.)` draw must stay
  under; `fallDamageBase` returns the `FALL` stat's seed, and the
  `ENABLE_FALLING_DAMAGE` and negative-height guards stay with the caller.
  `karmaLost` takes the level's `karmaModifier`.
- `healSpsEntry` rows print the copied heal table (the Go test builds its
  table from them); `healSps` rows are lookups over it, and `healAmountSps`
  rows run `calcHealAmount` with the correction the lookup returns for the
  row's skill id, level and magic level. `TestHealSpsMatchesReferenceProbe`
  (`data/xml`, needs the datapack) replays the `healSps` rows on the
  shipped file as loaded.
- The rows after "The inputs the live-actor resolver tests hand to the
  formulas" pin the values the player and summon resolver tests compare
  against.
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
