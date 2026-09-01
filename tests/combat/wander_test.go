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
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", 20001, home, home)
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

// TestGuardDoesNotIdleWander pins an unregistered Guard template: an empty
// desire queue stays idle instead of rolling a random walk.
func TestGuardDoesNotIdleWander(t *testing.T) {
	assertIdleNoWander(t, "Guard", 100)
}

// TestGuardMoveAroundIdleWanders pins opt-in Guard templates: 31845 queues
// wander the same way a field monster does.
func TestGuardMoveAroundIdleWanders(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Guard", 31845, home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander", got)
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
}

func TestWarriorHoldDoesNotIdleWander(t *testing.T) {
	assertIdleNoWander(t, "Monster", 27102)
}

func TestUnscriptedSquashDoesNotIdleWander(t *testing.T) {
	assertIdleNoWander(t, "Monster", 12774)
}

func TestNurseAntIdleWanderTimer(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", 29003, home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	d, ok := hostile.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionWander {
		t.Fatalf("Peek() = (%v, %v), want wander desire", d, ok)
	}
	if d.Timer != 40 || d.Weight != 20 {
		t.Fatalf("wander timer/weight = %d/%v, want 40/20", d.Timer, d.Weight)
	}
}

func assertIdleNoWander(t *testing.T, kind string, npcID int) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, kind, npcID, home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
	if hostile.IsMoving() {
		t.Fatal("IsMoving() = true, want no wander")
	}
}
