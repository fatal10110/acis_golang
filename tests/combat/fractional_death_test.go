package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Death is a half-point threshold on every damage route: the remainder of a
// blow is dead below 0.5 HP and alive from 0.5 up. Fractional current HP is
// ordinary (regeneration and heals write fractions straight into it), while
// the client only ever sees it truncated.
const (
	remainderBelowHalfPoint = 0.4
	remainderAtHalfPoint    = 0.5
)

// TestMeleeLeavingRemainderBelowHalfPointKillsPlayer drives a real NPC
// auto-attack against a player whose HP is the hit's landed damage plus a
// fractional remainder. A remainder under half a point is a death, not a
// survival at a displayed 0 HP.
func TestMeleeLeavingRemainderBelowHalfPointKillsPlayer(t *testing.T) {
	t.Parallel()
	meleeOnRemainder(t, remainderBelowHalfPoint, true)
}

// TestMeleeLeavingRemainderAtHalfPointSparesPlayer is the boundary: exactly
// half a point survives, at a displayed 0 HP.
func TestMeleeLeavingRemainderAtHalfPointSparesPlayer(t *testing.T) {
	t.Parallel()
	meleeOnRemainder(t, remainderAtHalfPoint, false)
}

// meleeOnRemainder swings once to learn the fixture attacker's landed damage,
// puts the player on exactly that damage plus remainder, and swings again.
func meleeOnRemainder(t *testing.T, remainder float64, wantDead bool) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	attacker.DoAttack(t, victim.(attackable.Combatant))
	damage := int(assertLandedDamagingHit(t, assertAttackBy(t, c, attacker.ObjectID()), objID))
	if srv.PlayerDead(t, objID) {
		t.Fatalf("control hit of %d killed the player", damage)
	}
	drainUntilQuiet(t, c)

	// The deterministic roll source makes every swing land the same damage.
	// Land the player on damage+remainder: the packet surface truncates HP, so
	// prove the remainder was placed rather than clamped away.
	if hp := srv.PlayerCurrentHP(t, objID); hp > damage {
		srv.DamagePlayerHP(t, objID, hp-damage)
	} else if hp < damage {
		srv.AddPlayerHP(t, objID, float64(damage-hp))
	}
	if added := srv.AddPlayerHP(t, objID, remainder); added != remainder {
		t.Fatalf("remainder added to player HP = %v, want %v", added, remainder)
	}

	attacker.DoAttack(t, victim.(attackable.Combatant))
	if got := int(assertLandedDamagingHit(t, assertAttackBy(t, c, attacker.ObjectID()), objID)); got != damage {
		t.Fatalf("second swing damage = %d, want the control hit's %d", got, damage)
	}
	if dead := srv.PlayerDead(t, objID); dead != wantDead {
		t.Fatalf("player dead after a hit leaving %v HP = %v, want %v", remainder, dead, wantDead)
	}
	if hp := srv.PlayerCurrentHP(t, objID); hp != 0 {
		t.Fatalf("player HP after a hit leaving %v HP = %d, want 0", remainder, hp)
	}
}

// TestSkillDamageOnMonsterBelowHalfPointKillsIt covers the creature-level
// primitive every NPC damage route shares: a monster on a fractional
// remainder under half a point dies, at or above it survives.
func TestSkillDamageOnMonsterBelowHalfPointKillsIt(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		remainder float64
		wantDead  bool
	}{
		{"below half point", remainderBelowHalfPoint, true},
		{"at half point", remainderAtHalfPoint, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			startInWorld(t, srv.Client)
			objID := srv.SoleObjectID(t)
			player := livePlayer(t, srv, objID).(attackable.Combatant)
			hostile := srv.SpawnHostileNPC(t)
			drainUntilQuiet(t, srv.Client)

			const damage = 10
			done := make(chan struct{})
			if !hostile.Queue().Post(func() {
				defer close(done)
				hostile.SetHP(damage + test.remainder)
				hostile.ReduceHP(damage, player, modelskill.Definition{})
			}) {
				t.Fatal("post to monster queue: queue closed")
			}
			<-done
			if dead := hostile.AlikeDead(); dead != test.wantDead {
				t.Fatalf("monster dead after a hit leaving %v HP = %v, want %v", test.remainder, dead, test.wantDead)
			}
		})
	}
}
