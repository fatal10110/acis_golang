package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const saintBasicDir = "ai/individual/Monster/WizardBase/Wizard/WizardSaintBasic"

// SaintBasic is WizardSaintBasic: a Wizard that wants its buff on itself as
// it appears and hates a playable hitting it by the damage, 300 more when
// it hated no one. One with a silhouette, hit between 50% and 70% of its HP,
// may call it once, on a draw under 30, turning on the attacker, and leave
// the world. Until it gives up its spells, a playable's hit while it hates
// someone wants its short range DD magic, unchecked, on the attacker and,
// on a draw under 33, its debuff when the attacker holds none of its
// effect; after each cast it does the same to its most hated. Once it gave
// up its spells it wants to attack the playable by the damage. A settled
// one with an empty hate list answers a playable's hit on its clan member
// with its debuff or its short range DD magic, both unchecked. Short of MP
// it gives up its spells and attacks instead.
func SaintBasic() script.Script {
	return script.Script{Dir: saintBasicDir, Behavior: true, NPCs: []int32{21521, 21536, 21520, 21530, 21533}, Hooks: saintBasicHooks()}
}

// saintDebuffs reports whether n, on a draw under 33, goes for its debuff on
// target: target holds none of its effect.
func saintDebuffs(s *script.Script, n *script.NPC, target script.Creature) bool {
	debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
	return s.Rnd(100) < 33 && s.AbnormalLevel(target, debuff) <= 0
}

// saintStrike makes n want its short range DD magic, unchecked, on target
// and maybe its debuff.
func saintStrike(s *script.Script, n *script.NPC, target script.Creature) {
	castOrAttack(n, target, npc.SkillTypeWShortRangeDDMagic, true)
	if saintDebuffs(s, n, target) {
		castOrAttack(n, target, npc.SkillTypeDebuff, false)
	}
}

// saintBasicHooks are SaintBasic's hooks, which its children derive theirs
// from.
func saintBasicHooks() script.Hooks {
	base := WizardHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI4, 0)
			castByType(n, n, npc.SkillTypeBuff, 1000000)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
			if attacker.Playable() {
				if n.HateList().Empty() {
					n.HateList().Add(attacker, (f0*100)+300)
				} else {
					n.HateList().Add(attacker, f0*100)
				}
			}
			if n.IntParam("silhouette") != 0 && n.Int(script.SlotIntAI4) == 0 {
				if hpRatio := n.HPRatio(); hpRatio < 0.7 && hpRatio > 0.5 && s.Rnd(100) < 30 {
					n.SetInt(script.SlotIntAI4, 1)
					n.SetSlot(script.SlotCreature0, attacker)
					at := script.LocOf(n)
					// The silhouette is placed at the caller's Y as its height.
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("silhouette"), script.Loc{X: at.X, Y: at.Y, Z: at.Y, Heading: at.Heading}, 0, script.Params{P1: 1000, P2: attacker.ObjectID()})
					n.DeleteMe()
					return
				}
			}
			base.Attacked(s, e)
			if attacker.Playable() {
				if n.Int(script.SlotIntAI0) == 0 {
					if n.HateList().MostHated() != nil {
						saintStrike(s, n, attacker)
					}
				} else {
					n.AddAttackDesire(attacker, f0*100)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.HateList().Empty() {
				if saintDebuffs(s, called, attacker) {
					castOrAttack(called, attacker, npc.SkillTypeDebuff, true)
				} else {
					castOrAttack(called, attacker, npc.SkillTypeWShortRangeDDMagic, true)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			mostHatedHI := n.HateList().MostHated()
			if mostHatedHI == nil {
				return
			}
			if n.Int(script.SlotIntAI0) == 0 {
				saintStrike(s, n, mostHatedHI)
			} else if mostHatedHI.Playable() {
				n.AddAttackDesire(mostHatedHI, 1000)
			}
		},
	}).Named("WizardSaintBasic")
}

// SaintBasicAggressive is WizardSaintBasicAggressive: a WizardSaintBasic
// that, settled in its territory with an empty hate list, hates a playable
// it sees and, within 40, wants its short range DD magic on it, unchecked;
// short of MP it gives up its spells and attacks instead.
func SaintBasicAggressive() script.Script {
	base := saintBasicHooks()
	return script.Script{Dir: saintBasicDir, Behavior: true, NPCs: []int32{21526}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			hateList := n.HateList()
			if n.LifeTime() > 7 && hateList.Empty() && n.InMyTerritory() {
				if n.Distance2D(c) < 40 {
					castOrAttack(n, c, npc.SkillTypeWShortRangeDDMagic, true)
				}
				hateList.AddDefault(c)
			}
			base.SeeCreature(s, e)
		},
	})}
}
