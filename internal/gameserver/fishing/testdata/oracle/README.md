# Fishing oracle

`../fishing.golden` is the expected output of `TestFishingMatchesReferenceProbe`
(`../../oracle_test.go`).

`run.sh` cuts these blocks **verbatim** out of the reference (only CRLF line ends are normalized)
and compiles them unchanged inside the harness `FishingCopy.java.tmpl`:

- `model/actor/container/player/FishingStance.java`: `getRandomFishType`, the lure-group
  `switch` and the look-period `if` chain of `start`, and the level spread of
  `getRandomFishLvl`,
- `handler/skillhandlers/FishingSkill.java`: the damage and expertise penalty lines of
  `useSkill`.

The harness only supplies what those blocks read: a settable roll in place of the random
generator, the fish's guts check time, and plain holders for the skill, rod and fisher
(`getPower`, crystal grade id, shot charged, Fishing Expertise level). The script fails if a
block is no longer found. `FishingProbe.java` drives them; the roll is forced, so nothing is
random.

Line formats (lure ids 6500-8599, every id the table names; lines are printed only for ids
that deviate from the default, the Go replay asserts the default for all others):

- `group <lure> <group>`: groups other than 1,
- `delay <lure> <ms>...`: look period for guts check times 1 3 7 99 1000 2000 3000 4000 5000
  (printed when any is not 0),
- `type <group> <lure> <check>:<type>...`: the fish type per roll 0-99 as runs (printed when
  any is not 1),
- `level <base> <check>:<level>...`: the fish level per roll 0-99 as runs,
- `damage <power> <rod grade id> <shot 0|1> <skill level> <expertise> <damage> <penalty>`.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/fishing/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
