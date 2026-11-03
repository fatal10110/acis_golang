# Stat pipeline oracle (summon, npc, player)

`run.sh` regenerates three golden files from one probe:

| golden | read by |
| --- | --- |
| `../golden_stats.json` | `TestGoldenSummonStatPipelineParity` (`../../formula_test.go`) |
| `../../../npc/testdata/golden_stats.json` | `TestGoldenHostileStatPipelineParity` (`npc/hostile_stats_test.go`) |
| `../../../player/testdata/golden_stats.json` | `TestGoldenPlayerStatPipelineParity` (`player/character_stats_golden_test.go`, `player/character_stats_golden_cases_test.go`) |

Each maps a case name to the bits of the raw, untruncated double (`Double.doubleToLongBits`,
equal to the unsigned `math.Float64bits` for these positive values) the stat reads after the
whole func chain, before the getters' `(int)` cast. `java StatPipelineProbe summon|npc|player`
prints one of them.

## What the probe is

`run.sh` compiles the reference stat calculator **unmodified** (only CRLF line ends are
normalized) and runs `StatPipelineProbe.java` on it:

- `skills/Calculator.java`: `addFunc` (equal orders keep insertion order), `removeOwner`
  (identity), `calc` (a `FuncSet` rebases the base later funcs read),
- `skills/basefuncs/`: `Func`, `FuncSet`, `FuncBaseMul`, `FuncBaseAdd`, `FuncAdd`, `FuncSub`,
  `FuncMul`, `FuncAddMul`, `FuncSubDiv`, `FuncEnchant`,
- `skills/funcs/`: `FuncPAtkMod`, `FuncMAtkMod`, `FuncPDefMod`, `FuncMDefMod`, `FuncMoveSpeed`,
  the order-10 builtins,
- `enums/skills/Stats.java`, `enums/Paperdoll.java`, and `enums/items/` `CrystalType`,
  `WeaponType`, `ItemType`.

These blocks are cut **verbatim** into the harness templates `stubs/*.tmpl`; the script fails if
one is no longer found:

- `model/actor/Creature.java`: `addStatFunc`, `addStatFuncs`, `removeStatsByOwner` (the lazily
  created per-stat calculator, the owner sweep that drops an emptied calculator) and the five
  `addStatFunc(Func…Mod/MoveSpeed.getInstance())` statements of `addFuncsToNewCharacter` for
  P.Atk, M.Atk, P.Def, M.Def and the run speed,
- `model/actor/status/CreatureStatus.java`: `calcStat` (the `size() == 0` shortcut and the
  `cantBeNegative` clamp), `getSTR`, `getDEX`, `getINT`, `getMEN`, `getLevelMod`,
- `skills/Formulas.java`: the attribute bonus tables with their static initializer, so
  `STR_BONUS`, `MEN_BONUS`, `INT_BONUS` and `DEX_BONUS` carry the reference's own `Math.pow`.

The harness only supplies what those blocks read: a template with the six base attributes, a
level, a `Player` with a worn-item map and a mage-class flag, items reduced to body part, crystal
grade and type, and empty stand-ins for the skill, the condition (every func is attached
without one) and the effect owner type.

## Cases

The cases mirror `goldenSummonScenarios`, `goldenHostileScenarios` and
`goldenPlayerScenarios`. All three share (each `addStatFuncs` call is one list):

- `order30_forward` / `order30_reverse`: `Add 1e16`, `Sub 1e16`, `Add 1` in that and in the
  reverse attach order, one call each, so equal-order funcs show their insertion order (P.Def
  for the summon and npc, P.Atk for the player),
- `set_rebase_mdef`: M.Def `Set 500` and `BaseMul 0.5` in one call,
- `attach_detach_before` / `_during` / `_after`: P.Atk before, with an owner's `Add 7` and
  `Mul 1.25`, and after `removeStatsByOwner` of that owner.

The summon is a level-44 actor with STR 40, MEN 20, P.Atk 100, P.Def 50, M.Def 40; the npc a
level-20 monster with STR 40, CON 21, DEX 30, INT 20, WIT 43, MEN 20 and the same defences; the
player STR 40, CON 43, DEX 30, INT 21, WIT 11, MEN 25 with P.Atk 5, P.Def 50, M.Def 60, M.Atk
20 and run speed 100, at level 1 unless a case says otherwise. Only the player has more:

- `mixed_pdef` / `mixed_matk` / `mixed_runspeed`: `BaseAdd`, `Mul`, `Add`, `AddMul`, `SubDiv`
  across P.Def, M.Atk and the run speed in one call. The run speed is read the way the player's
  move speed getter reads it: a float base, `(float)` of the stat, widened back (the swamp,
  weight and armor-grade maluses are neutral),
- `builtin_*`: each builtin on its own at level 40 with nothing worn,
- `penalty_<outfit>_pdef` / `_mdef`: a level-40 fighter or mage wearing the outfit's pieces
  (the armor penalties of P.Def, the jewelry penalties of M.Def, the mage chest and legs
  values, the full-body suit, and formal wear, which is a chest piece but not a full-body one).
  A full-body suit together with a legs piece is not a case: equipping either removes the other,
  so no inventory holds both,
- `enchant_patk_<grade>_<weapon>_e<level>`, `enchant_matk_<grade>_e<level>`,
  `enchant_pdef_e<level>`, `enchant_mdef_e<level>`, `enchant_patk_armor_e<level>`: `FuncEnchant`
  by crystal grade, weapon type and enchant level (the levels around the over-3 split too).

Keep the two sides in step when a case changes: each Go test compares both directions (a
missing or an extra case fails).

Not covered: a player's hennas (`FuncHenna`; the cases read the template attributes, which is
what a player with no hennas computes), and `removeStatsByOwner(null)`, which in the reference
would strip the builtins while the Go `RemoveStatsByOwner` ignores a zero owner (no caller
passes one).

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/model/actor/summon/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`. The output replaces the three golden files.
