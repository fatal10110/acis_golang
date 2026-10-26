package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// DDMagic2Curse is WizardDDMagic2Curse: a WizardDDMagic2 that, until it
// gives up its spells, may answer a playable's hit, while it hates someone
// and the attacker holds no effect of the skill it was hit with, with its
// debuff on the attacker, on a draw under 33. A settled one answers a
// playable's hit on its clan member the same way. Short of MP it gives up
// its spells and attacks instead.
func DDMagic2Curse() script.Script {
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{
		20258, 20781, 20792, 20556, 21577, 20644, 21101, 20581, 20266, 20587, 20685, 20067, 22127,
	}, Hooks: ddMagic2CurseHooks()}
}

// maybeCurseAttacker makes n, while it hates someone and attacker holds no
// effect of the skill used (none for a plain hit), want to cast its debuff
// at attacker on a draw under 33.
func maybeCurseAttacker(s *script.Script, n *script.NPC, attacker script.Creature, used skill.Ref) {
	if n.HateList().MostHated() != nil && s.AbnormalLevel(attacker, used) <= 0 && s.Rnd(100) < 33 {
		castOrAttack(n, attacker, npc.SkillTypeDebuff, false)
	}
}

// ddMagic2CurseHooks are DDMagic2Curse's hooks, which its children derive
// theirs from.
func ddMagic2CurseHooks() script.Hooks {
	base := ddMagic2Hooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 {
				maybeCurseAttacker(s, n, attacker, e.Skill)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				maybeCurseAttacker(s, called, attacker, e.Skill)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WizardDDMagic2Curse")
}

// DDMagic2CurseAggressive is WizardDDMagic2CurseAggressive: a
// WizardDDMagic2Curse that, settled in its territory with an empty hate
// list, strikes a playable it sees with its long range DD magic beyond
// 100, else its short range one, unchecked, and hates it; short of MP it
// gives up its spells and attacks instead.
func DDMagic2CurseAggressive() script.Script {
	base := ddMagic2CurseHooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{
		20068, 20817, 21106, 20564, 20809, 21406, 21645, 21144, 21072, 21156, 21148, 21160, 21307, 22121,
	}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			hateList := n.HateList()
			if n.LifeTime() > 7 && hateList.Empty() && n.InMyTerritory() {
				ddMagic2Spell(n, c)
				hateList.AddDefault(c)
				base.SeeCreature(s, e)
			}
		},
	})}
}

// HereticDDMagic2Curse is WizardHereticDDMagic2Curse: a WizardDDMagic2Curse
// that the calls of its neighbours steer. A hit on it or its clan while it
// neither attacks nor casts engages it; out of its territory unengaged it
// walks home. A call for help (10033) or a shouted target (10002) on a
// creature in the world drops its attack desires and strikes that creature
// by distance, putting a playable on its hate list by 200; the call for
// help also engages it. A call to walk home (10035) disengages it and walks
// it home.
func HereticDDMagic2Curse() script.Script {
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{22139, 22148}, Hooks: hereticDDMagic2CurseHooks()}
}

// hereticDDMagic2CurseHooks are HereticDDMagic2Curse's hooks, which its
// child derives its from.
func hereticDDMagic2CurseHooks() script.Hooks {
	base := ddMagic2CurseHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			hereticEngage(e.NPC)
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			hereticEngage(e.Called)
			base.ClanAttacked(s, e)
		},
		OnOutOfTerritory: hereticOutOfTerritory,
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.EventID {
			case eventHelpCall, eventMasterTarget:
				c0 := s.Object(e.Arg1)
				if c0 == nil {
					return
				}
				if e.EventID == eventHelpCall {
					n.SetInt(script.SlotIntAI4, 1)
				}
				n.RemoveAllAttackDesire()
				hallSpell(n, c0)
				if c0.Playable() {
					n.HateList().Add(c0, 200)
				}
			case eventWalkHome:
				n.SetInt(script.SlotIntAI4, 0)
				n.RemoveAllAttackDesire()
				n.AddMoveToDesire(n.SpawnLocation(), 100)
			}
		},
	}).Named("WizardHereticDDMagic2Curse")
}

// HereticDDMagic2CurseAggressive is WizardHereticDDMagic2CurseAggressive: a
// WizardHereticDDMagic2Curse that, settled in its territory with nobody on
// its hate list, strikes a playable it sees with its long range DD magic
// beyond 100, else its short range one, and hates it; short of MP it gives
// up its spells and attacks instead.
func HereticDDMagic2CurseAggressive() script.Script {
	base := hereticDDMagic2CurseHooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{22153, 22191, 22192}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.InMyTerritory() && n.HateList().Size() == 0 {
				hallSpell(n, c)
				n.HateList().AddDefault(c)
				base.SeeCreature(s, e)
			}
		},
	})}
}
