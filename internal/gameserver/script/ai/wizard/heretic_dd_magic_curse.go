package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// maybeDebuff1 makes n, on a draw under 33 while attacker holds no effect
// of its first debuff, want that debuff on attacker; short of MP it gives
// up its spells and attacks instead. hated gates the debuff after the
// draw.
func maybeDebuff1(s *script.Script, n *script.NPC, attacker script.Creature, hated bool) {
	debuff1, _ := n.SkillByType(npc.SkillTypeDebuff1)
	if s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff1) <= 0 && hated {
		castOrAttack(n, attacker, npc.SkillTypeDebuff1, false)
	}
}

// HereticDDMagicCurse is WizardHereticDDMagicCurse: a WizardDDMagic2Curse
// that the calls of its neighbours steer as they steer
// WizardHereticDDMagic2Curse. On a draw under 33 it wants its first debuff
// on whatever it sees. Until it gives up its spells, a playable's hit
// while it hates someone may bring that debuff on the attacker, on a draw
// under 33 while the attacker holds none of it; a settled one with an
// empty hate list answers a playable's hit on its clan member the same
// way. Short of MP it gives up its spells and attacks instead.
func HereticDDMagicCurse() script.Script {
	base := ddMagic2CurseHooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{22150}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			hereticEngage(n)
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 {
				maybeDebuff1(s, n, attacker, !n.HateList().Empty())
			}
			base.Attacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if s.Rnd(100) < 33 {
				castByType(e.NPC, e.Creature, npc.SkillTypeDebuff1, 1000000)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			hereticEngage(called)
			if called.LifeTime() > 7 && attacker.Playable() && called.HateList().Size() == 0 {
				maybeDebuff1(s, called, attacker, true)
			}
			base.ClanAttacked(s, e)
		},
		OnOutOfTerritory: hereticOutOfTerritory,
		OnScriptEvent:    hereticCallEvent,
	})}
}
