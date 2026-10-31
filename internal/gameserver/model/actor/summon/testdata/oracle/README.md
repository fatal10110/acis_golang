# Summon stat pipeline oracle

`../golden_stats.json` is the expected output of `TestGoldenSummonStatPipelineParity`
(`../../formula_test.go`): case name to the bits of the raw, untruncated double
(`Double.doubleToLongBits`, equal to the unsigned `math.Float64bits` for these positive values)
the stat reads after the whole func chain, before the getters' `(int)` cast.

## What the probe is

`run.sh` compiles the reference stat calculator **unmodified** (only CRLF line ends are
normalized) and runs `StatPipelineProbe.java` on it:

- `skills/Calculator.java`: `addFunc` (equal orders keep insertion order), `removeOwner`
  (identity), `calc` (a `FuncSet` rebases the base later funcs read),
- `skills/basefuncs/`: `Func`, `FuncSet`, `FuncBaseMul`, `FuncAdd`, `FuncSub`, `FuncMul`,
- `skills/funcs/`: `FuncPAtkMod`, `FuncPDefMod`, `FuncMDefMod`, the order-10 builtins,
- `enums/skills/Stats.java` and `enums/Paperdoll.java`.

These blocks are cut **verbatim** into the harness templates `stubs/*.tmpl`; the script fails if
one is no longer found:

- `model/actor/Creature.java`: `addStatFunc`, `addStatFuncs`, `removeStatsByOwner` (the lazily
  created per-stat calculator, the owner sweep that drops an emptied calculator) and the three
  `addStatFunc(Func…Mod.getInstance())` statements of `addFuncsToNewCharacter` for P.Atk, P.Def
  and M.Def,
- `model/actor/status/CreatureStatus.java`: `calcStat` (the `size() == 0` shortcut and the
  `cantBeNegative` clamp), `getSTR`, `getMEN`, `getLevelMod`,
- `skills/Formulas.java`: the attribute bonus tables with their static initializer, so
  `STR_BONUS` and `MEN_BONUS` carry the reference's own `Math.pow`.

The harness only supplies what those blocks read: a template with a base STR and MEN, a level,
and empty stand-ins for the skill, the condition (every func is attached without one), the effect
owner type, and the player's inventory (the armor and jewelry penalties of the defence funcs
apply to a `Player` only, and the summon cases never build one).

The cases mirror `goldenSummonScenarios` in `formula_test.go`: a level-44 summon with STR 40,
MEN 20, P.Atk 100, P.Def 50, M.Def 40, and these func attachments (each `addStatFuncs` call is
one list):

- `order30_forward` / `order30_reverse`: P.Def `Add 1e16`, `Sub 1e16`, `Add 1` in that and in
  the reverse attach order, one call each, so equal-order funcs show their insertion order,
- `set_rebase_mdef`: M.Def `Set 500` and `BaseMul 0.5` in one call,
- `attach_detach_before` / `_during` / `_after`: P.Atk before, with an owner's `Add 7` and
  `Mul 1.25`, and after `removeStatsByOwner` of that owner.

Keep the two sides in step when a case changes: the Go test compares both directions (a missing
or an extra case fails).

Not covered: a `Player`'s armor and jewelry penalties (the Go player fixture keeps its own
capture), and `removeStatsByOwner(null)`, which in the reference would strip the builtins while
the Go `RemoveStatsByOwner` ignores a zero owner (no caller passes one).

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/model/actor/summon/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`. The output replaces `../golden_stats.json`.
