package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The script events a clan hall wizard answers by their first argument: a
// guard names a creature to strike, or one to walk to.
const (
	argHallStrike = 10033
	argHallMoveTo = 10042
)

// DDMagic2Aggressive is WizardDDMagic2Aggressive: a WizardDDMagic2 that,
// settled in its territory with an empty hate list, strikes a playable it
// sees with its long range DD magic beyond 100, else its short range one,
// unchecked, and hates it; short of MP it gives up its spells and attacks
// instead.
func DDMagic2Aggressive() script.Script {
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{
		21311, 21285, 21575, 20813, 21293, 21416, 21111, 20052, 27156, 20044, 21102, 21399, 20619, 20420, 20507, 20835,
		21086, 21088, 21140, 21152, 21303, 21608, 21609, 21610, 21665, 21688, 21711, 21734, 21757, 21780, 27192, 22111,
	}, Hooks: ddMagic2AggressiveHooks()}
}

// ddMagic2AggressiveHooks are DDMagic2Aggressive's hooks, which its
// children derive theirs from.
func ddMagic2AggressiveHooks() script.Hooks {
	base := ddMagic2Hooks()
	return base.With(script.Hooks{
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
	}).Named("WizardDDMagic2Aggressive")
}

// DDMagic2DispellAggressive is WizardDDMagic2DispellAggressive: a
// WizardDDMagic2Aggressive that, until it gives up its spells, may answer a
// playable's hit, while it hates someone, with its dispell on the
// attacker, on a draw under 33; short of MP it gives up its spells and
// attacks instead.
func DDMagic2DispellAggressive() script.Script {
	base := ddMagic2AggressiveHooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{21663, 21686, 21709, 21732, 21755, 21778}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 {
				if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil && s.Rnd(100) < 33 {
					castOrAttack(n, attacker, npc.SkillTypeDispell, false)
				}
			}
			base.Attacked(s, e)
		},
	})}
}

// hallSpell makes n want to cast at c its long range DD magic when c
// stands farther than 100, else its short range one; short of MP it gives
// up its spells and attacks c instead.
func hallSpell(n *script.NPC, c script.Creature) {
	if n.Distance2D(c) > 100 {
		castOrAttack(n, c, npc.SkillTypeWLongRangeDDMagic, false)
	} else {
		castOrAttack(n, c, npc.SkillTypeWShortRangeDDMagic, false)
	}
}

// HallDDMagic2Aggressive is WizardHallDDMagic2Aggressive: a clan hall
// WizardDDMagic2Aggressive that neither starts its parent's timers nor
// wanders. It strikes by distance, with its long range DD magic beyond
// 100, else its short range one, what hits it or its clan member and a
// playable it sees, dropping its walks and hating that playable 200. On a
// draw under 50 it drops everything to strike a cleric it sees cast. A
// guard's event names a creature to drop everything and strike (10033) or
// one to walk to (10042). Short of MP it gives up its spells and attacks
// instead.
func HallDDMagic2Aggressive() script.Script {
	base := ddMagic2AggressiveHooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{18330, 18338}, Hooks: base.With(script.Hooks{
		OnCreated:  func(*script.Script, script.Created) {},
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			hallSpell(e.NPC, e.Attacker)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if s.Rnd(100) < 50 && caster.InClassGroup("@cleric_group") {
				n.RemoveAllDesire()
				hallSpell(n, caster)
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.Arg1 {
			case argHallStrike:
				if c0 := s.Object(e.Arg2); c0 != nil {
					n.RemoveAllDesire()
					hallSpell(n, c0)
				}
			case argHallMoveTo:
				if c0 := s.Object(e.Arg2); c0 != nil {
					n.AddMoveToDesire(script.LocOf(c0), 10000000)
				}
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			n.RemoveDesires(script.IntentionMoveTo)
			hallSpell(n, c)
			n.HateList().Add(c, 200)
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			hallSpell(e.Called, e.Attacker)
		},
	})}
}
