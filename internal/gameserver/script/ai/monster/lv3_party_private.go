package monster

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// LV3PartyPrivate is LV3PartyPrivateMonster: an LV3Monster raised as
// another one's private. It takes its master's three spawn parameters as it
// spawns and leaves the world once its master is gone or dead (checked
// every 5 s) or dies. With a live master it answers a playable's hit on
// itself with a tenth of the LV3 weight, and a playable's hit on its party
// or clan with weights of its own; masterless, it takes the full LV3 weight
// and the hate of a spell aimed at the creature it fights.
func LV3PartyPrivate() script.Script {
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27261, 27291, 27268}, Hooks: lv3PartyPrivateHooks()}
}

// lv3PartyPrivateHooks are LV3PartyPrivateMonster's hooks, which its
// children derive theirs from.
func lv3PartyPrivateHooks() script.Hooks {
	base := lv3MonsterHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotParam1, n.Master().Int(script.SlotParam1))
			n.SetInt(script.SlotParam2, n.Master().Int(script.SlotParam2))
			n.SetInt(script.SlotParam3, n.Master().Int(script.SlotParam3))
			s.StartTimer("3007", n, nil, 5000*time.Millisecond)
			base.Created(s, e)
		},
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if target.Playable() {
				called.AddAttackDesire(target, (((float64(e.Damage)*1.0)/float64(called.MaxHP()))/0.050000)*50)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if !n.HasMaster() || n.Master().Dead() {
				if attacker.Playable() {
					if e.Damage == 0 {
						e.Damage = 1
					}
					n.AddAttackDesire(attacker, ((1.0*float64(e.Damage))/float64(n.Level()+7))*100)
				}
			} else if attacker.Playable() {
				if e.Damage == 0 {
					e.Damage = 1
				}
				n.AddAttackDesire(attacker, ((1.0*float64(e.Damage))/float64(n.Level()+7))*10)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() {
				called.AddAttackDesire(attacker, ((1.0*float64(e.Damage))/float64(called.Level()+7))*30)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n := e.NPC
			if !n.HasMaster() || n.Master().Dead() {
				if s.Skill(e.Skill).AggroPoints() > 0 {
					if n.AttackingNow() && len(e.Targets) > 0 && script.Same(n.TopDesireTarget(), e.Targets[0]) {
						i0 := s.Skill(e.Skill).AggroPoints()
						f0 := defaultnpc.HateRatio(n, e.Caster)
						f0 = ((1.000000 * float64(i0)) / float64(n.Level()+7)) + ((f0 / 100) * ((1.000000 * float64(i0)) / float64(n.Level()+7)))
						n.AddAttackDesire(e.Caster, f0*150)
					}
				}
			}
		},
		OnPartyDied: func(_ *script.Script, e script.PartyDied) {
			if script.Same(e.Caller, e.Called.Master()) {
				e.Called.DeleteMe()
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1005") {
				if !n.InMyTerritory() && n.HasMaster() && !n.InCombat() {
					n.TeleportTo(n.SpawnLocation())
					n.RemoveAllAttackDesire()
				}
				s.StartTimer("1005", n, e.Player, 120000*time.Millisecond)
			}
			if strings.EqualFold(e.Name, "3007") {
				if !n.HasMaster() || n.Master().Dead() {
					n.DeleteMe()
				} else {
					s.StartTimer("3007", n, e.Player, 5000*time.Millisecond)
				}
			}
			return base.Timer(s, e)
		},
	}).Named("LV3PartyPrivateMonster")
}

// LV3WizardPrivate is an LV3PartyPrivateMonster an LV3Wizard raises. As it
// spawns it attacks the creature its first spawn parameter names (or
// blasts around itself when that creature stands close and it has lived
// past its third AI cycle) and tells its master its object id (event
// 1000). Every second it blasts around itself while that creature stands
// within 100 and attacks it. A finished blast is followed by one of its
// two other magics on that creature; any other finished cast kills it.
func LV3WizardPrivate() script.Script {
	base := lv3PartyPrivateHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27253}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetSlot(script.SlotCreature0, s.Object(n.Int(script.SlotParam1)))
			if c0 := n.Slot(script.SlotCreature0); c0 != nil {
				if n.Distance2D(c0) < 100 && n.LifeTime() > 3 {
					castByType(n, n, npc.SkillTypeSelfRangeDDMagic1, 1000000)
				} else {
					n.AddAttackDesire(c0, 300)
				}
			}
			master := n.Master()
			if master == nil {
				panic("monster: an LV3 wizard private has no master to tell")
			}
			s.SendScriptEvent(master, 1000, n.ObjectID(), 0)
			s.StartTimerAtFixedRate("4000", n, nil, 1000*time.Millisecond, 1000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "4000") {
				if c0 := n.Slot(script.SlotCreature0); c0 != nil {
					if n.Distance2D(c0) < 100 {
						castByType(n, n, npc.SkillTypeSelfRangeDDMagic1, 1000000)
					}
					n.AddAttackDesire(c0, 50)
				}
			}
			return base.Timer(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if skillIs(n, e.Skill, npc.SkillTypeSelfRangeDDMagic1) {
				if c0 := n.Slot(script.SlotCreature0); c0 != nil {
					if s.Rnd(100) < 50 && n.Distance2D(c0) < 200 {
						castByType(n, c0, npc.SkillTypeSelfRangeDDMagic2, 3000)
					} else {
						castByType(n, c0, npc.SkillTypeSelfRangeDDMagic3, 3000)
					}
				}
				n.RemoveAllAttackDesire()
			} else {
				n.DoDie(n)
			}
		},
	})}
}

// skillIs reports whether ref is the skill n's template lists under t; a
// template that lists none matches no skill.
func skillIs(n *script.NPC, ref skill.Ref, t npc.SkillType) bool {
	listed, ok := n.SkillByType(t)
	return ok && ref == listed
}
