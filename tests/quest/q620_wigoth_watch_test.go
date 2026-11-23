package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// q620WigothWatch is the name of the watch timer each ghost of Wigoth runs.
const q620WigothWatch = "62001"

// TestQ620WigothWatchStopsWithItsGhost (#5168): each Halisha kill leaves a
// one-off ghost of Wigoth whose watch ticks while it stands; once the ghost
// is gone, whether its own watch deleted it at the hour's closing or it was
// deleted earlier, no watch of any ghost stays behind.
func TestQ620WigothWatchStopsWithItsGhost(t *testing.T) {
	t.Parallel()
	w := q620Boot(t, q620Spec{})
	x, y, z := w.srv.PlayerPosition(t, w.ids["Leader"])

	watching := func(ghost *script.NPC) bool {
		t.Helper()
		var pending bool
		if !w.srv.RunScript(t, w.ids["Leader"], q620Name, func(s *script.Script, _ *script.Player) {
			pending = s.HasTimer(q620WigothWatch, ghost, nil)
		}) {
			t.Fatal("the timer read panicked")
		}
		return pending
	}
	killHalisha := func() *script.NPC {
		t.Helper()
		boss := w.srv.SpawnHostileNPCTemplateAt(t, q620Template(q620Halisha, "Monster"), location.Location{X: x + 40, Y: y, Z: z})
		killOnPlayerQueue(t, w.srv, w.ids["Leader"], boss)
		ghosts := npcsOf(w.srv, q620Wigoth)
		if len(ghosts) != 1 {
			t.Fatalf("ghosts of Wigoth = %d, want 1", len(ghosts))
		}
		if !watching(ghosts[0]) {
			t.Fatal("the new ghost of Wigoth runs no watch")
		}
		return ghosts[0]
	}

	// First hour: the watch deletes its ghost at 54 minutes past the hour.
	w.toMinute(t, 20, 10)
	first := killHalisha()
	w.toMinute(t, 53, 59)
	if !watching(first) {
		t.Fatal("the watch stopped while its ghost still stands")
	}
	w.srv.Advance(t, 3*time.Second)
	if n := len(npcsOf(w.srv, q620Wigoth)); n != 0 {
		t.Fatalf("ghosts of Wigoth after 54 minutes = %d, want 0", n)
	}
	if watching(first) {
		t.Fatal("the watch of the ghost deleted at 54 minutes is still pending")
	}

	// Next hour: a ghost deleted before the closing stops its watch too.
	w.toMinute(t, 20, 10)
	second := killHalisha()
	if second == first {
		t.Fatal("the second kill reused the first ghost's handle")
	}
	if !w.srv.RunScript(t, w.ids["Leader"], q620Name, func(*script.Script, *script.Player) { second.DeleteMe() }) {
		t.Fatal("deleting the ghost panicked")
	}
	if n := len(npcsOf(w.srv, q620Wigoth)); n != 0 {
		t.Fatalf("ghosts of Wigoth after the delete = %d, want 0", n)
	}
	w.srv.Advance(t, 3*time.Second)
	for name, ghost := range map[string]*script.NPC{"first": first, "second": second} {
		if watching(ghost) {
			t.Errorf("the watch of the %s ghost is still pending", name)
		}
	}
}
