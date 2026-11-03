package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// QuestPartyLeader is QuestPartyLeader: a PartyLeaderWarrior that still
// creates its privates as it spawns but otherwise reacts to nothing: no
// desire, no timer, no hit on itself, a private or a clan member, and no
// spell it sees moves it.
func QuestPartyLeader() script.Script {
	base := PartyLeaderHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{27185, 27186, 27187, 27188}, Hooks: base.With(script.Hooks{
		OnNoDesire:      func(*script.Script, script.NoDesire) {},
		OnTimer:         func(*script.Script, script.Timer) string { return "" },
		OnPartyAttacked: func(*script.Script, script.PartyAttacked) {},
		OnAttacked:      func(*script.Script, script.Attacked) {},
		OnClanAttacked:  func(*script.Script, script.ClanAttacked) {},
		OnSeeSpell:      func(*script.Script, script.SeeSpell) {},
	})}
}

// RoyalRushPartyLeader is RoyalRushPartyLeader: a PartyLeaderWarrior that
// may strike the player it most wants to attack with its physical special
// skill and, once, call its SummonSlave private at its attacker's feet to
// attack that player.
func RoyalRushPartyLeader() script.Script {
	base := PartyLeaderHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21434}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) {
						castByType(n, attacker, npc.SkillTypePhysicalSpecial1)
					}
					if s.Rnd(100) < 33 && n.Int(script.SlotIntAI0) == 0 {
						at := script.LocOf(attacker)
						s.Spawner().CreateOnePrivateEx(n, n.IntParam("SummonSlave"), script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0, script.Params{P1: 1000, P2: topDesireTarget.ObjectID(), P3: 0})
						n.SetInt(script.SlotIntAI0, 1)
					}
				}
			}
			base.Attacked(s, e)
		},
	})}
}
