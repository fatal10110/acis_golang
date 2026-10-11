package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PhysicalSpecial is WarriorPhysicalSpecial: a Warrior that may strike the
// player it hates most, or a clan member's attacker, with its physical
// special skill.
func PhysicalSpecial() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/WarriorPhysicalSpecial", Behavior: true, NPCs: physicalSpecialIDs(), Hooks: PhysicalSpecialHooks()}
}

// physicalSpecialIDs are WarriorPhysicalSpecial's own NPCs.
func physicalSpecialIDs() []int32 {
	return []int32{
		18337, 20606, 20607, 20613, 20932, 21016, 21135, 21166, 21169, 21172, 21175, 21178, 21181, 21184,
		21187, 21190, 21193, 21196, 21199, 21202, 21205, 21278, 21279, 21280, 21288, 21659, 21682, 21705,
		21728, 21751, 21774, 21431, 20199, 20084, 21286, 21397, 20246, 20238, 21104, 20242, 20567, 20667,
		21648, 21411, 21798, 21112, 21109, 20641, 20662, 20661, 20241, 21415, 20600, 21274, 21275, 21276,
		20604, 20654, 20563, 20230, 20836, 20788, 20999, 20240, 20666, 20665, 20572, 21294, 20684, 21261,
		21262, 21263, 21264, 21282, 21283, 21290, 21292, 21352, 21356, 21359, 21363, 21367, 22002, 22005,
		22007, 22008, 22020, 22025, 22048,
	}
}

// castPhysicalSpecial makes n want to cast its template's physical special
// skill on target; a template that lists none casts nothing.
func castPhysicalSpecial(n *script.NPC, target script.Creature) {
	if ref, ok := n.SkillByType(npc.SkillTypePhysicalSpecial); ok {
		n.AddCastDesire(target, ref, 1000000)
	}
}

// PhysicalSpecialHooks are WarriorPhysicalSpecial's hooks, which its
// children derive theirs from.
func PhysicalSpecialHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if e.Attacker.Playable() {
				if script.Same(e.NPC.MostHated(), e.Attacker) && s.Rnd(100) < 33 {
					castPhysicalSpecial(e.NPC, e.Attacker)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack && s.Rnd(100) < 33 {
				castPhysicalSpecial(called, e.Attacker)
			}
			base.ClanAttacked(s, e)
		},
	})
}
