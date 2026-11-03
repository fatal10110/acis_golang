package siegeguard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Wizard is GludioWizard: a wizard that walks up to cast its DD magic at
// the players and summons it sees, and sometimes at one attacking a clan
// mate; it spares its castle's defenders. Seeing anyone while it stands in
// a peace zone, it is sent back to its post, dropping its attacks.
func Wizard() script.Script {
	return script.Script{Dir: wizardDir, Behavior: true, NPCs: wizardIDs(), Hooks: wizardHooks()}
}

// wizardHooks are GludioWizard's hooks, which GludioWizardUseSkill derives
// its own from.
func wizardHooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if !spared(s, n, e.Creature) && e.Creature.Playable() {
				if dd, ok := n.SkillByType(npc.SkillTypeDDMagic); ok {
					n.AddCastDesire(e.Creature, dd, 1000000)
				}
			}

			if n.InsideZone(script.ZonePeace) {
				n.TeleportTo(n.SpawnLocation())
				n.RemoveAllAttackDesire()
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if !spared(s, e.Called, e.Attacker) && e.Attacker.Playable() && s.Rnd(100) < 10 {
				if dd, ok := e.Called.SkillByType(npc.SkillTypeDDMagic); ok {
					e.Called.AddCastDesire(e.Attacker, dd, 1000000)
				}
			}
		},
	}).Named("GludioWizard")
}

// wizardIDs are GludioWizard's own NPCs.
func wizardIDs() []int32 {
	return []int32{35019, 35029, 35039, 35049, 35059, 35067, 35109, 35151, 35193, 35236, 35283, 35327, 35472, 35519}
}

// WizardUseSkill is GludioWizardUseSkill: a wizard that attacks the
// players and summons it sees and casts its DD magic at them, now and then
// shouting at them, and that answers a hit or a clan mate's call with its
// DD magic or its hold magic; it spares its castle's defenders.
func WizardUseSkill() script.Script {
	base := wizardHooks()
	return script.Script{Dir: wizardDir, Behavior: true, NPCs: []int32{35061}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if !spared(s, n, e.Creature) && e.Creature.Playable() {
				if i0 := s.Rnd(10000); i0 < 1 {
					n.Shout(script.NpcString(1800012))
				} else if i0 < 2 {
					n.Shout(script.NpcString(1800013))
				}

				n.AddAttackDesire(e.Creature, 200)
				if dd, ok := n.SkillByType(npc.SkillTypeDDMagic); ok {
					n.AddCastDesire(e.Creature, dd, 1000000)
				}
			}

			if n.InsideZone(script.ZonePeace) {
				n.TeleportTo(n.SpawnLocation())
				n.RemoveAllAttackDesire()
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			n := e.Called
			if !spared(s, n, e.Attacker) && e.Attacker.Playable() {
				if s.Rnd(100) < 50 {
					if dd, ok := n.SkillByType(npc.SkillTypeDDMagic); ok {
						n.AddCastDesire(e.Attacker, dd, 1000000)
					}
				}
				if s.Rnd(100) < 25 {
					if hold, ok := n.SkillByType(npc.SkillTypeHoldMagic); ok {
						n.AddCastDesire(e.Attacker, hold, 1000000)
					}
				}
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if !spared(s, n, e.Attacker) && e.Attacker.Playable() {
				if s.Rnd(100) < 50 {
					if dd, ok := n.SkillByType(npc.SkillTypeDDMagic); ok {
						n.AddCastDesire(e.Attacker, dd, 1000000)
					}
				}
				if s.Rnd(100) < 25 {
					if hold, ok := n.SkillByType(npc.SkillTypeHoldMagic); ok {
						n.AddCastDesire(e.Attacker, hold, 1000000)
					}
				}
			}
		},
	})}
}
