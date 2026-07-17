package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// baseBuffSlots is the non-toggle, non-seven-signs buff-slot count every
// character starts with. Passive skills that raise this bound aren't wired
// onto Character yet, so it is currently also the character's permanent cap.
const baseBuffSlots = 20

// Character satisfies the actor surface skill target resolution needs.
var _ target.Creature = (*Character)(nil)

// EffectList returns c's active buffs and debuffs.
func (c *Character) EffectList() *effect.List {
	return c.effects
}

// MaxBuffCount is the number of non-toggle, non-seven-signs buffs c can
// hold at once. See baseBuffSlots.
func (c *Character) MaxBuffCount() int {
	return baseBuffSlots
}

// AddStatFuncs records fns as attached to c. Folding them into c's computed
// combat stats (PAtk, PDef, and the rest of the attribute chain) is not
// wired yet, so an active effect's stat bonuses have no observable effect
// on c until that lands; this only keeps add/remove bookkeeping correct in
// the meantime.
func (c *Character) AddStatFuncs(fns []basefunc.Func) {
	if len(fns) == 0 {
		return
	}
	c.statMu.Lock()
	defer c.statMu.Unlock()
	c.statFuncs = append(c.statFuncs, fns...)
}

// RemoveStatsByOwner drops every stat func previously added for owner.
func (c *Character) RemoveStatsByOwner(owner any) {
	c.statMu.Lock()
	defer c.statMu.Unlock()
	kept := c.statFuncs[:0]
	for _, f := range c.statFuncs {
		if f.Owner() != owner {
			kept = append(kept, f)
		}
	}
	c.statFuncs = kept
}

// Category reports c as a playable actor for skill target resolution.
func (c *Character) Category() target.Category {
	return target.CategoryPlayable
}

// Invul reports whether c is currently invulnerable. Always false: the
// invulnerability effect that would set this state isn't wired onto
// Character yet.
func (c *Character) Invul() bool {
	return false
}

// SkillSuccessInput returns the effect-landing roll input for def cast
// against c. Only the skill's own base land rate and ignore-resists flag
// are resolved from real skill data; the stat-derived modifiers (c's
// resistance, the caster's magic-attack ratio, the level difference)
// default to neutral until Character's stat-calculation chain is wired up,
// so an effect-landing roll against c is currently less accurate than the
// full formula but never fabricated.
func (c *Character) SkillSuccessInput(caster any, def modelskill.Definition) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{
		BaseChance:    float64(def.BaseLandRate),
		StatModifier:  1,
		VulnModifier:  1,
		MAtkModifier:  1,
		LevelModifier: 1,
		IgnoreResists: def.IgnoreResists,
	}, true
}
