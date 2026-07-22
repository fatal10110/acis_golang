package summon

import (
	"math"
	"math/rand/v2"
	"sync"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/funcs"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// CombatStats carries the live combat and resource bases for a pet or servitor.
type CombatStats struct {
	STR, CON, DEX, INT, WIT, MEN int
	PAtk, PDef, MAtk, MDef       float64
	MaxHP, MaxMP                 float64
	BaseRandomDamage             int
}

type summonVitals struct {
	// mu guards hp and mp.
	mu     sync.RWMutex
	hp, mp float64
}

type summonStatCalcs struct {
	// mu guards calcs.
	mu    sync.Mutex
	calcs map[stat.Stat]*basefunc.Calculator
}

func (a *Actor) initVitals() {
	a.vitals.hp = a.MaxHPValue()
	a.vitals.mp = a.MaxMPValue()
}

// AddStatFuncs attaches fns to a's live stat calculators.
func (a *Actor) AddStatFuncs(fns []basefunc.Func) {
	if len(fns) == 0 {
		return
	}
	a.statCalc.mu.Lock()
	defer a.statCalc.mu.Unlock()
	for _, fn := range fns {
		if fn == nil {
			continue
		}
		a.statCalcLocked(fn.Stat()).AddFunc(fn)
	}
}

// RemoveStatsByOwner drops every stat func previously added for owner.
func (a *Actor) RemoveStatsByOwner(owner any) {
	if owner == nil {
		return
	}
	a.statCalc.mu.Lock()
	defer a.statCalc.mu.Unlock()
	for _, calc := range a.statCalc.calcs {
		calc.RemoveOwner(owner)
	}
}

func (a *Actor) statCalculator(s stat.Stat) *basefunc.Calculator {
	a.statCalc.mu.Lock()
	defer a.statCalc.mu.Unlock()
	return a.statCalcLocked(s)
}

func (a *Actor) statCalcLocked(s stat.Stat) *basefunc.Calculator {
	if a.statCalc.calcs == nil {
		a.statCalc.calcs = make(map[stat.Stat]*basefunc.Calculator)
	}
	if calc := a.statCalc.calcs[s]; calc != nil {
		return calc
	}
	calc := &basefunc.Calculator{}
	for _, fn := range defaultStatFuncs(s) {
		calc.AddFunc(fn)
	}
	a.statCalc.calcs[s] = calc
	return calc
}

func (a *Actor) calcStat(s stat.Stat, base float64) float64 {
	value := a.statCalculator(s).Calc(summonStatActor{a: a}, a, nil, base)
	if s.CantBeNegative() && value < 0 {
		return 0
	}
	return value
}

// CalcStat finalizes base for s through a's live stat calculator.
func (a *Actor) CalcStat(s stat.Stat, base float64) float64 {
	return a.calcStat(s, base)
}

func defaultStatFuncs(s stat.Stat) []basefunc.Func {
	switch s {
	case stat.MaxHP:
		return []basefunc.Func{funcs.MaxHpMul}
	case stat.MaxMP:
		return []basefunc.Func{funcs.MaxMpMul}
	case stat.RegenerateHPRate:
		return []basefunc.Func{funcs.RegenHpMul}
	case stat.RegenerateMPRate:
		return []basefunc.Func{funcs.RegenMpMul}
	case stat.PowerAttack:
		return []basefunc.Func{funcs.PAtkMod}
	case stat.PowerDefence:
		return []basefunc.Func{funcs.PDefMod}
	case stat.MagicAttack:
		return []basefunc.Func{funcs.MAtkMod}
	case stat.MagicDefence:
		return []basefunc.Func{funcs.MDefMod}
	case stat.PowerAttackSpeed:
		return []basefunc.Func{funcs.PAtkSpeed}
	case stat.MagicAttackSpeed:
		return []basefunc.Func{funcs.MAtkSpeed}
	case stat.AccuracyCombat:
		return []basefunc.Func{funcs.AtkAccuracy}
	case stat.EvasionRate:
		return []basefunc.Func{funcs.AtkEvasion}
	case stat.CriticalRate:
		return []basefunc.Func{funcs.AtkCritical}
	case stat.MCriticalRate:
		return []basefunc.Func{funcs.MAtkCritical}
	case stat.RunSpeed:
		return []basefunc.Func{funcs.MoveSpeed}
	default:
		return nil
	}
}

type summonStatActor struct{ a *Actor }

var _ funcs.Actor = summonStatActor{}

func (s summonStatActor) STR() int { return defaultInt(s.a.stats.STR, 40) }
func (s summonStatActor) CON() int { return defaultInt(s.a.stats.CON, 21) }
func (s summonStatActor) DEX() int { return defaultInt(s.a.stats.DEX, 30) }
func (s summonStatActor) INT() int { return defaultInt(s.a.stats.INT, 20) }
func (s summonStatActor) WIT() int { return defaultInt(s.a.stats.WIT, 43) }
func (s summonStatActor) MEN() int { return defaultInt(s.a.stats.MEN, 20) }

func (s summonStatActor) Level() int {
	if s.a.level <= 0 {
		return 1
	}
	return s.a.level
}

func (s summonStatActor) LevelMod() float64 {
	return (89 + float64(s.Level())) / 100
}

func (s summonStatActor) IsSummon() bool { return true }

// STR returns this summon's current STR attribute.
func (a *Actor) STR() int { return summonStatActor{a: a}.STR() }

// CON returns this summon's current CON attribute.
func (a *Actor) CON() int { return summonStatActor{a: a}.CON() }

// DEX returns this summon's current DEX attribute.
func (a *Actor) DEX() int { return summonStatActor{a: a}.DEX() }

// INT returns this summon's current INT attribute.
func (a *Actor) INT() int { return summonStatActor{a: a}.INT() }

// WIT returns this summon's current WIT attribute.
func (a *Actor) WIT() int { return summonStatActor{a: a}.WIT() }

// MEN returns this summon's current MEN attribute.
func (a *Actor) MEN() int { return summonStatActor{a: a}.MEN() }

// LevelMod returns this summon's level-scaling factor.
func (a *Actor) LevelMod() float64 { return summonStatActor{a: a}.LevelMod() }

// Category reports a pet or servitor as a playable actor.
func (a *Actor) Category() skilltarget.Category {
	return skilltarget.CategoryPlayable
}

// Playable reports whether a is player-controlled.
func (a *Actor) Playable() bool { return true }

// Invul reports whether a is currently invulnerable.
func (a *Actor) Invul() bool { return false }

// Invulnerable reports whether a ignores direct resource effects.
func (a *Actor) Invulnerable() bool { return a.Invul() }

// PAtk returns this summon's physical attack stat.
func (a *Actor) PAtk() float64 {
	return a.calcStat(stat.PowerAttack, positiveBase(a.stats.PAtk))
}

// PDef returns this summon's physical defence stat.
func (a *Actor) PDef() float64 {
	return a.calcStat(stat.PowerDefence, positiveBase(a.stats.PDef))
}

// MAtk returns this summon's magic attack stat.
func (a *Actor) MAtk() float64 {
	return a.calcStat(stat.MagicAttack, positiveBase(a.stats.MAtk))
}

// MDef returns this summon's magic defence stat.
func (a *Actor) MDef() float64 {
	return a.calcStat(stat.MagicDefence, positiveBase(a.stats.MDef))
}

// MagicCriticalRate returns this summon's magic critical rate.
func (a *Actor) MagicCriticalRate() float64 {
	return a.calcStat(stat.MCriticalRate, 8)
}

// AttackType returns this summon's current physical attack style.
func (a *Actor) AttackType() item.WeaponType { return item.WeaponFist }

// SoulshotCharged reports whether a soulshot charge is currently active.
func (a *Actor) SoulshotCharged() bool { return false }

// SpiritshotCharged reports whether a spiritshot charge is currently active.
func (a *Actor) SpiritshotCharged() bool { return false }

// BlessedSpiritshotCharged reports whether a blessed spiritshot charge is active.
func (a *Actor) BlessedSpiritshotCharged() bool { return false }

// Roll draws a uniform random integer in [0, n) from a's combat random source.
func (a *Actor) Roll(n int) int {
	if n <= 0 {
		return 0
	}
	if a.roll != nil {
		return a.roll(n)
	}
	return rand.IntN(n)
}

// RandomDamageSpread returns the summon's random-damage spread.
func (a *Actor) RandomDamageSpread() int {
	return a.stats.BaseRandomDamage
}

// HP returns current HP as a floating-point skill-resource value.
func (a *Actor) HP() float64 {
	a.vitals.mu.RLock()
	defer a.vitals.mu.RUnlock()
	return a.vitals.hp
}

// MaxHPValue returns maximum HP as a floating-point skill-resource value.
func (a *Actor) MaxHPValue() float64 {
	return a.calcStat(stat.MaxHP, a.stats.MaxHP)
}

// MPValue returns current MP as a floating-point skill-resource value.
func (a *Actor) MPValue() float64 {
	a.vitals.mu.RLock()
	defer a.vitals.mu.RUnlock()
	return a.vitals.mp
}

// MaxMPValue returns maximum MP as a floating-point skill-resource value.
func (a *Actor) MaxMPValue() float64 {
	return a.calcStat(stat.MaxMP, a.stats.MaxMP)
}

// SetHP sets current HP, clamped to [0, MaxHP].
func (a *Actor) SetHP(value float64) {
	maxHP := a.MaxHPValue()
	if value < 0 {
		value = 0
	}
	if value > maxHP {
		value = maxHP
	}
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	a.vitals.hp = value
}

// AddHP restores HP, clamped to MaxHP, and returns the applied amount.
func (a *Actor) AddHP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxHP := a.MaxHPValue()
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	if a.vitals.hp >= maxHP {
		return 0
	}
	if a.vitals.hp+amount > maxHP {
		amount = maxHP - a.vitals.hp
	}
	a.vitals.hp += amount
	return amount
}

