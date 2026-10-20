package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The script events a heretic private answers from its master: a call
// for help on a target, and a call to walk home.
const (
	eventHelpCall = 10033
	eventWalkHome = 10035
)

// hereticEngage sets n's engaged flag (int slot 4) when a hit finds it
// neither attacking nor casting with the flag still clear.
func hereticEngage(n *script.NPC) {
	if !fighting(n.Intention()) && n.Int(script.SlotIntAI4) == 0 {
		n.SetInt(script.SlotIntAI4, 1)
	}
}

// hereticOutOfTerritory walks n home, dropping its attack desires, unless
// it is engaged.
func hereticOutOfTerritory(_ *script.Script, e script.OutOfTerritory) {
	n := e.NPC
	if n.Int(script.SlotIntAI4) == 0 {
		n.RemoveAllAttackDesire()
		n.AddMoveToDesire(n.SpawnLocation(), 100)
	}
}

// hereticScriptEvent answers n's master. A call for help (10033) or a
// shouted target (10002) on a creature in the world drops n's attack
// desires and puts a playable target on its hate list by 200; the call for
// help also engages n. A call to walk home (10035) clears the engaged flag,
// drops its attack desires and walks it home. Events from anyone else are
// ignored.
func hereticScriptEvent(s *script.Script, e script.ScriptEvent) {
	n := e.NPC
	if !n.HasMaster() || e.Arg2 != n.Master().ObjectID() {
		return
	}
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
		if c0.Playable() {
			n.HateList().Add(c0, 200)
		}
	case eventWalkHome:
		n.SetInt(script.SlotIntAI4, 0)
		n.RemoveAllAttackDesire()
		n.AddMoveToDesire(n.SpawnLocation(), 100)
	}
}

// PartyPrivateHereticDD2 is PartyPrivateWizardHereticDD2: a
// PartyPrivateWizardDD2 that its master's calls steer. A hit on it or its
// party while it neither attacks nor casts engages it; out of its
// territory unengaged it walks home. Its master's call for help or shouted
// target turns it on that target, and its master's call to walk home
// disengages it and walks it home.
func PartyPrivateHereticDD2() script.Script {
	base := partyPrivateDD2Hooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22166}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			hereticEngage(e.NPC)
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			hereticEngage(e.Called)
			base.PartyAttacked(s, e)
		},
		OnOutOfTerritory: hereticOutOfTerritory,
		OnScriptEvent:    hereticScriptEvent,
	})}
}

// PartyPrivateHereticDD2Curse is PartyPrivateWizardHereticDD2Curse: a
// PartyPrivateWizardDD2Curse its master's calls steer as they steer
// PartyPrivateWizardHereticDD2.
func PartyPrivateHereticDD2Curse() script.Script {
	base := partyPrivateDD2CurseHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22165, 22168, 22173}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			hereticEngage(e.NPC)
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			hereticEngage(e.Called)
			base.PartyAttacked(s, e)
		},
		OnOutOfTerritory: hereticOutOfTerritory,
		OnScriptEvent:    hereticScriptEvent,
	})}
}

// PartyPrivateHereticCoward is PartyPrivateWizardHereticCoward: a
// PartyPrivateWizardDD2Curse its master's calls steer as they steer
// PartyPrivateWizardHereticDD2, and that leaves the world at once, no
// corpse left, when its master dies.
func PartyPrivateHereticCoward() script.Script {
	base := partyPrivateDD2CurseHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22158, 22162, 22174}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			hereticEngage(e.NPC)
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			hereticEngage(e.Called)
			base.PartyAttacked(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if script.Same(caller, called.Master()) {
				called.AddFleeDesire(caller, 500, 10000000)
				called.DeleteMe()
			}
			base.PartyDied(s, e)
		},
		OnOutOfTerritory: hereticOutOfTerritory,
		OnScriptEvent:    hereticScriptEvent,
	})}
}
