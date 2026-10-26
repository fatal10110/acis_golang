package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// DDMagic2Heal is WizardDDMagic2Heal: a WizardDDMagic2 that may heal itself
// after a playable's hit below 70% of its HP, on a draw under 33. A settled
// one with an empty hate list may heal its clan member a playable hits, on
// a draw under 33.
func DDMagic2Heal() script.Script {
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{
		20292, 20807, 21414, 20657, 20923, 20576, 20633, 22030, 22037, 22125,
	}, Hooks: ddMagic2HealHooks()}
}

// ddMagic2HealHooks are DDMagic2Heal's hooks, which its children derive
// theirs from.
func ddMagic2HealHooks() script.Hooks {
	base := ddMagic2Hooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n := e.NPC
			if e.Attacker.Playable() && s.Rnd(100) < 33 && n.HPRatio() < 0.7 {
				castByType(n, n, npc.SkillTypeMagicHeal, 1000000)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called := e.Caller, e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 && called.HateList().Empty() && s.Rnd(100) < 33 {
				castByType(called, caller, npc.SkillTypeMagicHeal, 1000000)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WizardDDMagic2Heal")
}

// DDMagic2HealAggressive is WizardDDMagic2HealAggressive: a
// WizardDDMagic2Heal that, settled in its territory with an empty hate
// list, strikes a playable it sees with its long range DD magic beyond
// 100, else its short range one, unchecked, and hates it; short of MP it
// gives up its spells and attacks instead.
func DDMagic2HealAggressive() script.Script {
	base := ddMagic2HealHooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{21423, 22086}, Hooks: base.With(script.Hooks{
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

// lightBearer reports whether p follows one of the light's occupations:
// paladin (5), bishop (16), elven elder (30), phoenix knight (90),
// cardinal (97) or Eva's saint (105).
func lightBearer(p *script.Player) bool {
	switch p.ClassID() {
	case 5, 16, 30, 90, 97, 105:
		return true
	}
	return false
}

// DDMagic2Solina is WizardDDMagic2Solina: a WizardDDMagic2Heal that greets
// a character of the light's occupations it sees, once until it has
// nothing left to do, with one of two blessings (10075 or 10076, by a draw
// under 50), and strikes any other character it sees with its long range
// DD magic beyond 100 on a draw under 80, else its short range one, short
// of MP giving up its spells and attacking instead; it hates every
// character it sees. Hit by a character of those occupations it reproaches
// it, once until it has nothing left to do, with one of two lines (10077 or
// 10078, by a draw under 50).
func DDMagic2Solina() script.Script {
	base := ddMagic2HealHooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{22134}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI3, 0)
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			e.NPC.SetInt(script.SlotIntAI3, 0)
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.NoDesire(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if p, ok := c.(*script.Player); ok {
				switch {
				case lightBearer(p):
					if n.Int(script.SlotIntAI3) == 0 {
						if s.Rnd(100) < 50 {
							n.Say(script.NpcString(10075, p.Name()))
						} else {
							n.Say(script.NpcString(10076, p.Name()))
						}
						n.SetInt(script.SlotIntAI3, 1)
					}
				case n.Distance2D(c) > 100 && s.Rnd(100) < 80:
					castOrAttack(n, c, npc.SkillTypeWLongRangeDDMagic, false)
				default:
					castOrAttack(n, c, npc.SkillTypeWShortRangeDDMagic, false)
				}
				n.HateList().AddDefault(c)
			}
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if p, ok := e.Attacker.(*script.Player); ok && n.Int(script.SlotIntAI4) == 0 && lightBearer(p) {
				if s.Rnd(100) < 50 {
					n.Say(script.NpcString(10077, p.Name()))
				} else {
					n.Say(script.NpcString(10078, p.Name()))
				}
				n.SetInt(script.SlotIntAI4, 1)
			}
			base.Attacked(s, e)
		},
	})}
}
