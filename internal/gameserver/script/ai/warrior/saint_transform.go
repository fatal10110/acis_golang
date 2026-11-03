package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// SaintTransform is WarriorSaintTransform: a WarriorPhysicalSpecial that,
// hit by a party member, may blast around itself, and hit by a player
// while nearly unhurt, or again between 40 and 50 percent of its HP, wants
// to dispel the player it hates most, or the attacker when it hates no
// one, falling back to attacking that target when its HP or MP cannot pay
// for the dispel.
//
// Its memory: IntAI0 is 0 until its first dispel try or until a blast it
// could not pay for, 1 after either, and 2 after its dispel try below half
// its HP.
func SaintTransform() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{21534, 21528, 21522, 21538}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if p := script.ActingPlayer(attacker); p != nil && p.InParty() && s.Rnd(100) < 33 {
				blast, _ := n.SkillByType(npc.SkillTypeSelfRangeDDMagic)
				if n.MeetsHPMPConditions(n, blast) {
					n.AddCastDesire(n, blast, 1000000)
				} else {
					n.SetInt(script.SlotIntAI0, 1)
				}
			}

			if attacker.Playable() {
				target := n.MostHated()
				if target == nil {
					target = attacker
				}
				hpRatio := n.HPRatio()
				if n.Int(script.SlotIntAI0) == 0 && hpRatio > 0.9 && s.Rnd(100) < 90 {
					dispelOrAttack(n, target)
					n.SetInt(script.SlotIntAI0, 1)
				} else if n.Int(script.SlotIntAI0) < 1 && hpRatio > 0.4 && hpRatio < 0.5 && s.Rnd(100) < 80 {
					dispelOrAttack(n, target)
					n.SetInt(script.SlotIntAI0, 2)
				}
			}
			base.Attacked(s, e)
		},
	})}
}

// dispelOrAttack makes n want to cast its template's dispel skill on
// target, or to attack target with weight 1000 when n cannot pay for the
// dispel or its template lists none.
func dispelOrAttack(n *script.NPC, target script.Creature) {
	dispel, _ := n.SkillByType(npc.SkillTypeDispell)
	if n.MeetsHPMPConditions(target, dispel) {
		n.AddCastDesire(target, dispel, 1000000)
		return
	}
	n.AddAttackDesire(target, 1000)
}
