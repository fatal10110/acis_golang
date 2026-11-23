package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestAntharasCallsNoBehemothDead covers #5134: restored fighting at 3 at
// full HP and killed 10 s before its 2-minute check, its corpse still in
// the world at the check calls no Behemoth Dragon, nor does anything after.
func TestAntharasCallsNoBehemothDead(t *testing.T) {
	w := bootAntharas(t, antharasLiveRow(3), "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	handle := script.NPCOf(boss)

	w.advance(t, 110*time.Second)
	w.onQueue(t, boss, func() { handle.DoDie(script.PlayerOf(w.player)) })
	w.advance(t, 10*time.Second)
	if boss.Decayed() {
		t.Fatal("Antharas's corpse decayed before its check, want it still in the world")
	}
	if n := len(baiumNPCs(w, antharasBehemothID)); n != 0 {
		t.Fatalf("%d Behemoth Dragons at the check on its corpse, want none", n)
	}
	w.advance(t, 5*time.Minute)
	if n := len(baiumNPCs(w, antharasBehemothID)); n != 0 {
		t.Fatalf("%d Behemoth Dragons after its death, want none", n)
	}
}
