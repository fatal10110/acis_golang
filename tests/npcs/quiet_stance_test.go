package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFolkQuietStanceChangeShowsNothing: a script setting a folk NPC's
// stance without showing it (creature.setRunning) flips the stance, setting
// the stance it already has changes nothing, and neither player around it
// is sent a frame.
func TestFolkQuietStanceChangeShowsNothing(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	folk := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 31921), location.Location{X: w.at.X, Y: w.at.Y + 40, Z: w.at.Z})
	w.srv.ReadQueued(t, w.first)
	w.srv.ReadQueued(t, w.second)

	running := folk.Running()
	w.direct(t, func(_ *script.Player) {
		n := script.NPCOf(folk)
		n.SetRunningQuietly(!running)
		n.SetRunningQuietly(!running)
	})
	if folk.Running() == running {
		t.Fatalf("SetRunningQuietly(%v) left the folk stance at %v", !running, running)
	}
	w.direct(t, func(_ *script.Player) { script.NPCOf(folk).SetRunningQuietly(running) })
	if folk.Running() != running {
		t.Fatalf("SetRunningQuietly(%v) left the folk stance at %v", running, !running)
	}
	if got := w.srv.ReadQueued(t, w.first); len(got) != 0 {
		t.Errorf("first: the quiet stance changes sent %d frames, want none", len(got))
	}
	if got := w.srv.ReadQueued(t, w.second); len(got) != 0 {
		t.Errorf("second: the quiet stance changes sent %d frames, want none", len(got))
	}
}
