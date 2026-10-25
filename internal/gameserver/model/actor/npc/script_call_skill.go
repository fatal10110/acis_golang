package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// skillCaller is the cast seam a script's skill call takes.
type skillCaller interface {
	CallSkill(target attackable.Combatant, ref skill.Ref)
}

// CallSkill makes h use ref on target at once, as a script asks: the
// players around h see the cast start, then the skill's effects land on
// target alone, with no cast time, cost, reuse or check. It runs on the
// caller's goroutine. An NPC with no cast runtime does nothing.
func (h *Hostile) CallSkill(target attackable.Combatant, ref skill.Ref) {
	if c, ok := h.brain.CastController().(skillCaller); ok {
		c.CallSkill(target, ref)
	}
}

// CallSkill makes f use ref on target as CallSkill of Hostile does.
func (f *Folk) CallSkill(target attackable.Combatant, ref skill.Ref) {
	if c, ok := f.cast.castAI.(skillCaller); ok {
		c.CallSkill(target, ref)
	}
}
