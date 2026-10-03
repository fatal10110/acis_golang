# Olympiad calendar oracle

`../timeline.golden` is the expected trace of `TestOlympiadCalendarTimeline`
(`tests/olympiad/timeline_test.go`). It is the output of `OlyProbe.java` in
this directory, a probe of the reference calendar.

## What the probe is

`OlyProbe.java` copies these methods of aCis 409
`aCis_gameserver/java/net/sf/l2j/gameserver/model/olympiad/Olympiad.java`:
`reschedule`, `getDelay`, `executeTask`, `endOlympiad`, `checkPendingGames`,
`schedulePeriodEnd`, `revalidatePeriod`, `init`, `startCompetition`,
`setNextNoblePointsUpdate`, `setNewOlympiadEnd`, `startNewCycle` and
`deleteNobles`. The copies are unchanged except:

- `System.currentTimeMillis()` and `Calendar.getInstance()` read a virtual
  clock.
- `ThreadPool.schedule` records the pending delay. The driver then steps the
  clock to the soonest delay and runs every step due at that instant.
- Database, broadcast and game manager calls print trace lines (`DB ...`,
  `ANN ...`). No match ever runs (`isBattleStarted()` is false).
- The configuration is the shipped default: `OlyStartTime = 18`,
  `OlyMin = 00`, `OlyCPeriod = 21600000`, `OlyWeeklyPoints = 3`.

After each instant the driver prints the clock, the cycle, the period and the
held noble points (`@... cycle=... period=... nobles=...`).

### The one deviation

The reference `init()` renews the Olympiad end with `_olympiadEnd <
currentTime`; the probe uses `<=`, as the Go calendar does
(`internal/gameserver/olympiad/olympiad.go`, `enterPeriod`). In the reference,
a step due at the Olympiad end runs once its clock has moved on, usually at
least 1 ms later, so the two agree. On a clock that never moves, `<` renews
nothing and the steps due at the end repeat at zero delay forever. #3280 asks
whether to keep the calendar's daily wipe and unreachable end at all.

## Scenarios

`main` runs five scenarios, in the order `timelineScenarios` lists them in
the test. Each has a start instant, how many instants to step, the stored
cycle and the stored noble points. A change to one side needs the same change
to the other.

## Regenerating

From the repository root:

```bash
docker run --rm -v "$PWD/tests/olympiad/testdata/oracle:/p" -w /p \
  eclipse-temurin:21-jdk-alpine java -Duser.timezone=UTC OlyProbe.java \
  > tests/olympiad/testdata/timeline.golden
```

The Go test's virtual clock is in UTC, and the calendar reads the time of
day in the clock's zone, so the probe runs with `-Duser.timezone=UTC`. Regenerate only when the
reference calendar or the scenarios change. A Go-side change that alters the
trace is a parity break unless it is a recorded deviation.
