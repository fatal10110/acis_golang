package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestDamageDeniedAutoAttackDealsNoDamage has a player whose access level
// forbids dealing damage auto-attack a monster with every swing hitting.
// Each hit works out to 0 damage, so the Attack packets carry 0, the
// attacker never enters its attack stance, the monster neither enters its
// own nor hates the attacker, and it keeps its HP.
func TestDamageDeniedAutoAttackDealsNoDamage(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(time.Now),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	obj.(interface{ SetCanGiveDamage(bool) }).SetCanGiveDamage(false)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	setPlayerRoll(t, srv, objID, 0)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	frames := [][]byte{assertAttackBy(t, c, objID)}
	srv.Advance(t, 3*time.Second)
	frames = append(frames, readQuiet(c)...)

	swings := 0
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeAttack {
			continue
		}
		r := wireReader(frame[1:])
		if r.ReadInt32() != objID {
			continue
		}
		swings++
		target, damage, flags := r.ReadInt32(), r.ReadInt32(), r.ReadUint8()
		if target != hostile.ObjectID() || damage != 0 || flags&serverpackets.AttackHitMiss != 0 {
			t.Fatalf("Attack hit = target %d damage %d flags %#x, want a 0-damage hit on %d", target, damage, flags, hostile.ObjectID())
		}
	}
	if swings < 2 {
		t.Fatalf("swings = %d, want at least 2", swings)
	}
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, objID); n != 0 {
		t.Fatalf("attacker AutoAttackStart after 0-damage hits = %d, want 0", n)
	}
	if n := countByID(frames, serverpackets.OpcodeAutoAttackStart, hostile.ObjectID()); n != 0 {
		t.Fatalf("monster AutoAttackStart after 0-damage hits = %d, want 0", n)
	}
	if srv.AttackStance.InAttackStance(worldActor{id: objID}) || srv.AttackStance.InAttackStance(worldActor{id: hostile.ObjectID()}) {
		t.Fatal("0-damage hits put a fighter in the stance tracker")
	}
	if hate := hostile.AggroHate(obj.(attackable.Combatant)); hate != 0 {
		t.Fatalf("monster hate for the damage-denied attacker = %v, want 0", hate)
	}
	if got := hostile.CurrentHP(); got != hostile.MaxHP() {
		t.Fatalf("monster HP after 0-damage hits = %d, want full %d", got, hostile.MaxHP())
	}
}
