package siegeguard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// SwordGuard is GludioSwordGuard: a GludioHold guard that attacks a clan
// leader whose Seal of Ruler lands on it, its castle's own included.
func SwordGuard() script.Script {
	return script.Script{Dir: holdDir, Behavior: true, NPCs: swordGuardIDs(), Hooks: swordGuardHooks()}
}

// swordGuardHooks are GludioSwordGuard's hooks, which GludioSwordGuardUseSkill
// derives its own from.
func swordGuardHooks() script.Hooks {
	base := holdHooks()
	return base.With(script.Hooks{
		OnSpelled: func(_ *script.Script, e script.Spelled) {
			if e.Skill == sealOfRuler() {
				e.NPC.AddAttackDesire(e.Caster, 50000)
			}
		},
	}).Named("GludioSwordGuard")
}

// swordGuardIDs are GludioSwordGuard's own NPCs.
func swordGuardIDs() []int32 {
	return []int32{
		31400, 35015, 35025, 35035, 35045, 35055, 35071, 35074, 35077, 35113, 35116, 35119, 35155, 35158,
		35161, 35197, 35200, 35203, 35240, 35243, 35246, 35287, 35290, 35293, 35331, 35334, 35337, 35476,
		35479, 35482, 35523, 35526, 35529,
	}
}

// SwordGuardUseSkill is GludioSwordGuardUseSkill: a sword guard that, hit
// or called by a clan mate, sometimes uses its special attack on the one it
// fights and dispels itself when the attacker comes close, and that
// attacks whoever it sees outside its castle's side, now and then shouting
// at them.
func SwordGuardUseSkill() script.Script {
	base := swordGuardHooks()
	return script.Script{Dir: holdDir, Behavior: true, NPCs: []int32{35060}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if top := n.TopDesireTarget(); top != nil && !spared(s, n, e.Attacker) {
				if script.Same(e.Attacker, top) && s.Rnd(25) < 1 {
					if special, ok := n.SkillByType(npc.SkillTypePhysicalSpecial); ok {
						n.AddCastDesire(e.Attacker, special, 1000000)
					}
				}

				if n.Distance2D(e.Attacker) < 150 && s.Rnd(50*15) < 1 {
					if dispel, ok := n.SkillByType(npc.SkillTypeDispell); ok {
						n.AddCastDesire(n, dispel, 1000000)
					}
				}
			}

			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			n := e.Called
			if top := n.TopDesireTarget(); top != nil && !spared(s, n, e.Attacker) {
				if script.Same(e.Attacker, top) && s.Rnd(25) < 1 {
					if special, ok := n.SkillByType(npc.SkillTypePhysicalSpecial); ok {
						n.AddCastDesire(e.Attacker, special, 1000000)
					}
				}

				if n.Distance2D(e.Attacker) < 150 && s.Rnd(50*15) < 1 {
					if dispel, ok := n.SkillByType(npc.SkillTypeDispell); ok {
						n.AddCastDesire(n, dispel, 1000000)
					}
				}
			}

			base.ClanAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if !spared(s, n, e.Creature) {
				if i0 := s.Rnd(10000); i0 < 1 {
					n.Shout(script.NpcString(1800012))
				} else if i0 < 2 {
					n.Shout(script.NpcString(1800013))
				}

				n.AddAttackDesire(e.Creature, 200)
			}
		},
	})}
}
