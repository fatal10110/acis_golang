package hall

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The page directories of the Partisan Hideaway's soldiers and healers.
const (
	move1Dir  = "ai/siegeablehall/Agit01Move1"
	healerDir = "ai/siegeablehall/Agit01PartisanHealer1"
)

// move1Hooks are Agit01Move1's hooks, which the Partisan sorcerer derives
// its own from: the soldier fights the playables it sees, at weight 200,
// those that hit it or a clan mate, by the damage, and those whose spells
// it sees, by the spell's aggro.
func move1Hooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Attacker.Playable() {
				e.NPC.AddAttackDesire(e.Attacker, damageWeight(e.NPC, e.Damage, 100))
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			if e.Attacker.Playable() {
				e.Called.AddAttackDesire(e.Attacker, damageWeight(e.Called, e.Damage, 100))
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			e.NPC.AddAttackDesire(e.Creature, 200)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if aggro := s.Skill(e.Skill).AggroPoints(); aggro > 0 {
				e.NPC.AddAttackDesire(e.Caster, damageWeight(e.NPC, int32(aggro), 150))
			}
		},
	}).Named("Agit01Move1")
}

// Move1 is Agit01Move1: a Partisan Hideaway soldier or archer that fights
// the playables it sees, those that hit it or a clan mate, and those whose
// spells it sees.
func Move1() script.Script {
	return script.Script{Dir: move1Dir, Behavior: true, NPCs: []int32{35372, 35379, 35381}, Hooks: move1Hooks()}
}

// PartisanSorcerer2 is Agit01PartisanSorcerer2: an Agit01Move1 sorcerer
// that, in place of fighting them, casts skill 4043 from where it stands
// at every playable it sees, and on a draw under 10 at one attacking a
// clan mate.
func PartisanSorcerer2() script.Script {
	base := move1Hooks()
	return script.Script{Dir: move1Dir, Behavior: true, NPCs: []int32{35380}, Hooks: base.With(script.Hooks{
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

// partisanHealer1Hooks are Agit01PartisanHealer1's hooks, which the
// healer's child derives its own from: on a draw under 20 the healer wants
// to cast its heal 4044 on Bloody Lord Nurka when he is hit below 60% of
// his hit points.
func partisanHealer1Hooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if id := e.Caller.NpcID(); (id == 35368 || id == 35375) && e.Caller.HPRatio() < 0.6 && s.Rnd(100) < 20 {
				e.Called.AddCastDesire(e.Caller, level1(4044), 1000000)
			}
		},
	}).Named("Agit01PartisanHealer1")
}

// PartisanHealer1 is Agit01PartisanHealer1: a Partisan Hideaway healer
// that heals Bloody Lord Nurka when he is hit low.
func PartisanHealer1() script.Script {
	return script.Script{Dir: healerDir, Behavior: true, NPCs: []int32{35369}, Hooks: partisanHealer1Hooks()}
}
