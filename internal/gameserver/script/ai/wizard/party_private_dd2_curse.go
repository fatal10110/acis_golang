package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PartyPrivateDD2Curse is PartyPrivateWizardDD2Curse: a
// PartyPrivateWizardDD2 that may curse a playable that hits it while its
// hate list holds a creature and the attacker holds no effect of its debuff
// yet, and a playable that hits its clan while its hate list holds none.
func PartyPrivateDD2Curse() script.Script {
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{20972}, Hooks: partyPrivateDD2CurseHooks()}
}

// partyPrivateDD2CurseHooks are PartyPrivateWizardDD2Curse's hooks, which
// its children derive theirs from.
func partyPrivateDD2CurseHooks() script.Hooks {
	base := partyPrivateDD2Hooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 && n.HateList().MostHated() != nil {
				debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
				if s.AbnormalLevel(attacker, debuff) <= 0 && s.Rnd(100) < 33 {
					castOrAttack(n, attacker, npc.SkillTypeDebuff, false)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.HateList().Empty() {
				maybeCurse(s, called, attacker)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("PartyPrivateWizardDD2Curse")
}
