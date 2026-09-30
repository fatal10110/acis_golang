package effect

import (
	"fmt"
	"testing"
	"time"
)

// flagReadingOwner reports each stat change by reading its list's flags
// back, the way a player's RUN_SPEED refresh rebuilds its appearance for
// observers (CharInfo reads the fake-death flag from this list).
type flagReadingOwner struct {
	iconEventOwner
	list **List
}

func (o flagReadingOwner) StatFuncsAttached([]Mod) {
	(*o.list).Flags()
	*o.events = append(*o.events, "owner:report")
}

// runBounded runs fn and fails the test if it has not returned within a
// second: a stat report under the list's own lock never returns.
func runBounded(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("%s did not return: the stat report re-entered the list lock", what)
	}
}

// An activating effect's stat report runs after the list lock is released,
// so an owner refresh that reads the list back completes. The report
// follows the funcs attaching and precedes the felt message and the icon
// refresh, on first activation and on promotion after the head leaves.
func TestListStatReportRunsOutsideListLock(t *testing.T) {
	var events []string
	var list *List
	list = newTestList(flagReadingOwner{iconEventOwner{eventOwner{events: &events}}, &list}, WithEnv(Env{KeepLesser: true}))
	weak := namedEffect("weak", 2011, "speed_up", 1, false, &events)
	weak.Template.Icon = true
	strong := namedEffect("strong", 2034, "speed_up", 2, false, &events)
	strong.Template.Icon = true
	plain := namedEffect("plain", 1204, "none", 0, false, &events)

	runBounded(t, "stacked add", func() { list.Add(weak) })
	requireEvents(t, events, []string{"weak:start", "owner:add", "owner:report", "felt:2011:0", "icons"})

	events = nil
	runBounded(t, "unstacked add", func() { list.Add(plain) })
	requireEvents(t, events, []string{"plain:start", "owner:add", "owner:report", "icons"})

	runBounded(t, "stack displacement", func() { list.Add(strong) })
	events = nil
	runBounded(t, "head removal", func() { list.Remove(strong) })
	requireEvents(t, events, []string{"owner:remove:strong", "weak:start", "owner:add", "owner:report", "worn-off:2034:0", "icons", "strong:exit"})
}

// strippedOwner records each stat removal with whether its effect owner was
// ended by a stop-all.
type strippedOwner struct{ iconEventOwner }

func (o strippedOwner) RemoveStatsByOwner(owner ModOwner) {
	*o.events = append(*o.events, fmt.Sprintf("owner:remove:%s:stripped=%v", owner.effect.Template.Name, owner.Stripped()))
}

// A stop-all ends each effect with its stat removal marked as stripped, so
// the holder skips the per-effect refresh (EffectList.stopAllEffects ->
// AbstractEffect.exit(true) -> Creature.removeStatsByOwner skipping
// broadcastModifiedStats while cantUpdateAnymore, Creature.java:1198-1204).
// A stack member promoted mid-strip still reports its own activation, and a
// plain Remove or a death-surviving effect is not marked.
func TestListStopAllMarksStatRemovalsStripped(t *testing.T) {
	var events []string
	list := newTestList(strippedOwner{iconEventOwner{eventOwner{events: &events}}}, WithEnv(Env{KeepLesser: true}))
	strong := namedEffect("strong", 2034, "speed_up", 2, false, &events)
	weak := namedEffect("weak", 2011, "speed_up", 1, false, &events)
	plain := namedEffect("plain", 1204, "none", 0, false, &events)
	kept := namedEffect("kept", 1323, "none", 0, false, &events)
	kept.Skill.StayAfterDeath = true
	removed := namedEffect("removed", 1068, "none", 0, false, &events)
	for _, e := range []*Effect{strong, weak, plain, kept, removed} {
		list.Add(e)
	}

	events = nil
	list.Remove(removed)
	requireEvents(t, events, []string{"owner:remove:removed:stripped=false", "icons", "removed:exit"})

	events = nil
	list.StopAllExceptThoseThatLastThroughDeath()
	requireNames(t, list.All(), []string{"kept"})
	want := []string{
		"owner:remove:strong:stripped=true", "weak:start", "owner:add", "icons", "strong:exit",
		"owner:remove:weak:stripped=true", "icons", "weak:exit",
		"owner:remove:plain:stripped=true", "icons", "plain:exit",
	}
	requireEvents(t, events, want)

	events = nil
	list.StopAll()
	requireEvents(t, events, []string{"owner:remove:kept:stripped=true", "icons", "kept:exit"})
}
