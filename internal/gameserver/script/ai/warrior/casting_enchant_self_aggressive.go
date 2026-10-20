package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// CastingEnchantSelfAggressive is WarriorCastingEnchantSelfAggressive: a
// WarriorCastingEnchant that, settled in its territory and above half its
// hit points, may once buff itself on sight of any creature, and attacks
// the players and summons it sees.
func CastingEnchantSelfAggressive() script.Script {
	base := CastingEnchantHooks()
	return script.Script{Dir: castingEnchantDir, Behavior: true, NPCs: []int32{20260, 20424, 20088, 20620, 20624, 20642, 20234, 20098}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if n.LifeTime() > 7 && n.InMyTerritory() {
				if n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 && n.HPRatio() > 0.5 {
					addCastByType(n, n, npc.SkillTypeBuff, 1000000)
				}
				// Any settled sighting spends the buff, whether or not it was cast.
				n.SetInt(script.SlotIntAI1, 1)
			}
			// A creature other than a player or summon reaches no parent.
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})}
}
