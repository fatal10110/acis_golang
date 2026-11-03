package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PartyLeaderHereticAggressive is PartyLeaderWizardHereticAggressive: a
// PartyLeaderWizardDD2CurseAggressive that keeps to its spawn point and
// calls for help (event 10033) within its HelpCastRange when it is hit or
// sees a creature before it fights. Its party under attack, it may look
// around below 40% of its HP until its self heal has landed. Out of its
// territory it drops its hate, walks home and tells those within its
// DistNoDesire (event 10035).
func PartyLeaderHereticAggressive() script.Script {
	base := partyLeaderDD2CurseAggressiveHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{22163, 22171}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI3, 0)
			base.Created(s, e)
			n.AddMoveToDesire(n.SpawnLocation(), 30)
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if !fighting(n.Intention()) {
				s.BroadcastScriptEventEx(n, 10033, e.Attacker.ObjectID(), n.ObjectID(), int(n.IntParamOr("HelpCastRange", 500)))
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called := e.Called
			if called.HPRatio() < 0.4 && s.Rnd(100) < 33 && called.Int(script.SlotIntAI3) == 0 {
				called.LookNeighbor()
			}
			base.PartyAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			// An NPC is never playable: this branch never runs.
			if c.Playable() && script.Same(c, n) {
				castByType(n, c, npc.SkillTypeSpecialAttack, 1000000)
				castByType(n, n, npc.SkillTypeSelfMagicHeal, 1000000)
			}
			if n.LifeTime() > 7 && n.InMyTerritory() && n.HateList().Size() == 0 && !fighting(n.Intention()) {
				s.BroadcastScriptEventEx(n, 10033, c.ObjectID(), n.ObjectID(), int(n.IntParamOr("HelpCastRange", 500)))
			}
			base.SeeCreature(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if heal, ok := n.SkillByType(npc.SkillTypeSelfMagicHeal); ok && e.Skill == heal && e.Success {
				n.SetInt(script.SlotIntAI3, 1)
			}
			base.UseSkillFinished(s, e)
		},
		OnOutOfTerritory: func(s *script.Script, e script.OutOfTerritory) {
			n := e.NPC
			n.RemoveAllAttackDesire()
			n.AddMoveToDesire(n.SpawnLocation(), 100)
			n.HateList().CleanAll()
			s.BroadcastScriptEvent(n, 10035, n.ObjectID(), int(n.IntParamOr("DistNoDesire", 500)))
		},
	})}
}
