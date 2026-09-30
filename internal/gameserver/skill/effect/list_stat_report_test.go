package effect

import (
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
