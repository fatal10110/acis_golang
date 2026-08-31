package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var _ creature.RaidCurseAttacker = (*Character)(nil)

type skillDefinitions interface {
	Definition(modelskill.Ref) (modelskill.Definition, bool)
}

// SetRaidCursesDisabled records the npcs.properties DisableRaidCurse gate.
func (c *Character) SetRaidCursesDisabled(disabled bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.raidCursesDisabled = disabled
}

// SetSkillDefinitions records the loaded skill table raid-curse lookups use.
func (c *Character) SetSkillDefinitions(defs skillDefinitions) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.skillDefs = defs
}

// SetMagicSkillUseBroadcaster records the packet-layer hook that broadcasts
// a raid-curse MagicSkillUse animation. A nil hook is a silent no-op.
func (c *Character) SetMagicSkillUseBroadcaster(broadcast func(creature.MagicSkillUse)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastMagicSkillUse = broadcast
}

// TestCursesOnAttack applies this player's raid petrification and mounted
// anti-strider curses against target. True means the leftover physical hit
// must be cancelled.
func (c *Character) TestCursesOnAttack(target attackable.Combatant) bool {
	if c == nil {
		return false
	}
	c.stateMu.RLock()
	disabled := c.raidCursesDisabled
	skills := c.skillDefs
	broadcast := c.broadcastMagicSkillUse
	c.stateMu.RUnlock()
	return creature.TestCursesOnAttack(creature.RaidCurseInput{
		Attacker:  c,
		Target:    target,
		NPCID:     creature.NPCIDOf(target),
		Mounted:   c.Mounted(),
		Disabled:  disabled,
		Skills:    skills,
		Broadcast: broadcast,
	})
}
