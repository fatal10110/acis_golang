package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// enchant1Of4Buffs are the self buffs a WarriorCastEnchant1Of4 picks one
// of at its creation.
func enchant1Of4Buffs() []npc.SkillType {
	return []npc.SkillType{npc.SkillTypeSelfBuff1, npc.SkillTypeSelfBuff2, npc.SkillTypeSelfBuff3, npc.SkillTypeSelfBuff4}
}

// enchant1Of4 makes n, the first time it is called, want to cast on itself
// on a 33 in 100 draw, while above half of its hit points, the self buff
// it picked at its creation.
func enchant1Of4(s *script.Script, n *script.NPC) {
	if n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 && n.HPRatio() > 0.5 {
		castByType(n, n, enchant1Of4Buffs()[n.Int(script.SlotIntAI0)])
	}
	n.SetInt(script.SlotIntAI1, 1)
}

// CastEnchant1Of4 is WarriorCastEnchant1Of4: a Warrior that picks one of
// its four self buffs at its creation and may cast it on itself when a
// player or summon first hits it.
//
// Its memory: IntAI0 is the buff it picked, IntAI1 is set once it had its
// chance to cast it.
func CastEnchant1Of4() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{22009}, Hooks: castEnchant1Of4Hooks()}
}

// castEnchant1Of4Hooks are WarriorCastEnchant1Of4's hooks, which its
// child derives its own from.
func castEnchant1Of4Hooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, int32(s.Rnd(len(enchant1Of4Buffs()))))
			n.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if e.Attacker.Playable() {
				enchant1Of4(s, e.NPC)
			}
			base.Attacked(s, e)
		},
	}).Named("WarriorCastEnchant1Of4")
}

// CastEnchant1Of4Aggressive is WarriorCastEnchant1Of4Aggressive: a
// WarriorCastEnchant1Of4 that attacks the players and summons it sees and
// has its chance at its buff on the first creature it sees in its
// territory once settled.
func CastEnchant1Of4Aggressive() script.Script {
	base := castEnchant1Of4Hooks()
	hooks := base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if n.LifeTime() > 7 && n.InMyTerritory() {
				enchant1Of4(s, n)
			}
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, e.Creature)
			base.SeeCreature(s, e)
		},
	})
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21426, 20554, 21304, 20588}, Hooks: hooks}
}
