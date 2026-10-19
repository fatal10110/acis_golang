package warrior

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// CastingCurse is WarriorCastingCurse: a Warrior that may lay its debuff
// on its most hated attacker and on a clan member's attacker, and once
// hit, every 5 s while it fights, on the creature it hates most.
func CastingCurse() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/WarriorCastingCurse", Behavior: true, NPCs: castingCurseIDs(), Hooks: CastingCurseHooks()}
}

// castingCurseIDs are WarriorCastingCurse's own NPCs.
func castingCurseIDs() []int32 {
	return []int32{
		20075, 20261, 20506, 20541, 20850, 21095, 21669, 21692, 21715, 21738, 21761, 21784, 20082, 20795,
		20838, 20204, 20228, 20998, 20776, 21007, 20800, 20555, 20646, 20231, 20478, 27120, 20679, 20926,
		20038, 20558, 20033, 27119, 20677, 22051,
	}
}

// CastingCurseHooks are WarriorCastingCurse's hooks, which its children
// derive theirs from.
func CastingCurseHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if !fighting(n.Intention()) {
					n.SetInt(script.SlotIntAI1, 0)
					n.SetInt(script.SlotIntAI2, 0)
					return base.Timer(s, e)
				}
				fightTick(s, n, npc.SkillTypeDebuff)
			}
			return base.Timer(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			mostHated := n.MostHated()
			armFightTimer(s, n, attacker, mostHated)
			if attacker.Playable() && script.Same(mostHated, attacker) {
				maybeDebuff(s, n, attacker, npc.SkillTypeDebuff)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if e.Attacker.Playable() && e.Called.LifeTime() > 7 {
				maybeDebuff(s, e.Called, e.Attacker, npc.SkillTypeDebuff)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCastingCurse")
}
