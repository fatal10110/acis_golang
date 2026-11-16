package quest

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestQ620KeeperLeavesBehindWhosePassIsGone: a member whose entrance pass is
// moved away after the keeper checked every member and before his own take
// stays behind: he is not moved, gets no Used Grave Pass and keeps his
// completed mark; the rest of the party goes in as before. The take reports
// what it took, an owner-approved divergence (#5165): a pass is never both
// kept and used.
func TestQ620KeeperLeavesBehindWhosePassIsGone(t *testing.T) {
	t.Parallel()
	// The sepulcher world moves its clock to the entry minute, up to an
	// hour on, which the real pool cannot step through in a test's time:
	// the entry runs on the inline executor only.
	// The move runs on the leader's queue, at the leader's Used Grave Pass:
	// the entry's first give, past every member's check and before Mate2's
	// take.
	var move atomic.Pointer[func()]
	ids := func(next func() (int32, error)) func() (int32, error) {
		return func() (int32, error) {
			if f := move.Swap(nil); f != nil {
				(*f)()
			}
			return next()
		}
	}
	w := q620Boot(t, q620Spec{mates: 3, extra: []gameservertest.Option{gameservertest.WithScriptItemIDs(ids)}})
	keeper := w.folk(t, "keeper", q620Keeper)
	w.toMinute(t, 56, 10)

	id := w.ids["Mate2"]
	x0, y0, _ := w.srv.PlayerPosition(t, id)
	mate2 := onlineCharacter(t, w.srv, id)
	moved := false
	takePass := func() { moved = mate2.ConsumeItem(q620Pass, 1) }
	move.Store(&takePass)

	got := w.enter(t, "Leader", keeper.ObjectID())
	requireLine(t, got, htmlLine("keeper", keeperPage(t, "-08.htm")))
	w.arrive(t)
	if !moved {
		t.Fatal("Mate2's pass was not moved away during the entry")
	}

	// The step every member takes on arrival moves him 30 at most.
	if x, y, _ := w.srv.PlayerPosition(t, id); x < x0 || x > x0+30 || y != y0 {
		t.Errorf("Mate2 moved to %d,%d, want to stay at %d,%d", x, y, x0, y0)
	}
	if n := w.srv.PlayerItemCount(t, id, q620UsedPass); n != 0 {
		t.Errorf("Mate2 holds %d used grave passes, want 0", n)
	}
	if v := w.srv.QuestVars(t, id, q620Name)["completed"]; v != "true" {
		t.Errorf("Mate2's completed = %q, want true", v)
	}
	for _, name := range []string{"Leader", "Mate1", "Mate3"} {
		x, y, _ := w.srv.PlayerPosition(t, w.ids[name])
		if dx, dy := x-q620Sepulcher.X, y-q620Sepulcher.Y; dx < -80 || dx > 80 || dy < -80 || dy > 80 {
			t.Errorf("%s stands at %d,%d, want within 80 of the sepulcher", name, x, y)
		}
		if n := w.srv.PlayerItemCount(t, w.ids[name], q620UsedPass); n != 1 {
			t.Errorf("%s holds %d used grave passes, want 1", name, n)
		}
	}

	w.srv.FlushItems(t)
	if rows := itemRows(t, w.srv, id); rows[q620Pass] != 0 || rows[q620UsedPass] != 0 {
		t.Errorf("Mate2's item rows = %v, want no pass and no used grave pass", rows)
	}
}
