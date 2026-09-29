package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestArrivalLeavesQueuedAttackForNextRunAI pins that the THINK an arrival
// runs only continues the current intention: a monster that gets an attack
// desire while it walks a MOVE_TO does not take the attack up when the walk
// arrives. It sends no Attack, MoveToPawn or chase walk on the arrival and
// starts the attack on the next desire selection.
func TestArrivalLeavesQueuedAttackForNextRunAI(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID).(attackable.Combatant)

	x, y, z := srv.PlayerPosition(t, objID)
	at := location.Location{X: x + 400, Y: y, Z: z}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, gameservertest.MovingHostileTemplate("Monster"), at, at)
	drainUntilQuiet(t, c)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	walkInPlace(t, srv, hostile)
	drainUntilQuiet(t, c)

	dest := location.Location{X: at.X, Y: at.Y + 100, Z: at.Z}
	if !hostile.AI().AddMoveToDesire(dest, 50) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("walk TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the walk desire = %v, want %v", got, ai.IntentionMoveTo)
	}
	readUntil(t, c, serverpackets.OpcodeMoveToLocation, "MoveToLocation for the walk")

	// Hate first, so the attack desire is not the first one and does not
	// run desire selection itself: it waits in the queue behind the walk.
	hostile.AddDamageHate(victim, 0, 100)
	hostile.AddAttackDesire(victim, 5)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the queued attack desire = %v, want %v", got, ai.IntentionMoveTo)
	}

	for i := 0; hostile.Move().Moving(); i++ {
		if i >= int(5*time.Second/move.PositionUpdateInterval) {
			t.Fatal("walk never arrived")
		}
		srv.TickPositions()
	}

	if got := hostile.AI().CurrentIntention(); got == ai.IntentionAttack {
		t.Fatal("arrival took up the queued attack, want it left for the next runAI")
	}
	if got := hostile.Move().FollowMode(); got != move.FollowNone {
		t.Fatalf("FollowMode() after arrival = %v, want none", got)
	}
	if !hostile.AI().Desires().Has(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: victim}) {
		t.Fatal("attack desire dropped on arrival, want it still queued")
	}
	for {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			break
		}
		switch frame[0] {
		case serverpackets.OpcodeAttack, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeMoveToLocation:
			t.Fatalf("arrival sent opcode %#x, want no attack or chase before the next runAI", frame[0])
		}
	}

	if err := hostile.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, ai.IntentionAttack)
	}
	if got := hostile.Move().FollowMode(); got != move.FollowOffensive {
		t.Fatalf("FollowMode() after RunAI = %v, want the chase started", got)
	}
}
