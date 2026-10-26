package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const corpseNecroDir = "ai/individual/Monster/WizardBase/Wizard/WizardDDMagic2/WizardCorpseNecro"

// CorpseNecro is WizardCorpseNecro: a WizardDDMagic2 that, hit by a
// playable while it has a top desire target, wants its debuff on the
// attacker and remembers it (creature slot 0) above 90% of its HP unless
// it attacks, and may want its DD magic on its top desire target that hits
// it below 40% of its HP, on a draw under 33. Dying within 100 of the
// creature it remembers, its cancel lands on that creature, without its
// parent's death line.
func CorpseNecro() script.Script {
	base := ddMagic2Hooks()
	return script.Script{Dir: corpseNecroDir, Behavior: true, NPCs: []int32{21565, 21580, 21584, 21558}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if top := n.TopDesireTarget(); top != nil {
					hpRatio := n.HPRatio()
					if hpRatio > 0.9 && n.Intention() != script.IntentionAttack {
						castByType(n, attacker, npc.SkillTypeDebuff, 1000000)
						n.SetSlot(script.SlotCreature0, attacker)
					}
					if script.Same(top, attacker) && hpRatio < 0.4 && s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypeDDMagic, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			if c := n.Slot(script.SlotCreature0); c != nil && n.Distance2D(c) < 100 {
				if cancel, ok := n.SkillByType(npc.SkillTypeCancel); ok {
					s.ApplyEffects(cancel, c, c)
				}
			}
		},
	})}
}
