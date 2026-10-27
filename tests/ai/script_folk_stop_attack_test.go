package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestScriptStopsAFolkSwingInFlight pins CreatureAttack.stop on a civilian
// NPC (the quests that halt an NPC's attack before it leaves): a civilian
// patrol striking back at the character has its swing in flight; the
// script drops its attack desires and stops the attack, and the hit never
// lands, nothing is shown, and no new swing follows.
func TestScriptStopsAFolkSwingInFlight(t *testing.T) {
	t.Parallel()
	guard := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
			OnAttacked: func(_ *script.Script, e script.Attacked) { e.NPC.AddAttackDesire(e.Attacker, 2000) },
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkGuard": guard})
	player := characterOf(t, srv)
	tmpl := folkFighterTemplate(folkPatrolID)
	tmpl.Level = 80 // a near-certain hit, so the stop has a hit to cancel
	f := srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
	srv.ReadQueued(t, srv.Client)
	vitals := player.(interface{ CurrentHP() int })
	hpBefore := vitals.CurrentHP()

	hitFolk(t, srv, f, player, 10)
	if swings := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeAttack, f.ObjectID()); len(swings) != 1 {
		t.Fatalf("swings = %d, want one at the character", len(swings))
	}

	done := make(chan struct{})
	if !f.Queue().Post(func() {
		defer close(done)
		n := script.NPCOf(f)
		n.RemoveAllAttackDesire()
		n.StopAttack()
	}) {
		t.Fatal("the npc's queue is closed")
	}
	<-done
	srv.Settle(t)
	if frames := srv.ReadQueued(t, srv.Client); len(frames) != 0 {
		t.Fatalf("the stop showed %d frames, want none", len(frames))
	}

	srv.Advance(t, 3*time.Second)
	if got := vitals.CurrentHP(); got != hpBefore {
		t.Fatalf("character HP = %d after the stopped swing, want %d", got, hpBefore)
	}
	if swings := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeAttack, f.ObjectID()); len(swings) != 0 {
		t.Fatalf("%d swings after the stop, want none", len(swings))
	}
}
