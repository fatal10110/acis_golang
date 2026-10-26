package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// hereticCallEvent answers the calls of a heretic wizard's neighbours. A
// call for help (10033) or a shouted target (10002) on a creature in the
// world drops n's attack desires and strikes that creature by distance,
// putting a playable on its hate list by 200; the call for help also
// engages n. A call to walk home (10035) disengages n and walks it home.
func hereticCallEvent(s *script.Script, e script.ScriptEvent) {
	n := e.NPC
	switch e.EventID {
	case eventHelpCall, eventMasterTarget:
		c0 := s.Object(e.Arg1)
		if c0 == nil {
			return
		}
		if e.EventID == eventHelpCall {
			n.SetInt(script.SlotIntAI4, 1)
		}
		n.RemoveAllAttackDesire()
		hallSpell(n, c0)
		if c0.Playable() {
			n.HateList().Add(c0, 200)
		}
	case eventWalkHome:
		n.SetInt(script.SlotIntAI4, 0)
		n.RemoveAllAttackDesire()
		n.AddMoveToDesire(n.SpawnLocation(), 100)
	}
}

// HereticDDMagic2Aggressive is WizardHereticDDMagic2Aggressive: a
// WizardDDMagic2 that the calls of its neighbours steer as they steer
// WizardHereticDDMagic2Curse, and that, settled in its territory with
// nobody on its hate list, strikes a playable it sees with its long range
// DD magic beyond 100, else its short range one, and hates it; short of MP
// it gives up its spells and attacks instead.
func HereticDDMagic2Aggressive() script.Script {
	base := ddMagic2Hooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{22144, 22193}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.InMyTerritory() && n.HateList().Size() == 0 {
				hallSpell(n, c)
				n.HateList().AddDefault(c)
				base.SeeCreature(s, e)
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			hereticEngage(e.NPC)
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			hereticEngage(e.Called)
			base.ClanAttacked(s, e)
		},
		OnOutOfTerritory: hereticOutOfTerritory,
		OnScriptEvent:    hereticCallEvent,
	})}
}
