package warrior

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Berserker is WarriorBerserker: a Warrior that roars as its clan members
// die, casting its fury on itself one level higher each time, up to its
// MaxRoarCount.
func Berserker() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: berserkerIDs(), Hooks: berserkerHooks()}
}

// berserkerIDs are WarriorBerserker's own NPCs.
func berserkerIDs() []int32 {
	return []int32{20086, 20248, 20267, 20601, 21208, 21212, 21216, 21220, 21224, 21228, 21232, 21236, 21240, 21244, 21248, 21252}
}

// roar counts one more roar of called and returns its fury at that count,
// when called has roared fewer times than its MaxRoarCount. A template
// that lists no fury skill aborts the hook after the count.
func roar(called *script.NPC) (skill.Ref, bool) {
	if called.Int(script.SlotIntAI1) >= called.IntParam("MaxRoarCount") {
		return skill.Ref{}, false
	}
	called.SetInt(script.SlotIntAI1, called.Int(script.SlotIntAI1)+1)
	furyBase, ok := called.SkillByType(npc.SkillTypeFury)
	if !ok {
		panic("warrior: a berserker's template lists no fury skill")
	}
	return skill.Ref{ID: furyBase.ID, Level: int(called.Int(script.SlotIntAI1))}, true
}

// berserkerHooks are WarriorBerserker's hooks, which its children derive
// theirs from.
func berserkerHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnClanDied: func(s *script.Script, e script.ClanDied) {
			if called := e.Called; !script.Same(e.Caller, called) {
				if fury, ok := roar(called); ok {
					called.AddCastDesire(called, fury, 1000000)
				}
			}
			base.ClanDied(s, e)
		},
	}).Named("WarriorBerserker")
}

// BerserkerPhysicalSpecial is WarriorBerserkerPhysicalSpecial: a
// berserker that strikes the player it hates most, or a clan member's
// attacker, with its physical special skill and its debuff, and adds its
// ranged strike to a roar while it has a target.
func BerserkerPhysicalSpecial() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21410}, Hooks: berserkerPhysicalSpecialHooks()}
}

// castByType makes n want to cast at target the skill its template lists
// under t; a template that lists none casts nothing.
func castByType(n *script.NPC, target script.Creature, t npc.SkillType) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesire(target, ref, 1000000)
	}
}

// berserkerPhysicalSpecialHooks are WarriorBerserkerPhysicalSpecial's
// hooks, which its children derive theirs from.
func berserkerPhysicalSpecialHooks() script.Hooks {
	base := berserkerHooks()
	return base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil && script.Same(topDesireTarget, attacker) {
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypePhysicalSpecial)
					}
					debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
					if s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
						castByType(n, attacker, npc.SkillTypeDebuff)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					castByType(called, attacker, npc.SkillTypePhysicalSpecial)
				}
				if s.Rnd(100) < 33 {
					castByType(called, attacker, npc.SkillTypeDebuff)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnClanDied: func(s *script.Script, e script.ClanDied) {
			if called := e.Called; !script.Same(e.Caller, called) {
				if fury, ok := roar(called); ok {
					called.AddCastDesire(called, fury, 1000000)
					if topDesireTarget := called.TopDesireTarget(); topDesireTarget != nil {
						castByType(called, called, npc.SkillTypeSelfRangePhysicalSpecial)
					}
				}
			}
			base.ClanDied(s, e)
		},
	}).Named("WarriorBerserkerPhysicalSpecial")
}

// BerserkerPhysicalSpecialAggressive is
// WarriorBerserkerPhysicalSpecialAggressive: a
// WarriorBerserkerPhysicalSpecial that attacks the players and summons it
// sees.
func BerserkerPhysicalSpecialAggressive() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21382, 21420}, Hooks: berserkerPhysicalSpecialAggressiveHooks()}
}

// berserkerPhysicalSpecialAggressiveHooks are
// WarriorBerserkerPhysicalSpecialAggressive's hooks, which its child
// derives its own from.
func berserkerPhysicalSpecialAggressiveHooks() script.Hooks {
	base := berserkerPhysicalSpecialHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	}).Named("WarriorBerserkerPhysicalSpecialAggressive")
}

// BerserkerPhysicalSpecialStoneAggressive is
// WarriorBerserkerPhysicalSpecialStoneAggressive: a Ketra or Varka
// aggressive berserker that petrifies an attacker allied with its own
// faction, and a player who helps one, and ignores such a player it sees.
func BerserkerPhysicalSpecialStoneAggressive() script.Script {
	base := berserkerPhysicalSpecialAggressiveHooks()
	hooks := base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if defaultnpc.MaybeCastPetrify(e.NPC, e.Attacker) {
				return
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if defaultnpc.MaybeCastPetrify(e.Called, e.Attacker) {
				return
			}
			base.ClanAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			p := script.ActingPlayer(e.Creature)
			if p == nil {
				return
			}
			if p.AlliedWithVarka() && slices.Contains(n.Clans(), "varka_silenos_clan") {
				return
			}
			if p.AlliedWithKetra() && slices.Contains(n.Clans(), "ketra_orc_clan") {
				return
			}
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: berserkerStoneIDs(), Hooks: hooks}
}

// berserkerStoneIDs are WarriorBerserkerPhysicalSpecialStoneAggressive's
// own NPCs.
func berserkerStoneIDs() []int32 {
	return []int32{21327, 21332, 21336, 21339, 21340, 21353, 21358, 21362, 21365, 21366}
}
