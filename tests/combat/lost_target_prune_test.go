package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFollowDesireOnATeleportedPlayerIsPruned pins the prune at the start of
// desire selection (NpcAI.runAI drops every desire whose final target the NPC
// no longer knows): a monster following a player who teleports out of its
// known list drops the follow desire on its next cycle and takes up the
// next desire, a walk.
func TestFollowDesireOnATeleportedPlayerIsPruned(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID).(attackable.Combatant)
	px, py, pz := srv.PlayerPosition(t, objID)
	// The teleport leaves the monster's region without a player; a monster
	// that does not sleep then keeps its AI running.
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.NoSleepMode = true
	at := location.Location{X: px + 150, Y: py, Z: pz}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
	drainUntilQuiet(t, srv.Client)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	x, y, z := hostile.Position()
	walk := location.Location{X: x + 200, Y: y, Z: z}

	hostile.AI().AddFollowDesire(victim, 100)
	if !hostile.AI().AddMoveToDesire(walk, 1) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionFollow {
		t.Fatalf("CurrentIntention() with the follow queued = %v, want %v", got, ai.IntentionFollow)
	}

	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.TeleportTo(px+3*2048, py, pz, 0) })
	srv.Settle(t)
	if hostile.Knows(victim) {
		t.Fatal("the monster still knows the player after the teleport, want it forgotten")
	}
	follow := &ai.Desire{Kind: ai.IntentionFollow, FinalTarget: victim}
	if !hostile.AI().Desires().Has(follow) {
		t.Fatal("follow desire dropped before the next cycle, want the cycle's prune to drop it")
	}

	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if hostile.AI().Desires().Has(follow) {
		t.Fatal("follow desire on the teleported player still queued after the next cycle")
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the next cycle = %v, want the walk %v", got, ai.IntentionMoveTo)
	}
}
