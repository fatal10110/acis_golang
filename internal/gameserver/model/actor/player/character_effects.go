package player

import (
	"math"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
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

// Invul reports whether c is currently invulnerable. Always false: the
// invulnerability effect that would set this state isn't wired onto
// Character yet.
func (c *Character) Invul() bool {
	return false
}

// SkillSuccessInput returns the effect-landing roll input for def cast
// against c.
func (c *Character) SkillSuccessInput(caster any, def modelskill.Definition) (formulas.SkillSuccessInput, bool) {
	attacker, ok := caster.(*Character)
	if !ok || attacker == nil {
		return formulas.SkillSuccessInput{}, false
	}
	return formulas.SkillSuccessInput{
		BaseChance:    float64(def.BaseLandRate),
		StatModifier:  c.skillStatModifier(def.EffectType, def.Magic),
		VulnModifier:  c.skillVulnerability(def.EffectType, def),
		MAtkModifier:  c.skillMAtkModifier(attacker, def),
		LevelModifier: c.skillLevelModifier(attacker, def),
		IgnoreResists: def.IgnoreResists,
	}, true
}

func (c *Character) skillStatModifier(typ string, magic bool) float64 {
	switch skillTypeKey(typ) {
	case "STUN", "BLEED", "POISON":
		return math.Max(0, 2-math.Sqrt(statbonus.CONBonus[c.CON()]))
	case "SLEEP", "DEBUFF", "WEAKNESS", "ERASE", "ROOT", "MUTE", "FEAR", "BETRAY", "CONFUSION", "AGGREDUCE_CHAR", "PARALYZE":
		if magic {
			return math.Max(0, 2-math.Sqrt(statbonus.MENBonus[c.MEN()]))
		}
	}
	return 1
}

// skillVulnerability returns the target's success-roll vulnerability
// multiplier for typ, folding in def's elemental resistance modifier (as
// its square root) as the base every stat-specific vulnerability builds on.
func (c *Character) skillVulnerability(typ string, def modelskill.Definition) float64 {
	base := math.Sqrt(c.elementalSkillModifier(def))
	switch skillTypeKey(typ) {
	case "BLEED":
		return c.calcStat(stat.BleedVuln, base)
	case "POISON":
		return c.calcStat(stat.PoisonVuln, base)
	case "STUN":
		return c.calcStat(stat.StunVuln, base)
	case "PARALYZE":
		return c.calcStat(stat.ParalyzeVuln, base)
	case "ROOT":
		return c.calcStat(stat.RootVuln, base)
	case "SLEEP":
		return c.calcStat(stat.SleepVuln, base)
	case "MUTE", "FEAR", "BETRAY", "AGGDEBUFF", "AGGREDUCE_CHAR", "ERASE", "CONFUSION":
		return c.calcStat(stat.DerangementVuln, base)
	case "DEBUFF", "WEAKNESS":
		return c.calcStat(stat.DebuffVuln, base)
	case "CANCEL":
		return c.calcStat(stat.CancelVuln, base)
	default:
		return base
	}
}

func (c *Character) skillMAtkModifier(attacker *Character, def modelskill.Definition) float64 {
	if !def.Magic {
		return 1
	}
	mDef := c.MDef()
	if mDef <= 0 {
		mDef = 1
	}
	return math.Sqrt(attacker.MAtk()) / mDef * 11
}

func (c *Character) skillLevelModifier(attacker *Character, def modelskill.Definition) float64 {
	if def.LevelDepend == 0 {
		return 1
	}
	level := attacker.CharLevel
	if def.MagicLevel > 0 {
		level = def.MagicLevel
	}
	delta := level + def.LevelDepend - c.CharLevel
	scale := 0.005
	if delta < 0 {
		scale = 0.01
	}
	return 1 + scale*float64(delta)
}

func skillTypeKey(s string) string {
	return strings.ToUpper(s)
}
