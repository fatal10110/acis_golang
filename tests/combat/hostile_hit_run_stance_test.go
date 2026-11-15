package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestHitWalkingMonsterStartsRunning pins the hit's stance switch: a hit
// puts a walking monster in run stance, shown with ChangeMoveType then its
// NpcInfo; a second hit changes nothing.
func TestHitWalkingMonsterStartsRunning(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	attacker := livePlayer(t, srv, objID).(attackable.Combatant)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	hostile.SetRunning(false)
	drainUntilQuiet(t, c)

	onHostileQueue(t, hostile, func() { hostile.TakeDamage(10, attacker) })
	srv.Settle(t)
	frames := readQuiet(c)
	id := hostile.ObjectID()
	moveType := indexOf(frames, 0, serverpackets.OpcodeChangeMoveType, id)
	if moveType < 0 {
		t.Fatalf("hit walking monster sent no ChangeMoveType; frames %d", len(frames))
	}
	assertChangeMoveType(t, frames[moveType], id, true)
	info := indexOf(frames, moveType, serverpackets.OpcodeNPCInfo, id)
	if info < 0 {
		t.Fatalf("ChangeMoveType at %d with no NpcInfo after it", moveType)
	}
	if !hostile.Running() {
		t.Fatal("hit monster still walking")
	}

	onHostileQueue(t, hostile, func() { hostile.TakeDamage(10, attacker) })
	srv.Settle(t)
	if n := countByID(readQuiet(c), serverpackets.OpcodeChangeMoveType, id); n != 0 {
		t.Fatalf("second hit sent %d ChangeMoveType, want 0", n)
	}
}

// TestReturnHomeDropsQueuedAttack pins return home's hate clean-up for the
// generic and the siege-guard walk back: the threat entries keep no hate,
// no ATTACK desire stays queued, and the next AI cycle takes up no attack.
func TestReturnHomeDropsQueuedAttack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind  string
		drift int
	}{
		{"Monster", 500},
		{"SiegeGuard", 50},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			victim := livePlayer(t, srv, objID).(attackable.Combatant)

			home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
			hostile := srv.SpawnMovingHostileNPCAt(t, tc.kind, home, home)
			drainUntilQuiet(t, c)
			hostile.AddDamageHate(victim, 10, 50)
			hostile.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: victim, Weight: 50})

			hostile.SetXYZ(hostileX, hostileY+tc.drift, hostileZ)
			if !hostile.ReturnHome() {
				t.Fatal("ReturnHome() = false, want true outside drift range")
			}
			if hostile.AI().Desires().HasKind(ai.IntentionAttack) {
				t.Fatal("ATTACK desire still queued after return home")
			}
			if hate := hostile.AI().ThreatsIfBuilt().Hate(victim); hate != 0 {
				t.Fatalf("attacker hate after return home = %v, want 0", hate)
			}
			if err := hostile.TickThink(); err != nil {
				t.Fatalf("TickThink() error: %v", err)
			}
			if got := hostile.AI().CurrentIntention(); got == ai.IntentionAttack {
				t.Fatal("AI cycle after return home took up the attack")
			}
		})
	}
}
