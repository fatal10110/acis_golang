package npcs

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// With server.properties NoSpawns=True the boot spawn pass is skipped
// whole: no on-start maker, no SpawnEvents maker and no Seven Signs group
// spawns, so the character sees no NPC. The next Seven Signs period change
// still spawns the groups of the new period.
func TestNoSpawnsLeavesTheMakersUnspawnedUntilAPeriodChange(t *testing.T) {
	t.Parallel()
	w := bootMakers(t, validationStatus, gameservertest.WithNoSpawns())
	c := w.srv.Client

	if got := shown(w.burst); len(got) != 0 {
		t.Fatalf("NPCs shown at enter world = %v, want none", got)
	}
	if got := w.srv.NpcSpawns.LiveCount(); got != 0 {
		t.Fatalf("live spawned npcs = %d, want none", got)
	}

	if w.change == nil {
		t.Fatal("no period change pending")
	}
	w.change()
	w.srv.Settle(t)
	if got := shown(drainFrames(t, c)); !slices.Equal(got, repeatID(makerSSQEventID, 3)) {
		t.Fatalf("NPCs shown after the period change = %v, want the event group", got)
	}
}
