package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const castSummonPCDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastSummonPC"

// CastSummonPC is WarriorCastSummonPC: a Warrior that, once (IntAI0), may
// want its summon magic on a player or summon hitting it: on a draw under
// 50 from farther than 300; from farther than 100, on a draw under 50 when
// it is the one it hates most, else under 10; each time only when it has
// the HP and MP for the cast and no mute keeps it from it. When any cast of
// its then finishes well, it stops what it does and pulls that attacker to
// itself, once (IntAI1). After every cast that finishes, it may want its
// physical special skill on that attacker, on a draw under 33, when it is
// the one it hates most.
//
// Its memory: Creature0 is the last player or summon that hit it.
func CastSummonPC() script.Script {
	return script.Script{Dir: castSummonPCDir, Behavior: true, NPCs: []int32{20213}, Hooks: castSummonPCHooks()}
}

// castSummonPCHooks are WarriorCastSummonPC's hooks, which its child
// derives its own from.
func castSummonPCHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				i6 := s.Rnd(100)
				n.SetSlot(script.SlotCreature0, attacker)
				if n.Int(script.SlotIntAI0) == 0 {
					if distance := n.Distance2D(attacker); distance > 300 {
						if i6 < 50 {
							castSummon(n, attacker)
						}
					} else if distance > 100 {
						if (script.Same(n.MostHated(), attacker) && i6 < 50) || i6 < 10 {
							castSummon(n, attacker)
						}
					}
				}
			}
			base.Attacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			c0 := n.Slot(script.SlotCreature0)
			if c0 != nil && e.Success && n.Int(script.SlotIntAI1) == 1 {
				n.AbortAll()
				script.TeleportTo(c0, n)
				n.SetInt(script.SlotIntAI1, 0)
			}
			if mostHated := n.MostHated(); mostHated != nil && script.Same(mostHated, c0) && s.Rnd(100) < 33 {
				addCastByType(n, c0, npc.SkillTypePhysicalSpecial, 1000000)
			}
		},
	}).Named("WarriorCastSummonPC")
}

// castSummon makes n want its summon magic on target, weight 1000000, and
// marks IntAI0 and IntAI1 that it did, when it has the HP and MP for the
// cast and no mute keeps it from it.
func castSummon(n *script.NPC, target script.Creature) {
	summonMagic, _ := n.SkillByType(npc.SkillTypeSummonMagic)
	if n.MeetsHPMPDisabledConditions(target, summonMagic) {
		n.AddCastDesire(target, summonMagic, 1000000)
		n.SetInt(script.SlotIntAI0, 1)
		n.SetInt(script.SlotIntAI1, 1)
	}
}

// CastSummonPCAggressive is WarriorCastSummonPCAggressive: a
// WarriorCastSummonPC that attacks the players and summons it sees.
func CastSummonPCAggressive() script.Script {
	base := castSummonPCHooks()
	return script.Script{Dir: castSummonPCDir, Behavior: true, NPCs: []int32{20221}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})}
}
