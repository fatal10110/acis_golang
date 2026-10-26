package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const corpseVampireDir = "ai/individual/Monster/WizardBase/Wizard/WizardDDMagic2/WizardCorpseVampireBasic"

// CorpseVampireBasic is WizardCorpseVampireBasic: a WizardDDMagic2 that,
// hit by a playable while it has a top desire target, wants its debuff on
// the attacker above 90% of its HP unless it attacks, and, once, may want
// its DD magic on its top desire target that hits it below 50% of its HP,
// on a draw under 33; that strike gives up its spells (int slot 0).
func CorpseVampireBasic() script.Script {
	return script.Script{Dir: corpseVampireDir, Behavior: true, NPCs: []int32{21576, 21422, 21590}, Hooks: corpseVampireBasicHooks()}
}

// corpseVampireBasicHooks are CorpseVampireBasic's hooks, which its
// children derive theirs from.
func corpseVampireBasicHooks() script.Hooks {
	base := ddMagic2Hooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if top := n.TopDesireTarget(); top != nil {
					hpRatio := n.HPRatio()
					if hpRatio > 0.9 && n.Intention() != script.IntentionAttack {
						castByType(n, attacker, npc.SkillTypeDebuff, 1000000)
					}
					if script.Same(top, attacker) && n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 33 && hpRatio < 0.5 {
						castByType(n, attacker, npc.SkillTypeDDMagic, 1000000)
						n.SetInt(script.SlotIntAI0, 1)
					}
				}
			}
			base.Attacked(s, e)
		},
	}).Named("WizardCorpseVampireBasic")
}

// CorpseVampireBasicAggressive is WizardCorpseVampireBasicAggressive: a
// WizardCorpseVampireBasic that, settled in its territory with an empty
// hate list, strikes a playable it sees with its long range DD magic
// beyond 100, else its short range one, unchecked, and hates it; short of
// MP it gives up its spells and attacks instead. Unsettled, it ignores the
// playable. It binds no NPC of its own.
func CorpseVampireBasicAggressive() script.Script {
	return script.Script{Dir: corpseVampireDir, Behavior: true, Hooks: corpseVampireBasicAggressiveHooks()}
}

// corpseVampireBasicAggressiveHooks are CorpseVampireBasicAggressive's
// hooks, which its child derives its from.
func corpseVampireBasicAggressiveHooks() script.Hooks {
	base := corpseVampireBasicHooks()
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
	}).Named("WizardCorpseVampireBasicAggressive")
}

// CorpseVampireCurseAggressive is WizardCorpseVampireCurseAggressive: a
// WizardCorpseVampireBasicAggressive whose first playable's hit wants its
// first debuff on the attacker and remembers it (creature slot 0); a later
// hit, below 10% of its HP with that creature within 100, wants its cancel
// on the creature, once. Int slot 4 holds the stage: 0 fresh, 1 cursed, 2
// cancelled.
func CorpseVampireCurseAggressive() script.Script {
	base := corpseVampireBasicAggressiveHooks()
	return script.Script{Dir: corpseVampireDir, Behavior: true, NPCs: []int32{21585}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if n.Int(script.SlotIntAI4) == 0 {
					castByType(n, attacker, npc.SkillTypeDebuff1, 1000000)
					n.SetInt(script.SlotIntAI4, 1)
					n.SetSlot(script.SlotCreature0, attacker)
				}
				if n.Int(script.SlotIntAI4) == 1 {
					if c0 := n.Slot(script.SlotCreature0); c0 != nil && n.HPRatio() < 0.1 && n.Distance2D(c0) < 100 {
						castByType(n, c0, npc.SkillTypeCancel, 1000000)
						n.SetInt(script.SlotIntAI4, 2)
					}
				}
			}
			base.Attacked(s, e)
		},
	})}
}

// CorpseVampireRange is WizardCorpseVampireRange: a WizardCorpseVampireBasic
// that, hit by a playable in a party while it has a top desire target, may
// want its self range DD magic on the attacker, unchecked, on a draw under
// 33; short of MP it gives up its spells and attacks instead.
func CorpseVampireRange() script.Script {
	return script.Script{Dir: corpseVampireDir, Behavior: true, NPCs: []int32{21591}, Hooks: corpseVampireRangeHooks()}
}

// corpseVampireRangeHooks are CorpseVampireRange's hooks, which its child
// derives its from.
func corpseVampireRangeHooks() script.Hooks {
	base := corpseVampireBasicHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.TopDesireTarget() != nil {
				if player := script.ActingPlayer(attacker); player != nil && player.InParty() && s.Rnd(100) < 33 {
					castOrAttack(n, attacker, npc.SkillTypeWSelfRangeDDMagic, true)
				}
			}
			base.Attacked(s, e)
		},
	}).Named("WizardCorpseVampireRange")
}

// CorpseVampireRangeAggressive is WizardCorpseVampireRangeAggressive: a
// WizardCorpseVampireRange that, settled in its territory with an empty
// hate list, strikes a playable it sees with its long range DD magic
// beyond 100, else its short range one, unchecked, and hates it; short of
// MP it gives up its spells and attacks instead. Unsettled, it ignores the
// playable.
func CorpseVampireRangeAggressive() script.Script {
	base := corpseVampireRangeHooks()
	return script.Script{Dir: corpseVampireDir, Behavior: true, NPCs: []int32{21586}, Hooks: base.With(script.Hooks{
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
