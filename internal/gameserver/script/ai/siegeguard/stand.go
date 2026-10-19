package siegeguard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Stand is GludioStand: a guard that keeps its post, attacking from where
// it stands the players and summons that hit it, hit a clan mate, hit its
// castle's doors, come into its sight or cast at what it fights; it spares
// its castle's defenders, and an attacker standing more than 100 above the
// one hit.
func Stand() script.Script {
	return script.Script{Dir: standDir, Behavior: true, NPCs: standIDs(), Hooks: standHooks()}
}

// standHooks are GludioStand's hooks, which GludioBowGuardStand derives its
// own from.
func standHooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if !above(e.Attacker, e.NPC) && !spared(s, e.NPC, e.Attacker) {
				e.NPC.AddAttackDesireHold(e.Attacker, damageWeight(e.NPC, int(e.Damage), 100))
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if !above(e.Attacker, e.Caller) && !spared(s, e.Called, e.Attacker) {
				e.Called.AddAttackDesireHold(e.Attacker, damageWeight(e.Called, int(e.Damage), 100))
			}
		},
		OnStaticObjectClanAttacked: func(s *script.Script, e script.StaticObjectClanAttacked) {
			switch {
			case e.Attacker.SiegeSummon():
				if s.Rnd(100) < 10 {
					e.Called.AddAttackDesireHold(script.ActingPlayer(e.Attacker), 5000)
				} else {
					e.Called.AddAttackDesireHold(e.Attacker, 1000)
				}
			case !spared(s, e.Called, e.Attacker) && e.Attacker.Playable():
				e.Called.AddAttackDesireHold(e.Attacker, damageWeight(e.Called, int(e.Damage), 50))
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if e.Creature.Playable() && !spared(s, e.NPC, e.Creature) {
				e.NPC.AddAttackDesireHold(e.Creature, 200)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n := e.NPC
			if spared(s, n, e.Caster) {
				return
			}
			k := s.Skill(e.Skill)
			if k.Offensive() && n.InCombat() && targets(e.Targets, n.TopDesireTarget()) {
				n.AddAttackDesireHold(e.Caster, damageWeight(n, k.AggroPoints(), 50))
			}
		},
	}).Named("GludioStand")
}

// standIDs are GludioStand's own NPCs.
func standIDs() []int32 {
	return []int32{
		35010, 35011, 35020, 35021, 35030, 35031, 35040, 35041, 35050, 35051, 35079, 35082, 35083, 35084,
		35086, 35087, 35089, 35090, 35121, 35124, 35125, 35126, 35128, 35129, 35131, 35132, 35163, 35166,
		35167, 35168, 35170, 35171, 35173, 35174, 35205, 35208, 35209, 35210, 35212, 35213, 35215, 35216,
		35248, 35251, 35252, 35253, 35255, 35256, 35258, 35259, 35295, 35298, 35299, 35300, 35302, 35303,
		35305, 35306, 35339, 35342, 35343, 35344, 35346, 35347, 35349, 35350, 35484, 35487, 35488, 35489,
		35491, 35492, 35494, 35495, 35531, 35534, 35535, 35536, 35538, 35539, 35541, 35542,
	}
}

// BowGuardStand is GludioBowGuardStand: a GludioStand archer that answers a
// hit or a clan mate's call from any height, and attacks a clan leader
// within 1000 whose Seal of Ruler lands on it, its castle's own included.
func BowGuardStand() script.Script {
	base := standHooks()
	return script.Script{Dir: holdDir, Behavior: true, NPCs: bowGuardStandIDs(), Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if !spared(s, e.NPC, e.Attacker) && e.Attacker.Playable() {
				e.NPC.AddAttackDesireHold(e.Attacker, damageWeight(e.NPC, int(e.Damage), 100))
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if !spared(s, e.Called, e.Attacker) {
				e.Called.AddAttackDesireHold(e.Attacker, damageWeight(e.Called, int(e.Damage), 50))
			}
		},
		OnSpelled: func(_ *script.Script, e script.Spelled) {
			if e.Skill == sealOfRuler() && e.NPC.Distance2D(e.Caster) < 1000 {
				e.NPC.AddAttackDesireHold(e.Caster, 50000)
			}
		},
	})}
}

// bowGuardStandIDs are GludioBowGuardStand's own NPCs.
func bowGuardStandIDs() []int32 {
	return []int32{
		35012, 35022, 35032, 35042, 35052, 35085, 35088, 35091, 35127, 35130, 35133, 35169, 35172, 35175,
		35211, 35214, 35217, 35254, 35257, 35260, 35301, 35304, 35307, 35345, 35348, 35351, 35490, 35493,
		35496, 35537, 35540, 35543,
	}
}
