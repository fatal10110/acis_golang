package warrior

import (
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PhysicalSpecialDebuffAggressive is
// WarriorPhysicalSpecialDebuffAggressive: a WarriorPhysicalSpecial that
// attacks the players and summons it sees and may debuff the one it hates
// most, or a clan member's attacker, when the debuff is not on them yet:
// on a hit, on a clan call, and every 5 s of a fight in which that one
// has not hit it.
//
// Its memory: IntAI1 is set while the 5 s check runs, IntAI2 once the
// most hated hit it since the last check.
func PhysicalSpecialDebuffAggressive() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{21383, 21407, 21380, 21413}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			mostHated := n.MostHated()
			if n.Int(script.SlotIntAI1) == 1 {
				if script.Same(mostHated, attacker) {
					n.SetInt(script.SlotIntAI2, 1)
				}
			} else {
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI1, 1)
			}
			if attacker.Playable() && script.Same(mostHated, attacker) && s.Rnd(100) < 33 {
				if debuff, ok := n.SkillByType(npc.SkillTypeDebuff); ok && s.AbnormalLevel(attacker, debuff) <= 0 {
					n.AddCastDesire(attacker, debuff, 1000000)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && s.Rnd(100) < 33 {
				if debuff, ok := called.SkillByType(npc.SkillTypeDebuff); ok && s.AbnormalLevel(attacker, debuff) <= 0 {
					called.AddCastDesire(attacker, debuff, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if n.Intention() != script.IntentionAttack && n.Intention() != script.IntentionCast {
					n.SetInt(script.SlotIntAI1, 0)
					n.SetInt(script.SlotIntAI2, 0)
					return base.Timer(s, e)
				}
				if n.Int(script.SlotIntAI2) == 0 {
					if mostHated := n.MostHated(); mostHated != nil && s.Rnd(100) < 50 {
						if debuff, ok := n.SkillByType(npc.SkillTypeDebuff); ok && s.AbnormalLevel(mostHated, debuff) <= 0 {
							n.AddCastDesire(mostHated, debuff, 1000000)
						}
					}
				}
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
	})}
}

// PhysicalSpecialFollowing is WarriorPhysicalSpecialFollowing: a
// WarriorPhysicalSpecial that, told by script event 10019 the object id of
// a creature, walks to a random point within 50 of it on each axis.
func PhysicalSpecialFollowing() script.Script {
	base := PhysicalSpecialHooks()
	ids := []int32{21674, 21675, 21697, 21698, 21720, 21721, 21743, 21744, 21766, 21767, 21789, 21790}
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: ids, Hooks: base.With(script.Hooks{
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			if e.EventID == 10019 {
				if c0 := s.Object(e.Arg1); c0 != nil {
					at := script.LocOf(c0)
					x := (at.X - 50) + s.Rnd(100)
					y := (at.Y - 50) + s.Rnd(100)
					e.NPC.AddMoveToDesire(script.Loc{X: x, Y: y, Z: at.Z}, 15)
				}
			}
			base.ScriptEvent(s, e)
		},
	})}
}

// PhysicalSpecialStoneAggressive is
// WarriorPhysicalSpecialStoneAggressive: a Ketra or Varka
// WarriorPhysicalSpecial that attacks the players and summons it sees,
// except one allied with its own faction, and petrifies such an ally when
// it attacks it or a clan member, or helps someone near it.
func PhysicalSpecialStoneAggressive() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{21328, 21325, 21351, 21354}, Hooks: base.With(script.Hooks{
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
			defaultnpc.TryToAttack(s, n, e.Creature)
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}

// PhysicalSpecialStrongAggressive is
// WarriorPhysicalSpecialStrongAggressive: a WarriorPhysicalSpecial that
// attacks the players and summons it sees and, below half its HP, may
// strike an attacker with its physical special skill, or one in a party
// with its self range physical special skill.
func PhysicalSpecialStrongAggressive() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{22052, 22055, 22058, 22062}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			p := script.ActingPlayer(attacker)
			inParty := p != nil && p.InParty()
			if n.HPRatio() < 0.5 {
				if !inParty && s.Rnd(100) < 33 {
					if ref, ok := n.SkillByType(npc.SkillTypePhysicalSpecial); ok {
						n.AddCastDesire(attacker, ref, 1000000)
					}
				} else if inParty && s.Rnd(100) < 33 {
					if ref, ok := n.SkillByType(npc.SkillTypeSelfRangePhysicalSpecial); ok {
						n.AddCastDesire(attacker, ref, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
	})}
}
