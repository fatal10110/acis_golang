package hall

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartisanSorcerer1 is Agit01PartisanSorcerer1: a Partisan Hideaway
// sorcerer that casts skill 4043 from where it stands at every player and
// summon it sees, and on a draw under 10 at one attacking a clan mate.
func PartisanSorcerer1() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35373}, Hooks: base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if e.Attacker.Playable() && s.Rnd(100) < 10 {
				e.Called.AddCastDesireHold(e.Attacker, level1(4043), 1000000)
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			e.NPC.AddCastDesireHold(e.Creature, level1(4043), 1000000)
		},
	})}
}
