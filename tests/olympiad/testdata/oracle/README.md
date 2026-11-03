# Olympiad calendar oracle

`../timeline.golden` is the expected trace of `TestOlympiadCalendarTimeline`
(`tests/olympiad/timeline_test.go`). It is the output of `OlyProbe.java` in
this directory, a probe of the calendar chosen in #3280: the daily windows
and weekly grant of aCis 409 on the monthly cycle of aCis 401.

## What the probe is

`OlyProbe.java` copies these methods of aCis 409
`aCis_gameserver/java/net/sf/l2j/gameserver/model/olympiad/Olympiad.java`:
`reschedule`, `getDelay`, `executeTask`, `endOlympiad`, `checkPendingGames`,
`schedulePeriodEnd`, `revalidatePeriod`, `init`, `startCompetition`,
`setNextNoblePointsUpdate`, `setNewOlympiadEnd` and `deleteNobles`. The
copies are unchanged except:

- `System.currentTimeMillis()` and `Calendar.getInstance()` read a virtual
  clock.
- `ThreadPool.schedule` records the pending delay. The driver then steps the
  clock to the soonest delay and runs every step due at that instant.
- Database, broadcast and game manager calls print trace lines (`DB ...`,
  `ANN ...`). No match ever runs (`isBattleStarted()` is false), and no hero
  is elected.
- The configuration is the shipped default: `OlyStartTime = 18`,
  `OlyMin = 00`, `OlyCPeriod = 21600000`, `OlyWeeklyPoints = 3`, plus the
  aCis 401 default `OlyVPeriod = 86400000` for the validation period.

After each instant the driver prints the clock, the cycle, the period and the
held noble points (`@... cycle=... period=... nobles=...`). The period is
`COMPETITION` inside a window, `INTERVAL` between two windows of a running
Olympiad, and `VALIDATION` from an Olympiad's end until the next cycle opens.

## The monthly cycle (aCis 401)

The 409 calendar opens a new cycle and clears the records at every window's
opening, and never reaches the Olympiad's end. The probe brings in the 401
cycle (`git -C aCis_gameserver show 6537f102:./java/net/sf/l2j/gameserver/model/olympiad/Olympiad.java`:
`olympiadEnd`, `validationEnd`, `load`), each change marked `401:`:

- A window's opening only enters the period; it starts no cycle and clears
  no record.
- The Olympiad's end (noon on the first) announces it, saves the records and
  the status and keeps the month's standings, then a validation period of
  `OlyVPeriod` follows, with no window and no Olympiad end. Its close opens
  the next cycle: the records are cleared, the cycle is announced, and the
  next Olympiad end is set.
- A weekly grant due in the validation period adds nothing.
- The Olympiad's end moves only when a cycle opens.
- At start, a stored validation period resumes, or opens the next cycle at
  once when it is already over.

## Deviations

Each marked `Deviation:` in the probe and matched by the Go calendar:

- `END_OLYMPIAD` ties ahead of a window's opening, and from the Olympiad's
  end on `revalidatePeriod` opens no window, so no window opens at the end
  itself. In 409 the window steps never tie with the end.
- The close of the validation period is set before the status save at the
  end, and the status is saved as soon as a cycle opens, so a crash in
  either keeps the calendar where it was. aCis 401 saves the old close at
  the end, and nothing when a cycle opens.

## Scenarios

`main` runs the scenarios in the order `timelineScenarios` lists them in the
test. Each has a start instant, how many instants to step, the stored
cycle, the stored noble points and the stored close of a validation period.
A change to one side needs the same change to the other.

## Regenerating

From the repository root:

```bash
docker run --rm -v "$PWD/tests/olympiad/testdata/oracle:/p" -w /p \
  eclipse-temurin:21-jdk-alpine java -Duser.timezone=UTC OlyProbe.java \
  > tests/olympiad/testdata/timeline.golden
```

The Go test's virtual clock is in UTC, and the calendar reads the time of
day in the clock's zone, so the probe runs with `-Duser.timezone=UTC`. Regenerate only when the
calendar or the scenarios change. A Go-side change that alters the trace is
a parity break unless it is a recorded deviation.