// AddMP restores MP, clamped to MaxMP, and returns the applied amount.
func (a *Actor) AddMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxMP := a.MaxMPValue()
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	if a.vitals.mp >= maxMP {
		return 0
	}
	if a.vitals.mp+amount > maxMP {
		amount = maxMP - a.vitals.mp
	}
	a.vitals.mp += amount
	return amount
}

// ReduceMP subtracts MP, clamped at zero, and returns the applied amount.
func (a *Actor) ReduceMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	if a.vitals.mp <= 0 {
		return 0
	}
	if amount > a.vitals.mp {
		amount = a.vitals.mp
	}
	a.vitals.mp -= amount
	return amount
}

// ReduceHP applies skill HP damage and marks the summon dead at zero HP.
func (a *Actor) ReduceHP(amount float64, _ any, _ modelskill.Definition) {
	if amount <= 0 || a.Dead() {
		return
	}
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	if a.vitals.hp <= 0 {
		return
	}
	a.vitals.hp -= amount
	if a.vitals.hp <= 0 {
		a.vitals.hp = 0
		a.dead = true
	}
}

// CanBeHealed reports whether a may receive HP/MP restoration.
func (a *Actor) CanBeHealed() bool {
	return !a.Dead() && !a.Invul()
}

// HealEffectiveness returns the percentage multiplier applied to incoming heals.
func (a *Actor) HealEffectiveness() float64 {
	return a.calcStat(stat.HealEffectiveness, 100)
}

