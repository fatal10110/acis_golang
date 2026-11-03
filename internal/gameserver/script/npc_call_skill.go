package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// npcSkillCaller is an NPC a script makes use a skill: a hostile or a
// civilian one.
type npcSkillCaller interface {
	CallSkill(target attackable.Combatant, ref skill.Ref)
}

// CallSkill makes the NPC, hostile or civilian, use ref on target:
// everyone watching the NPC sees the cast, and the skill's effects land on
// target with no cast time, cost, reuse or target check, as CallSkill of
// Script does for a player. The NPC's own cast, if one is in flight, goes
// on. It runs inside the call, on the caller's queue, so the cast is shown
// before what the rest of the hook sends; an effect the skill's handler
// leaves to a player target's own queue (a teleport) lands once the hook's
// task is done. No target, or a skill with no data, does nothing.
func (n *NPC) CallSkill(target Creature, ref skill.Ref) {
	c, ok := n.combatant().(npcSkillCaller)
	if !ok {
		panic("script: the skill call of a handle on nothing")
	}
	c.CallSkill(combatantOf(target), ref)
}
