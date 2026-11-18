package items

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The ways a line comes out that no cast of the player ends: the fish
// stealing the bait, spitting the hook, and a night lure by day
// (FishingStance.start's look task and fight task).

// fishingMsg is the event line of the system message id.
func fishingMsg(id int) string { return fmt.Sprintf("msg %d", id) }

// countTicks counts the fight's per-second gauges in events.
func countTicks(events []string) int {
	n := 0
	for _, e := range events {
		if strings.HasPrefix(e, "tick ") {
			n++
		}
	}
	return n
}

// TestFishingFishSpitsTheHook: a fight nobody acts in runs its 24 seconds,
// a gauge each, and the second after the last the fish spits the hook: the
// fisher is told so, the line comes out without a catch and the fisher is
// told it is reeled in.
func TestFishingFishSpitsTheHook(t *testing.T) {
	t.Parallel()
	r := bootFishing(t)
	id := r.objID
	r.cast(t, fishingSkill)
	r.awaitEvent(t, "combat ")
	fishingEvents(t, r.c)

	events, _ := r.awaitEvent(t, fishingMsg(serverpackets.SystemMessageFishSpitTheHook))
	if n := countTicks(events); n != 24 {
		t.Fatalf("gauges before the hook was spat = %d, want the fight's 24", n)
	}
	if last := events[len(events)-2]; last != "tick time 0 hp 100 mode 0 penalty 0 deceptive 0" {
		t.Fatalf("last gauge = %q, want the fight's time at 0", last)
	}
	requireEvents(t, "spat", fishingEvents(t, r.c), fmt.Sprintf("end %d win 0", id), fishingMsg(serverpackets.SystemMessageReelLineAndStopFishing))
	r.srv.Advance(t, 5*time.Second)
	requireEvents(t, "after the spit", fishingEvents(t, r.c))
}

// TestFishingFishStealsTheBait: a fish that has got its HP back to twice
// its maximum takes the bait at the next second: two failed reels on a
// resting fish give it 52, then it regains 4 a second once fighting, and
// the second after it reaches exactly 200 the fisher is told the bait was
// stolen and the line comes out without a catch. A fish whose HP passes
// 200 by an action's failure is lost there instead.
func TestFishingFishStealsTheBait(t *testing.T) {
	t.Parallel()
	r := bootFishing(t)
	id := r.objID
	r.cast(t, fishingSkill)
	r.awaitEvent(t, "combat ")
	fishingEvents(t, r.c)

	for i, hp := range []int{126, 152} {
		requireEvents(t, fmt.Sprintf("reel %d", i+1), r.cast(t, reelingSkill), append(castStart(reelingSkill),
			"msg 1468 26",
			fmt.Sprintf("gauge hp %d mode 0 good 2 anim 2 penalty 0 deceptive 0", hp))...)
	}
	// The fish turns to fight at its next look, and stays so.
	r.dice.check.Store(90)
	if events, _ := r.awaitEvent(t, "tick "); events[len(events)-1] != "tick time 23 hp 152 mode 1 penalty 0 deceptive 0" {
		t.Fatalf("first second = %q, want the fish fighting at 152", events)
	}
	r.dice.check.Store(50)

	events, _ := r.awaitEvent(t, fishingMsg(serverpackets.SystemMessageBaitStolenByFish))
	if last := events[len(events)-2]; last != "tick time 11 hp 200 mode 1 penalty 0 deceptive 0" {
		t.Fatalf("last gauge = %q, want the fish at exactly 200", last)
	}
	if slicesContainsPrefix(events, "end ") {
		t.Fatalf("line came out before the bait was stolen: %q", events)
	}
	requireEvents(t, "stolen", fishingEvents(t, r.c), fmt.Sprintf("end %d win 0", id), fishingMsg(serverpackets.SystemMessageReelLineAndStopFishing))
}

func slicesContainsPrefix(events []string, prefix string) bool {
	for _, e := range events {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

// TestFishingNightLureByDay: a night lure draws no fish type by day (the
// start shows -1) and gets no bite however the dice fall, the line coming
// out empty at the first look past the wait; by night the same cast gets its bite.
func TestFishingNightLureByDay(t *testing.T) {
	t.Parallel()
	for _, night := range []bool{false, true} {
		t.Run(fmt.Sprintf("night=%v", night), func(t *testing.T) {
			t.Parallel()
			r := bootFishingWith(t, false)
			id := r.objID
			// The in-game hour follows the wall clock: start every run by day.
			if r.srv.CrossDayNight(t) {
				r.srv.CrossDayNight(t)
			}
			drainUntilQuiet(t, r.c)
			r.c.Send(encodeUseItem(r.nightLure, false))
			fishingEvents(t, r.c)
			fishType := -1
			if night {
				if !r.srv.CrossDayNight(t) {
					t.Fatal("the clock was already night")
				}
				fishType = 1
				drainUntilQuiet(t, r.c)
			}
			requireEvents(t, "cast", r.cast(t, fishingSkill), append(castStart(fishingSkill),
				"msg 1461",
				fmt.Sprintf("start %d type %d at 260,20,10 night 1 ranking 0", id, fishType),
				"sound 1 SF_P_01")...)
			if night {
				requireEvents(t, "bite", func() []string { e, _ := r.awaitEvent(t, "combat "); return e }(),
					fmt.Sprintf("combat %d time 24 hp 100 mode 0 lure 1 deceptive 0", id))
				return
			}
			ended, after := r.awaitEvent(t, "end ")
			// This low grade lure looks every 6.65 s from the 10th: the
			// look past the 30 s deadline is the fifth, at 36.6 s.
			if after < 36500*time.Millisecond || after > 37100*time.Millisecond {
				t.Fatalf("empty line back %v after the cast, want 36.6s", after)
			}
			requireEvents(t, "no bite", ended, fmt.Sprintf("end %d win 0", id))
			requireEvents(t, "no bite notice", fishingEvents(t, r.c), fishingMsg(serverpackets.SystemMessageReelLineAndStopFishing))
		})
	}
}
