// Package wizard holds the Wizard family of monster behaviors: WizardBase
// and Wizard. A wizard keeps a hate list beside its attack hate; its spells
// are aimed by the hate list.
package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
)

// Base is WizardBase. It binds no NPC of its own.
func Base() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WizardBase", Behavior: true, Hooks: BaseHooks()}
}

// The script events a wizard answers: a shouting monster names its target,
// and a caller asks it to look around.
const (
	eventShoutTarget = 10016
	eventLook        = 10020
)

// isPlayer reports whether c is a player.
func isPlayer(c script.Creature) bool {
	_, ok := c.(*script.Player)
	return ok
}

// fighting reports whether i is an attack or a cast.
func fighting(i script.Intention) bool {
	return i == script.IntentionAttack || i == script.IntentionCast
}

// damageHate returns the hate list weight of amount, before its
// multiplier: amount scaled down by the NPC's level and up by its hate
// ratio for the attacker, in percent.
func damageHate(n *script.NPC, amount, hateRatio float64) float64 {
	return ((1.0 * amount) / float64(n.Level()+7)) + ((hateRatio / 100) * ((1.0 * amount) / float64(n.Level()+7)))
}

// BaseHooks are WizardBase's hooks, which the Wizard behaviors derive
// theirs from.
func BaseHooks() script.Hooks {
	base := monster.Hooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1") {
				if n.IntParam("AttackLowLevel") == 1 {
					n.LookNeighbor()
				}
			} else if strings.EqualFold(e.Name, "2") {
				if n.IntParam("IsVs") == 1 {
					n.SetSlot(script.SlotCreature0, n)
				}
			}
			return base.Timer(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.Int(script.SlotParam1) == 1000 {
				if c0 := s.Object(n.Int(script.SlotParam2)); c0 != nil {
					n.AddCastDesire(c0, skill.Ref{ID: 4663, Level: 1}, 1000000)
					n.HateList().Add(c0, 500)
				}
			}
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if !attacked(s, e) {
				return
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.IntParam("AttackLowHP") == 1 && attacker.HPRatio() < 0.3 && s.Rnd(100) < 3 {
				hateList := called.HateList()
				if mostHated := hateList.MostHated(); mostHated != nil && !script.Same(mostHated, attacker) {
					hateList.Clear()
					if attacker.Playable() {
						f0 := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, attacker))
						if hateList.Empty() {
							hateList.Add(attacker, (f0*100)+300)
						} else {
							hateList.Add(attacker, f0*100)
						}
					}
				}
			}
			if called.IntParam("AttackLowLevel") == 1 && !fighting(called.Intention()) {
				s.StartTimer("1", called, nil, 7000*time.Millisecond)
			}
			base.ClanAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() || n.Dead() {
				return
			}
			if n.IntParam("AttackLowLevel") == 1 {
				if fighting(n.Intention()) && n.Distance2D(c) < 300 {
					if c.Level()+15 < n.Level() {
						hateList := n.HateList()
						hateList.Clear()
						hateList.Add(c, 7*100)
					}
					s.StartTimer("1", n, nil, 7000*time.Millisecond)
				}
			}
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if caster.Level()+15 >= n.Level() {
				return
			}
			hateList := n.HateList()
			hateList.Clear()
			k := s.Skill(e.Skill)
			if k.AggroPoints() > 0 || k.Power(n) > 0 || k.Offensive() {
				mostHated := n.MostHated()
				intention := n.Intention()
				skillPower := max(max(float64(k.AggroPoints()), k.Power(n)), 20)
				f0 := damageHate(n, skillPower, defaultnpc.HateRatio(n, caster))
				if mostHated != nil && script.Same(mostHated, caster) && intention == script.IntentionAttack {
					hateList.Add(caster, f0*150)
				} else {
					hateList.Add(caster, f0*75)
				}
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.EventID {
			case eventShoutTarget:
				if s.Rnd(100) < 50 {
					hateList := n.HateList()
					hateList.Clear()
					if c0 := s.Object(e.Arg1); c0 != nil && c0.Playable() {
						hateList.Add(c0, 200)
					}
				}
			case eventLook:
				if s.Object(e.Arg1) != nil {
					n.LookNeighbor()
				}
			}
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			if selfExplosion, ok := e.NPC.SkillByType(npc.SkillTypeSelfExplosion); ok && e.Skill.ID == selfExplosion.ID {
				e.NPC.DoDie(e.NPC)
			}
		},
	}).Named("WizardBase")
}