// HealProficiency returns the flat heal-power bonus a contributes.
func (a *Actor) HealProficiency() float64 {
	return a.calcStat(stat.HealProficiency, 0)
}

// RechargeMP applies a's MP recharge multiplier to amount.
func (a *Actor) RechargeMP(amount float64) float64 {
	return a.calcStat(stat.RechargeMPRate, amount)
}

// HealAmount resolves a's outgoing HEAL amount before target effectiveness.
func (a *Actor) HealAmount(def modelskill.Definition) (float64, bool) {
	amount := float64(def.Power) + a.HealProficiency()
	if creature.SkillTypeKey(def.SkillType) == "HEAL_STATIC" {
		return amount, true
	}
	return amount + math.Sqrt(float64(int(a.MAtk()))), true
}

// PhysicalSkillInput resolves the damage formula input for a physical skill
// cast by caster against a.
func (a *Actor) PhysicalSkillInput(caster any, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return creature.ResolvePhysicalSkillInput(caster, a, def, creature.Playable(caster) && a.Playable(), 1)
}

// MagicDamageInput resolves the damage formula input for a magic skill cast by
// caster against a.
func (a *Actor) MagicDamageInput(caster any, def modelskill.Definition) (formulas.MagicDamageInput, bool) {
	return creature.ResolveMagicDamageInput(caster, a, def, creature.Playable(caster) && a.Playable())
}

// BlowInput resolves the damage formula input for a blow skill cast by caster
// against a.
func (a *Actor) BlowInput(caster any, def modelskill.Definition) (formulas.BlowInput, bool) {
	return creature.ResolveBlowInput(caster, a, def, creature.Playable(caster) && a.Playable())
}

// ManaDamageInput resolves the MP-damage formula input for a magic skill cast
// by caster against a.
func (a *Actor) ManaDamageInput(caster any, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return creature.ResolveManaDamageInput(caster, a, a.MaxMPValue(), def)
}

// SkillSuccessInput returns the effect-landing roll input for def cast against a.
func (a *Actor) SkillSuccessInput(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return creature.ResolveSkillSuccessInput(caster, a, def, bss, shield)
}

func defaultInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveBase(value float64) float64 {
	if value > 0 {
		return value
	}
	return 1
}
