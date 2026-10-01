package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target/targettest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// withoutForceCaster is a player that may attack whatever lets it.
type withoutForceCaster struct {
	world.Presence
	targettest.Actor
}

func (*withoutForceCaster) ObjectID() int32           { return 7 }
func (*withoutForceCaster) Kind() actor.Kind          { return actor.KindPlayer }
func (*withoutForceCaster) Heading() int              { return 0 }
func (*withoutForceCaster) Position() (int, int, int) { return 0, 0, 0 }

// TestHostileAttackableWithoutForceOnlyMonsterFamily pins which hostile NPCs
// a player attacks without Ctrl: every Monster-family kind, while it lives,
// and never a town Guard, a FriendlyMonster or a SiegeGuard outside a siege.
func TestHostileAttackableWithoutForceOnlyMonsterFamily(t *testing.T) {
	t.Parallel()
	caster := &withoutForceCaster{}
	for kind := range hostileInstanceKinds {
		_, monster := monsterInstanceKinds[kind]
		h, err := NewHostile(&Instance{ObjectID: 101, Template: &Template{ID: 9001, Type: string(kind)}, Kind: kind}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
		if err != nil {
			t.Fatal(err)
		}
		if got := h.AttackableWithoutForceBy(caster); got != monster {
			t.Errorf("%s AttackableWithoutForceBy = %v, want %v", kind, got, monster)
		}
		if !h.AttackableBy(caster) {
			t.Errorf("%s AttackableBy = false, want true", kind)
		}
	}
}
