package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestIdleHostileWanderBroadcastsWalkThenMove pins AttackableAI.thinkWander's
// first idle step: walk stance, then a MoveToLocation away from the spawn
// home (offset = walk speed * 3).
func TestIdleHostileWanderBroadcastsWalkThenMove(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander", got)
	}

	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	move := mustRead(t, c, "MoveToLocation")
	assertFrameOpcode(t, move, serverpackets.OpcodeMoveToLocation, "MoveToLocation")
	if !hostile.IsMoving() {
		t.Fatal("IsMoving() = false after wander Think, want a live walk")
	}
}

// TestGuardDoesNotIdleWander pins hold-position kinds: a Guard with an empty
// desire queue stays idle instead of rolling a random walk.
func TestGuardDoesNotIdleWander(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Guard", home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
	if hostile.IsMoving() {
		t.Fatal("IsMoving() = true for idle Guard, want no wander")
	}
}
