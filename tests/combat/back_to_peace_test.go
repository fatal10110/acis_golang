package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestHateRunningOutKeepsTheAttackDesire pins the return to peace
// (NpcAI.setBackToPeace clears the aggro and hate lists only): a monster
// whose hate on its target runs out while its attack desire still has
// weight swings at it again. Each AI cycle selects before its periodic
// refresh (NpcAI.runAI), so the cycle whose refresh finds the aggro list
// empty (AggroList.refresh) and drops the attack desire still keeps the
// attack, and only the cycle after it idles the monster.
func TestHateRunningOutKeepsTheAttackDesire(t *testing.T) {
	t.Parallel()
	srv, hostile, victim, attackTime := startHostileAttack(t, 300, 0)
	hostile.AddAttackDesire(victim, 100)
	hostile.ReduceAllAggroHate(205)

	if _, ok := hostile.AI().Threats().MostHated(); ok || !hostile.AI().Threats().IsEmpty() {
		t.Fatal("threat table not cleared once the hate ran out")
	}
	if !hostile.AI().Desires().Has(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: victim}) {
		t.Fatal("attack desire dropped by the return to peace, want it kept")
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the return to peace = %v, want %v", got, ai.IntentionAttack)
	}

	srv.Advance(t, attackTime+10*time.Millisecond)
	readUntil(t, srv.Client, serverpackets.OpcodeAttack, "the swing after the hate ran out")

	// One AI cycle as the AI task runs it: Tick, then TickThink.
	cycle := func() {
		t.Helper()
		hostile.Tick()
		if err := hostile.TickThink(); err != nil {
			t.Fatalf("TickThink() error: %v", err)
		}
	}
	for n := 1; hostile.AI().Desires().Len() != 0; n++ {
		if n > 3 {
			t.Fatal("attack desire still queued after three AI cycles, want the periodic refresh to drop it")
		}
		cycle()
		if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
			t.Fatalf("CurrentIntention() after AI cycle %d = %v, want %v until the cycle after the refresh", n, got, ai.IntentionAttack)
		}
	}
	cycle()
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() in the AI cycle after the refresh = %v, want %v", got, ai.IntentionIdle)
	}
}

// TestScriptDesireSurvivesTheReturnToPeace pins that a desire a behavior
// queued, here a flee, outlives the return to peace that stopping the last
// hate causes, and is taken up once the swing's hit animation ends.
func TestScriptDesireSurvivesTheReturnToPeace(t *testing.T) {
	t.Parallel()
	srv, hostile, victim, _ := startHostileAttack(t, 300, 0)
	script.NewNPC(hostile).AddFleeDesire(script.PlayerOf(victim), 300, 1)
	flee := &ai.Desire{Kind: ai.IntentionFlee, FinalTarget: victim}
	if !hostile.AI().Desires().Has(flee) {
		t.Fatal("flee desire not queued")
	}

	hostile.StopAggroHate(victim)

	if !hostile.AI().Threats().IsEmpty() {
		t.Fatal("threat table not cleared by the return to peace")
	}
	if !hostile.AI().Desires().Has(flee) {
		t.Fatal("flee desire dropped by the return to peace, want it kept")
	}
	srv.AdvanceUntil(t, "the flee taken up after the swing's hit animation", func() bool {
		return hostile.AI().CurrentIntention() == ai.IntentionFlee
	})
}

// TestInactiveRegionLeavesTheWalkAlone pins Npc.onInactiveRegion's return to
// peace on a walking monster: the region going to sleep clears its hate
// lists only, so it walks on with its walk desire queued.
func TestInactiveRegionLeavesTheWalkAlone(t *testing.T) {
	t.Parallel()
	srv, hostile, _, _ := scriptDesireMonster(t, 150, 0)
	x, y, z := hostile.Position()
	far := location.Location{X: x + 4000, Y: y, Z: z}
	if !hostile.AI().AddMoveToDesire(far, 1000) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if dest, moving := hostile.MovingTo(); !moving || dest != far {
		t.Fatalf("monster moving to %v (%v), want walking to %v", dest, moving, far)
	}

	srv.Client.Send(encodeLogout())
	srv.AdvanceUntil(t, "the region going to sleep", func() bool {
		_, active := srv.State.RegionActivity(hostile)
		return !active
	})
	// The AI tick finds the region asleep and applies its reset, if the
	// player's departure has not already.
	hostile.Tick()
	srv.Settle(t)

	if dest, moving := hostile.MovingTo(); !moving || dest != far {
		t.Fatalf("monster moving to %v (%v) after the region slept, want still walking to %v", dest, moving, far)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the region slept = %v, want %v", got, ai.IntentionMoveTo)
	}
	if !hostile.AI().Desires().Has(&ai.Desire{Kind: ai.IntentionMoveTo, Location: far}) {
		t.Fatal("walk desire dropped when the region slept, want it kept")
	}
}
