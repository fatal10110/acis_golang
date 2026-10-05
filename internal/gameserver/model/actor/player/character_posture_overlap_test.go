package player

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// Reference overlapping posture transitions: Player.sitDown, standUp,
// startFakeDeath and stopFakeDeath (Player.java:1542-1590, 7017-7056) each
// raise their own _isSittingNow / _isStandingNow flag, leave the other one
// alone, and schedule an uncancelled task. Every task fires at its own time:
// a sit-down's or lie-down's end clears _isSittingNow, sets _isSitting and
// notifies SAT_DOWN; a stand-up's or get-up's end clears _isStandingNow and
// notifies STOOD_UP. A later transition never drops an earlier one's end.

// settledStances lists the StoodUp flag of every PostureSettled rec holds.
func settledStances(rec *event.Recorder) []bool {
	var out []bool
	for _, e := range event.Of[event.PostureSettled](rec) {
		out = append(out, e.StoodUp)
	}
	return out
}

// TestSitAtGetUpEndKeepsLieDownEnd pins issue #3353's timeline: a get-up
// started at once ends inside the lie-down, and the sit it then runs (the
// player took the standing posture, so PlayerAI.thinkSit sits it down) does
// not drop the lie-down's end. The lie-down still ends at its own time,
// clearing SittingNow and seating the player while the sit-down runs on;
// the sit-down's own end follows sitStandDelay after it began.
func TestSitAtGetUpEndKeepsLieDownEnd(t *testing.T) {
	c, clock, lieDown, getUp := fakeDeathTimingCharacter(t)
	if getUp >= lieDown {
		t.Fatalf("get-up %v not shorter than lie-down %v; the scenario needs the get-up to end first", getUp, lieDown)
	}
	c.StartFakeDeath()
	rec := recordEvents(c)
	c.StopFakeDeath()
	clock.Advance(getUp)
	if !c.Standing() || c.StandingNow() || !c.SittingNow() || c.FakeDead() {
		t.Fatalf("at the get-up's end Standing=%v StandingNow=%v SittingNow=%v FakeDead=%v, want standing, the lie-down under way, out of fake death",
			c.Standing(), c.StandingNow(), c.SittingNow(), c.FakeDead())
	}

	c.Sit()
	if c.Standing() || !c.SittingNow() || c.Seated() {
		t.Fatalf("after the sit Standing=%v SittingNow=%v Seated=%v, want sitting down", c.Standing(), c.SittingNow(), c.Seated())
	}

	clock.Advance(lieDown - getUp - time.Millisecond)
	if !c.SittingNow() {
		t.Fatal("SittingNow() = false 1ms before the lie-down ends")
	}
	clock.Advance(time.Millisecond)
	if c.SittingNow() || !c.Seated() {
		t.Fatalf("at the lie-down's end SittingNow=%v Seated=%v, want the lie-down's end to clear it and seat the player", c.SittingNow(), c.Seated())
	}
	if got, want := settledStances(rec), []bool{true, false}; !slices.Equal(got, want) {
		t.Fatalf("PostureSettled at the lie-down's end = %v, want %v (get-up, lie-down)", got, want)
	}

	clock.Advance(getUp + sitStandDelay - lieDown - time.Millisecond)
	if got := len(settledStances(rec)); got != 2 {
		t.Fatalf("PostureSettled 1ms before the sit-down ends = %d, want 2", got)
	}
	clock.Advance(time.Millisecond)
	if got, want := settledStances(rec), []bool{true, false, false}; !slices.Equal(got, want) {
		t.Fatalf("PostureSettled after the sit-down = %v, want %v", got, want)
	}
	if !c.Seated() || c.SittingNow() || c.StandingNow() {
		t.Fatalf("after every end Seated=%v SittingNow=%v StandingNow=%v, want seated", c.Seated(), c.SittingNow(), c.StandingNow())
	}
}

