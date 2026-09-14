package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SummonCreature is the SUMMON_CREATURE skill handler's entry point
// (handler/skill/summon.go's creatureSummonRuntime), matching Java's
// SummonCreature.useSkill: only a pet-collar-item cast reaches here, so a
// non-*item.Instance item is a silent no-op, same as Java's item==nil /
// getSummonItem==null early returns.
func (c *Character) SummonCreature(_ modelskill.Definition, itemArg any) {
	inst, ok := itemArg.(*item.Instance)
	if !ok {
		return
	}
	c.emit(event.PetSummonRequested{ControlItem: inst})
}

// SummonServitor is the non-cubic SUMMON skill handler's entry point.
func (c *Character) SummonServitor(def modelskill.Definition) {
	c.emit(event.ServitorSummonRequested{Skill: def})
}
