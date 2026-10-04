# Lottery oracle

`../lottery.golden` is the expected output of `TestLotteryMatchesReferenceProbe`
(`tests/lottery/oracle_test.go`). It is the output of `LotteryProbe.java` in
this directory, a probe of the reference lottery.

## What the probe is

`LotteryProbe.java` copies, from aCis 409
`aCis_gameserver/java/net/sf/l2j/gameserver/data/manager/LotteryManager.java`,
`StartLottery`, `StopSellingTickets`, `FinishLottery`, `increasePrize`,
`decodeNumbers` and `checkTicket`, and from
`aCis_gameserver/java/net/sf/l2j/gameserver/model/actor/Npc.java`
`showLotoWindow`'s button handling (pages 1-21), the instructions page's
`String.valueOf(rate * 100)` and the `%enddate%`
`DateFormat.getDateInstance().format(endDate)`. The copies are unchanged
except:

- `System.currentTimeMillis()` reads a virtual clock.
- `ThreadPool.schedule` records a task at `now + max(0, delay)`; the driver
  steps the clock to the soonest task, runs every task due by then (ties in
  scheduling order) and prints the state.
- `Rnd.get(20)` returns the scenario's scripted rolls.
- The `games` rows and the ticket items are in-memory rows; every statement
  prints a `DB ...` line. World broadcasts print `ANN ...` (the
  announcement text) and `SM <id> <numbers>` lines.
- Scripted purchases (`BUY`) raise the jackpot through `increasePrize` and
  add a ticket, but only while a round is started and selling, as
  `showLotoWindow` page 22 requires.

The configuration is the shipped default: `LotteryPrize = 50000`,
`LotteryTicketPrice = 2000`, rates 0.6 / 0.2 / 0.2 and
`Lottery2and1NumberPrize = 200`.

The reference `decodeNumbers` throws on a mask holding more than five
numbers; no ticket the seller sells holds one, so the probe does not
include that case, and the Go `Decode` lists the first five.

## Regenerating

From the repository root:

```bash
docker run --rm -v "$PWD/tests/lottery/testdata/oracle:/p" -w /p \
  eclipse-temurin:21-jdk-alpine java -Duser.timezone=UTC \
  -Duser.language=en -Duser.country=US LotteryProbe.java \
  > tests/lottery/testdata/lottery.golden
```

The Go test's virtual clock is in UTC and the calendar reads the day of the
week and the hour in the clock's zone, so the probe runs in UTC. The drawing
date is formatted in the `en_US` locale's medium style. The scenarios are
listed on both sides: a change to one needs the same change to the other.
