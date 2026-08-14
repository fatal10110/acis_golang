package player

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// baseBuffSlots is the non-toggle, non-seven-signs buff-slot count every
// character starts with. Passive skills that raise this bound aren't wired
// onto Character yet, so it is currently also the character's permanent cap.
const baseBuffSlots = 20

// Character satisfies the actor surface skill target resolution needs.
var _ target.Creature = (*Character)(nil)

// MaxBuffCount is the number of non-toggle, non-seven-signs buffs c can
// hold at once. See baseBuffSlots.
func (c *Character) MaxBuffCount() int {
	return baseBuffSlots
}

// AddStatFuncs attaches fns to c's live stat calculators.
func (c *Character) AddStatFuncs(fns []basefunc.Func) {
	if len(fns) == 0 {
		return
	}
	c.statMu.Lock()
	defer c.statMu.Unlock()
	for _, fn := range fns {
		if fn == nil {
			continue
		}
		c.statCalcLocked(fn.Stat()).AddFunc(fn)
	}
}

// RemoveStatsByOwner drops every stat func previously added for owner.
func (c *Character) RemoveStatsByOwner(owner any) {
	if owner == nil {
		return
	}
	c.statMu.Lock()
	defer c.statMu.Unlock()
	for _, calc := range c.statCalcs {
		calc.RemoveOwner(owner)
	}
}

// Category reports c as a playable actor for skill target resolution.
func (c *Character) Category() target.Category {
	return target.CategoryPlayable
}

// Invul reports whether c is currently invulnerable.
func (c *Character) Invul() bool {
	return c != nil && (c.SpawnProtected() || c.Live != nil && c.Live.Invul())
}

// SpawnProtected reports whether c currently has spawn protection.
func (c *Character) SpawnProtected() bool {
	if c == nil {
		return false
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.spawnProtected
}

// SetSpawnProtection sets or clears c's spawn protection.
func (c *Character) SetSpawnProtection(v bool) {
	if c == nil {
		return
	}
	c.stateMu.Lock()
	c.spawnProtected = v
	c.stateMu.Unlock()
}

// CanGiveDamage reports whether c's access level permits damage.
func (c *Character) CanGiveDamage() bool {
	if c == nil {
		return false
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return !c.damagePermissionSet || c.canGiveDamage
}

// SetCanGiveDamage sets the damage permission resolved from c's access level.
func (c *Character) SetCanGiveDamage(v bool) {
	if c == nil {
		return
	}
	c.stateMu.Lock()
	c.damagePermissionSet = true
	c.canGiveDamage = v
	c.stateMu.Unlock()
}

// SkillSuccessInput returns the effect-landing roll input for def cast
// against c, given the caster's blessed-spiritshot charge state (bss) and
// this cast's already-resolved shield-block outcome (shield).
func (c *Character) SkillSuccessInput(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return creature.ResolveSkillSuccessInput(caster, c, def, bss, shield)
}

func (c *Character) EffectSuccessInput(caster any, def modelskill.Definition, tmpl modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if tmpl.EffectType == "" {
		return formulas.SkillSuccessInput{BaseChance: tmpl.EffectPower, IgnoreResists: true, Shield: shield}, true
	}
	if strings.EqualFold(tmpl.EffectType, "CANCEL") {
		return formulas.SkillSuccessInput{BaseChance: 100, IgnoreResists: true, Shield: shield}, true
	}
	def.EffectType = tmpl.EffectType
	def.IgnoreResists = false
	in, ok := c.SkillSuccessInput(caster, def, bss, shield)
	in.BaseChance = tmpl.EffectPower
	return in, ok
}
