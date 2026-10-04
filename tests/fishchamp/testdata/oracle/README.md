# Fishing championship oracle

`../fishchamp.golden` is the expected output of
`TestFishingChampionshipMatchesReferenceProbe` (`tests/fishchamp/oracle_test.go`).
It is the output of `FishChampProbe.java` in this directory, a probe of the
reference fishing championship, run on `../scenarios.txt`.

## What the probe is

`FishChampProbe.java` copies, from aCis 409
`aCis_gameserver/java/net/sf/l2j/gameserver/data/manager/FishingChampionshipManager.java`,
the constructor, `setEndOfChamp`, `restoreData`, `refreshResult`,
`refreshWinResult`, `finishChamp`, `recalculateMinLength`, `newFish`,
`getTimeRemaining`, the name and length getters, `isWinner`, `getReward`,
`showMidResult`, `showChampScreen` and `shutdown`, and from
`aCis_gameserver/java/net/sf/l2j/gameserver/model/actor/instance/Fisherman.java`
the `FishingReward` branch's `isWinner` check. The copies are unchanged
except:

- `System.currentTimeMillis()` reads a virtual clock.
- `ThreadPool.schedule` records a task at `now + max(0, delay)`; an
  `advance` step runs every task due by its instant, soonest first (ties in
  scheduling order), at the task's own instant.
- `Rnd.get(min, max)` returns `min` plus the step's next scripted roll.
- `server_memo` and `fishing_championship` are in-memory rows; `shutdown`
  stores each length rounded to three decimals, as the `DOUBLE(10,3)` column
  keeps it.
- Sent packets print as lines: `MSG <id> [text]`, `ADD <item> <count>`,
  `HTML <file> [filled template]`. The pages are a placeholder template
  (`%TABLE%|%prizeItem%|...`) instead of the page file, the prize item named
  `Adena`, the fisherman's object id 1000.

The configuration is the shipped default: `AllowFishChampionship = True`,
`FishChampionshipRewardItemId = 57`, prizes 800000, 500000, 300000, 200000
and 100000.

The Go side saves after every change as well as on stop; the reference saves
only when a week ends and at shutdown. The scenarios compare the stored rows
only after a save on both sides (`rows`, `restart`), so the extra saves do
not show.

## Regenerating

From the repository root:

```bash
docker run --rm -v "$PWD/tests/fishchamp/testdata:/p" -w /p/oracle \
  eclipse-temurin:21-jdk-alpine java -Duser.timezone=UTC \
  -Duser.language=en -Duser.country=US FishChampProbe.java ../scenarios.txt \
  > tests/fishchamp/testdata/fishchamp.golden
```

The Go test's virtual clock is in UTC and the week end reads the day of the
week and the hour in the clock's zone, so the probe runs in UTC. The `en_US`
locale starts the week on Sunday, which decides which Tuesday
`Calendar.set(DAY_OF_WEEK, 3)` picks.
