package monster

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// LV3Ranger is an LV3Monster that spawns in its arrow defence mode and
// strikes its top desire target with its physical specials. A hit by
// either of its check skills while in that mode drops it to its normal
// mode; once the defence mode's cast finishes there, it goes back to it
// after 5 s (first check skill), 3 s (second) or 3 s.
func LV3Ranger() script.Script {
	base := lv3MonsterHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27289, 27292, 27293, 27296, 27297, 27298}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			castByType(n, n, npc.SkillTypeArrowDefenceMode, 1000000)
			n.SetInt(script.SlotIntAI0, 1)
			n.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			e.Damage = lv3Strike(s, n, e.Attacker, e.Damage, 1000000)
			if n.Int(script.SlotIntAI0) == 1 && e.Skill.ID != 0 {
				if skillIs(n, e.Skill, npc.SkillTypeCheckSkill1) {
					n.SetInt(script.SlotIntAI0, 0)
					castByType(n, n, npc.SkillTypeArrowDefenceMode, 1000000)
					castByType(n, n, npc.SkillTypeArrowNormalMode, 1000000)
					n.SetInt(script.SlotIntAI1, 5)
				}
				if skillIs(n, e.Skill, npc.SkillTypeCheckSkill2) {
					n.SetInt(script.SlotIntAI0, 0)
					castByType(n, n, npc.SkillTypeArrowDefenceMode, 1000000)
					castByType(n, n, npc.SkillTypeArrowNormalMode, 1000000)
					n.SetInt(script.SlotIntAI1, 3)
				}
			}
			base.Attacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if skillIs(n, e.Skill, npc.SkillTypeArrowDefenceMode) && n.Int(script.SlotIntAI0) == 0 {
				if n.Int(script.SlotIntAI1) != 0 {
					s.StartTimer("4004", n, nil, time.Duration(n.Int(script.SlotIntAI1))*1000*time.Millisecond)
				} else {
					s.StartTimer("4004", n, nil, 3000*time.Millisecond)
				}
			}
			base.UseSkillFinished(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "4000"), strings.EqualFold(e.Name, "4001"), strings.EqualFold(e.Name, "4002"),
				strings.EqualFold(e.Name, "4003"), strings.EqualFold(e.Name, "4004"):
				if n.Int(script.SlotIntAI0) == 0 {
					n.SetInt(script.SlotIntAI0, 1)
					castByType(n, n, npc.SkillTypeArrowDefenceMode, 1000000)
					castByType(n, n, npc.SkillTypeArrowNormalMode, 1000000)
				}
			}
			return base.Timer(s, script.Timer{Name: e.Name, NPC: n})
		},
	})}
}

// LV3SongDance is an LV3Monster that, hit, may want to cast each of its
// three debuffs on the attacker until that debuff's cast finishes once.
// A spell of the player its second spawn parameter names with the same
// effect as one of the debuffs makes that debuff wanted again.
func LV3SongDance() script.Script {
	base := lv3MonsterHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27269, 27270, 27272, 27288}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			n.SetInt(script.SlotIntAI1, 0)
			n.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if e.Damage == 0 {
					e.Damage = 1
				}
				n.AddAttackDesire(attacker, ((1.000000*float64(e.Damage))/float64(n.Level()+7))*100)
			}
			if n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypeDebuff1, 1000000)
			}
			if n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypeDebuff2, 1000000)
			}
			if n.Int(script.SlotIntAI2) == 0 && s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypeDebuff3, 1000000)
			}
			base.Attacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if skillIs(n, e.Skill, npc.SkillTypeDebuff1) {
				n.SetInt(script.SlotIntAI0, 1)
			}
			if skillIs(n, e.Skill, npc.SkillTypeDebuff2) {
				n.SetInt(script.SlotIntAI1, 1)
			}
			if skillIs(n, e.Skill, npc.SkillTypeDebuff3) {
				n.SetInt(script.SlotIntAI2, 1)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n := e.NPC
			if e.Caster != nil && e.Caster.ObjectID() == n.Int(script.SlotParam2) {
				if s.Skill(e.Skill).EffectID() == listedEffectID(s, n, npc.SkillTypeDebuff1) {
					n.SetInt(script.SlotIntAI0, 0)
				}
				if s.Skill(e.Skill).EffectID() == listedEffectID(s, n, npc.SkillTypeDebuff2) {
					n.SetInt(script.SlotIntAI1, 0)
				}
				if s.Skill(e.Skill).EffectID() == listedEffectID(s, n, npc.SkillTypeDebuff3) {
					n.SetInt(script.SlotIntAI2, 0)
				}
			}
			base.SeeSpell(s, e)
		},
	})}
}