// attacked is WizardBase's attacked hook up to its parent call; false when
// the hook returns before it.
func attacked(s *script.Script, e script.Attacked) bool {
	n, attacker := e.NPC, e.Attacker
	if n.IntParam("CreviceOfDiminsion") != 0 && !n.InSpawnTerritory(attacker) {
		n.StopHate(attacker)
		return false
	}
	if !n.Stunned() && !n.Paralyzed() {
		if ssRate := n.IntParam("SoulShotRate"); ssRate != 0 && int32(s.Rnd(100)) < ssRate {
			n.RechargeShots(true, false)
		}
		if spsRate := n.IntParam("SpiritShotRate"); spsRate != 0 && int32(s.Rnd(100)) < spsRate {
			n.RechargeShots(false, true)
		}
	}

	intention := n.Intention()
	hpRatio := n.HPRatio()
	attackerHPRatio := attacker.HPRatio()
	hateList := n.HateList()
	mostHated := hateList.MostHated()

	if n.IntParam("AttackLowLevel") == 1 && !fighting(intention) {
		s.StartTimer("1", n, nil, 7000*time.Millisecond)
	}

	if attacker.Playable() && mostHated != nil && !script.Same(mostHated, attacker) && n.IntParam("AttackLowHP") == 1 && attackerHPRatio <= 0.3 && s.Rnd(100) < 10 {
		hateList.Clear()
		f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
		if hateList.Empty() {
			hateList.Add(attacker, (f0*100)+300)
		} else {
			hateList.Add(attacker, f0*100)
		}
	}

	if isPlayer(attacker) {
		if helpHeroSilhouetteID := n.IntParam("HelpHeroSilhouette"); helpHeroSilhouetteID != 0 && attackerHPRatio <= 0.2 && s.Rnd(100) < 3 {
			at := script.LocOf(n)
			at.X, at.Y = at.X+80, at.Y+80
			s.Spawner().CreateOnePrivateEx(n, helpHeroSilhouetteID, at, 0, script.Params{P3: n.ObjectID()})
		}
	}

	if specialSkill, ok := n.SkillByType(npc.SkillTypeSpecialSkill); ok && hpRatio <= 0.3 && s.Rnd(100) < 10 && s.AbnormalLevel(n, specialSkill) <= 0 {
		switch s.Rnd(4) {
		case 0:
			n.Say(script.NpcString(1000290))
		case 1:
			n.Say(script.NpcString(1000395))
		case 2:
			n.Say(script.NpcString(1000396))
		case 3:
			n.Say(script.NpcString(1000397))
		}
		n.AddCastDesire(n, specialSkill, 1000000)
	}

	if mostHated != nil && script.Same(mostHated, attacker) && n.IntParam("ShoutTarget") != 0 && s.Rnd(100) < 5 {
		s.BroadcastScriptEvent(n, eventShoutTarget, attacker.ObjectID(), 300)
	}

	if selfExplosion, ok := n.SkillByType(npc.SkillTypeSelfExplosion); ok && hpRatio < 0.5 && s.Rnd(100) < 5 && 10-int(hpRatio*10) > s.Rnd(100) {
		n.AddCastDesire(n, selfExplosion, 3000000)
	}

	if isTransform := n.IntParam("IsTransform"); isTransform > 0 {
		transform(s, n, attacker, isTransform, hpRatio)
	}
	return true
}

// transform replaces the NPC, at its transformation step, with the NPC of
// its next step, which casts on attacker first.
func transform(s *script.Script, n *script.NPC, attacker script.Creature, isTransform int32, hpRatio float64) {
	step := n.Int(script.SlotParam3)
	next := func(param string, nextStep int32, says [3]int32) {
		switch s.Rnd(3) {
		case 0:
			n.Say(script.NpcString(says[0]))
		case 1:
			n.Say(script.NpcString(says[1]))
		case 2:
			n.Say(script.NpcString(says[2]))
		}
		s.Spawner().CreateOnePrivateEx(n, n.IntParam(param), script.LocOf(n), 0, script.Params{P1: 1000, P2: attacker.ObjectID(), P3: nextStep})
		n.DeleteMe()
	}
	switch step {
	case 0:
		if step < isTransform && hpRatio < 0.7 && hpRatio > 0.5 && s.Rnd(100) < 30 {
			next("step1", 1, [3]int32{1000406, 1000407, 1000408})
		}
	case 1:
		if step < isTransform && hpRatio < 0.5 && hpRatio > 0.3 && s.Rnd(100) < 20 {
			next("step2", 2, [3]int32{1000409, 1000410, 1000411})
		}
	case 2:
		if step < isTransform && hpRatio < 0.3 && hpRatio > 0.05 && s.Rnd(100) < 10 {
			next("step3", 3, [3]int32{1000412, 1000413, 1000414})
		}
	}
}