// TestFakeDeathAtGetUpEndKeepsLieDownEnd is TestSitAtGetUpEndKeepsLieDownEnd
// for a Fake Death started at the get-up's end: the new lie-down does not
// drop the first one's end, which clears SittingNow at its own time.
func TestFakeDeathAtGetUpEndKeepsLieDownEnd(t *testing.T) {
	c, clock, lieDown, getUp := fakeDeathTimingCharacter(t)
	c.StartFakeDeath()
	rec := recordEvents(c)
	c.StopFakeDeath()
	clock.Advance(getUp)
	c.StartFakeDeath()

	clock.Advance(lieDown - getUp)
	if c.SittingNow() || !c.Seated() || !c.FakeDead() {
		t.Fatalf("at the first lie-down's end SittingNow=%v Seated=%v FakeDead=%v, want seated, playing dead", c.SittingNow(), c.Seated(), c.FakeDead())
	}
	if got, want := settledStances(rec), []bool{true, false}; !slices.Equal(got, want) {
		t.Fatalf("PostureSettled at the first lie-down's end = %v, want %v", got, want)
	}
	clock.Advance(getUp)
	if got, want := settledStances(rec), []bool{true, false, false}; !slices.Equal(got, want) {
		t.Fatalf("PostureSettled after the second lie-down = %v, want %v", got, want)
	}
}

// TestResitKeepsEarlierSitEnd pins a sit-down begun again while one is under
// way (Relax, a store opening: Sit always starts a transition): the first
// sit-down's end still clears SittingNow and notifies, and the second one
// notifies again at its own end.
func TestResitKeepsEarlierSitEnd(t *testing.T) {
	c, clock, _, _ := fakeDeathTimingCharacter(t)
	rec := recordEvents(c)
	c.Sit()
	clock.Advance(time.Second)
	c.Sit()

	clock.Advance(sitStandDelay - time.Second)
	if c.SittingNow() || !c.Seated() {
		t.Fatalf("at the first sit-down's end SittingNow=%v Seated=%v, want seated", c.SittingNow(), c.Seated())
	}
	if got := len(settledStances(rec)); got != 1 {
		t.Fatalf("PostureSettled at the first sit-down's end = %d, want 1", got)
	}
	clock.Advance(time.Second)
	if got, want := settledStances(rec), []bool{false, false}; !slices.Equal(got, want) {
		t.Fatalf("PostureSettled after the second sit-down = %v, want %v", got, want)
	}
}

// TestStandUpDuringSitDownKeepsBothFlags pins a stand-up begun while a
// sit-down is under way (enterObserverMode stands the player up whatever its
// posture, Player.java:5240-5252): standUp leaves _isSittingNow up, the
// sit-down's end seats the player (_isSitting) without ending the stand-up,
// and the stand-up's own end clears StandingNow at its time and leaves the
// player seated, as the reference's isSitting() stays true.
func TestStandUpDuringSitDownKeepsBothFlags(t *testing.T) {
	c, clock, _, _ := fakeDeathTimingCharacter(t)
	rec := recordEvents(c)
	c.Sit()
	clock.Advance(time.Second)
	c.StandUp()
	if !c.Standing() || !c.SittingNow() || !c.StandingNow() {
		t.Fatalf("stand-up during the sit-down Standing=%v SittingNow=%v StandingNow=%v, want standing with both under way",
			c.Standing(), c.SittingNow(), c.StandingNow())
	}

	clock.Advance(sitStandDelay - time.Second)
	if c.SittingNow() || !c.StandingNow() || !c.Seated() {
		t.Fatalf("at the sit-down's end SittingNow=%v StandingNow=%v Seated=%v, want seated, the stand-up still under way",
			c.SittingNow(), c.StandingNow(), c.Seated())
	}
	clock.Advance(time.Second - time.Millisecond)
	if !c.StandingNow() {
		t.Fatal("StandingNow() = false 1ms before the stand-up ends: the sit-down's end ended it")
	}
	clock.Advance(time.Millisecond)
	if c.StandingNow() || !c.Seated() {
		t.Fatalf("after the stand-up StandingNow=%v Seated=%v, want the stand-up ended and the player still seated", c.StandingNow(), c.Seated())
	}
	if got, want := settledStances(rec), []bool{false, true}; !slices.Equal(got, want) {
		t.Fatalf("PostureSettled = %v, want %v", got, want)
	}
}
