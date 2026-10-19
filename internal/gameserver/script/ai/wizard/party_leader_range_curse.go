package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderRangeCurseAggressive is PartyLeaderWizardRangeCurseAggressive:
// an aggressive PartyLeaderWizardDD2 that buffs its party on the first hit
// and may strike an attacker other than its top desire target with its
// range DD magic. Five seconds after its first hit, unless hit again in
// between (which defers the check by five more), it curses its top desire
// target; its death lands its debuff cancel on the creature it cursed.
func PartyLeaderRangeCurseAggressive() script.Script {
	base := partyLeaderDD2Hooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21312}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI1, 1)
			n.SetInt(script.SlotIntAI3, 0)
			n.SetInt(script.SlotIntAI4, 0)
			n.SetSlot(script.SlotCreature2, n)
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
			if n.Int(script.SlotIntAI1) == 1 {
				castByType(n, n, npc.SkillTypeRangeBuff, 1000000)
				n.SetInt(script.SlotIntAI1, 0)
			}
			if n.Int(script.SlotIntAI4) == 0 {
				if n.Int(script.SlotIntAI3) == 0 {
					n.SetInt(script.SlotIntAI3, 1)
					s.StartTimer("5001", n, nil, 5000*time.Millisecond)
				} else if n.Int(script.SlotIntAI3) == 1 {
					n.SetInt(script.SlotIntAI4, 1)
				}
			}
			if top := n.TopDesireTarget(); top != nil && !script.Same(top, attacker) && s.Rnd(100) < 10 {
				castOrAttack(n, attacker, npc.SkillTypeRangeDD, false)
			}
			base.Attacked(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			if c := n.Slot(script.SlotCreature2); c != nil && !script.Same(c, n) {
				if cancel, ok := n.SkillByType(npc.SkillTypeDebuffCancel); ok {
					s.ApplyEffects(cancel, c, c)
				}
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "5001") && n.Int(script.SlotIntAI3) == 1 {
				switch n.Int(script.SlotIntAI4) {
				case 1:
					s.StartTimer("5001", n, nil, 5000*time.Millisecond)
					n.SetInt(script.SlotIntAI4, 0)
				case 0:
					if top := n.TopDesireTarget(); top != nil {
						castByType(n, top, npc.SkillTypeDebuff, 9999999)
						n.SetSlot(script.SlotCreature2, top)
					}
				}
			}
			return base.Timer(s, e)
		},
	})}
}