// listedEffectID returns the effect id of the debuff n's template lists
// under t. A template that lists none panics, so the invocation aborts
// there.
func listedEffectID(s *script.Script, n *script.NPC, t npc.SkillType) int32 {
	ref, ok := n.SkillByType(t)
	if !ok {
		panic("monster: a song dancer's template lists no debuff skill")
	}
	return s.Skill(ref).EffectID()
}

// LV3Summoner is an LV3Monster that strikes its top desire target with its
// physical specials, checks every second whether it holds its buff and,
// every 3 s while it does not, may drop its desires, buff itself and go
// back to the creature its first spawn parameter named.
func LV3Summoner() script.Script {
	base := lv3MonsterHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27313, 27314, 27315}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimerAtFixedRate("4000", n, nil, 1000*time.Millisecond, 1000*time.Millisecond)
			s.StartTimerAtFixedRate("4001", n, nil, 3000*time.Millisecond, 3000*time.Millisecond)
			n.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			e.Damage = lv3Strike(s, e.NPC, e.Attacker, e.Damage, 1000)
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "4000") {
				buff1, _ := n.SkillByType(npc.SkillTypeBuff1)
				if s.AbnormalLevel(n, buff1) > 0 {
					n.SetInt(script.SlotIntAI0, 1)
				} else {
					n.SetInt(script.SlotIntAI0, 0)
				}
			} else if strings.EqualFold(e.Name, "4001") {
				if c0 := n.Slot(script.SlotCreature0); c0 != nil && n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 50 {
					n.RemoveAllDesire()
					castByType(n, n, npc.SkillTypeBuff1, 1000000)
					n.AddAttackDesire(c0, 200)
					n.SetInt(script.SlotIntAI0, 1)
				}
			}
			return base.Timer(s, e)
		},
	})}
}

// lv3WizardPrivateID is the private an LV3Wizard raises.
const lv3WizardPrivateID = 27253

// LV3Wizard is an LV3Monster that spawns in its summon mode and raises an
// LV3WizardPrivate with its own spawn parameters. Each time a private of
// its party dies it raises another, up to five, then only returns to its
// summon mode. A private's event 1000 makes it want to heal the creature
// the event names.
func LV3Wizard() script.Script {
	base := lv3MonsterHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27250, 27251, 27252, 27257}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 5)
			castByType(n, n, npc.SkillTypeSummonMode, 1000000)
			raiseWizardPrivate(s, n)
			base.Created(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if !script.Same(called, caller) {
				if called.Int(script.SlotIntAI0) > 0 {
					raiseWizardPrivate(s, called)
					castByType(called, called, npc.SkillTypeSummonEffect, 1000000)
					called.SetInt(script.SlotIntAI0, called.Int(script.SlotIntAI0)-1)
				} else {
					castByType(called, called, npc.SkillTypeSummonMode, 1000000)
				}
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.EventID == 1000 {
				if c0 := s.Object(e.Arg1); c0 != nil {
					if s.Rnd(100) < 50 {
						castByType(n, c0, npc.SkillTypeSummonHeal1, 1000000)
					} else {
						castByType(n, c0, npc.SkillTypeSummonHeal2, 1000000)
					}
				}
			}
		},
	})}
}

// raiseWizardPrivate raises an LV3WizardPrivate where n stands, facing
// heading 0, with n's three spawn parameters.
func raiseWizardPrivate(s *script.Script, n *script.NPC) {
	at := script.LocOf(n)
	params := script.Params{P1: n.Int(script.SlotParam1), P2: n.Int(script.SlotParam2), P3: n.Int(script.SlotParam3)}
	s.Spawner().CreateOnePrivateEx(n, lv3WizardPrivateID, script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0, params)
}
