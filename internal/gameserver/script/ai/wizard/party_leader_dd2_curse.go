package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PartyLeaderDD2CurseAggressive is PartyLeaderWizardDD2CurseAggressive: a
// PartyLeaderWizardDD2 that, settled in its territory with an empty hate
// list, curses the playable it sees and strikes it with the DD magic its
// range calls for. It may also curse its attackers and those of its clan.
func PartyLeaderDD2CurseAggressive() script.Script {
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21090}, Hooks: partyLeaderDD2CurseAggressiveHooks()}
}

// maybeCurse makes n, on a draw under 33 when c holds no effect of its
// debuff yet, want to cast that debuff at c, or attack c when it cannot pay
// for it.
func maybeCurse(s *script.Script, n *script.NPC, c script.Creature) {
	debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
	if s.Rnd(100) < 33 && s.AbnormalLevel(c, debuff) <= 0 {
		castOrAttack(n, c, npc.SkillTypeDebuff, false)
	}
}

// partyLeaderDD2CurseAggressiveHooks are
// PartyLeaderWizardDD2CurseAggressive's hooks, which its children derive
// theirs from.
func partyLeaderDD2CurseAggressiveHooks() script.Hooks {
	base := partyLeaderDD2Hooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			hateList := n.HateList()
			if n.LifeTime() > 7 && hateList.Empty() && n.InMyTerritory() {
				maybeCurse(s, n, c)
				dd2Spell(n, c)
				hateList.AddDefault(c)
				base.SeeCreature(s, e)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 && n.HateList().MostHated() != nil {
				maybeCurse(s, n, attacker)
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
	}).Named("PartyLeaderWizardDD2CurseAggressive")
}
