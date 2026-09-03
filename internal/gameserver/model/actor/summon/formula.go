package summon

import (
	"math"
	"math/rand/v2"
	"strings"
	"sync"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
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
	SSCount, SPSCount            int
	AttackRange                  int
	AttackSpeed                  float64
	CritRate                     float64
}

// PhysicalAttackSpeed returns this summon's physical attack speed from its NPC template.
func (a *Actor) PhysicalAttackSpeed() float64 { return a.PAtkSpd(a.combatStats().AttackSpeed) }

func (a *Actor) combatStats() CombatStats {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.stats
}

type summonVitals struct {
	// mu guards hp, mp, and Actor.dead.
	mu     sync.RWMutex
	hp, mp float64
}

type summonStatCalcs struct {
	// mu guards calcs slot creation; each slot's own Calculator then
	// guards its own Mods independently, so a warm read only ever takes
	// mu's read lock.
	mu    sync.RWMutex
	calcs [stat.Count]*effect.Calculator
}

func (a *Actor) initVitals() {
	a.vitals.hp = a.MaxHPValue()
	a.vitals.mp = a.MaxMPValue()
}

// AddStatFuncs attaches fns to a's live stat calculators. Each Mod is
// published independently under its own Calculator's lock — the batch is
// not atomic against a concurrent CalcStat, which may observe fns partially
// applied. Callers that need a batch to appear all-or-nothing to readers
// must serialize at a higher level (see effect.List, which does this for
// effect-driven adds).
func (a *Actor) AddStatFuncs(fns []effect.Mod) {
	for _, fn := range fns {
		a.statCalcOrCreate(fn.Stat).AddMod(fn)
	}
}

// RemoveStatsByOwner drops every stat func previously added for owner.
func (a *Actor) RemoveStatsByOwner(owner effect.ModOwner) {
	if owner == (effect.ModOwner{}) {
		return
	}
	a.statCalc.mu.RLock()
	calcs := a.statCalc.calcs
	a.statCalc.mu.RUnlock()
	for _, calc := range calcs {
		if calc != nil {
			calc.RemoveOwner(owner)
		}
	}
}

func (a *Actor) statCalculator(s stat.Stat) *effect.Calculator {
	a.statCalc.mu.RLock()
	if calc := a.statCalc.calcs[s]; calc != nil {
		a.statCalc.mu.RUnlock()
		return calc
	}
	a.statCalc.mu.RUnlock()
	return a.statCalcOrCreate(s)
}

func (a *Actor) statCalcOrCreate(s stat.Stat) *effect.Calculator {
	a.statCalc.mu.Lock()
	defer a.statCalc.mu.Unlock()
	if calc := a.statCalc.calcs[s]; calc != nil {
		return calc
	}
	calc := effect.NewCalculator(defaultBuiltin(s))
	a.statCalc.calcs[s] = &calc
	return &calc
}

func (a *Actor) calcStat(s stat.Stat, base float64) float64 {
	value := a.statCalculator(s).Calc(summonStatActor{a: a}, base)
	if s.CantBeNegative() && value <= 0 {
		return 1
	}
	return value
}

// CalcStat finalizes base for s through a's live stat calculator.
func (a *Actor) CalcStat(s stat.Stat, base float64) float64 {
	return a.calcStat(s, base)
}

// defaultBuiltin returns the static, attribute-driven finalize step every
// summon's calculation chain for s runs at order 10, or nil for a Stat with
// no builtin.
func defaultBuiltin(s stat.Stat) funcs.Func {
	switch s {
	case stat.MaxHP:
		return funcs.MaxHpMul
	case stat.MaxMP:
		return funcs.MaxMpMul
	case stat.RegenerateHPRate:
		return funcs.RegenHpMul
	case stat.RegenerateMPRate:
		return funcs.RegenMpMul
	case stat.PowerAttack:
		return funcs.PAtkMod
	case stat.PowerDefence:
		return funcs.PDefMod
	case stat.MagicAttack:
		return funcs.MAtkMod
	case stat.MagicDefence:
		return funcs.MDefMod
	case stat.PowerAttackSpeed:
		return funcs.PAtkSpeed
	case stat.MagicAttackSpeed:
		return funcs.MAtkSpeed
	case stat.AccuracyCombat:
		return funcs.AtkAccuracy
	case stat.EvasionRate:
		return funcs.AtkEvasion
	case stat.CriticalRate:
		return funcs.AtkCritical
	case stat.MCriticalRate:
		return funcs.MAtkCritical
	case stat.RunSpeed:
		return funcs.MoveSpeed
	default:
		return nil
	}
}

type summonStatActor struct{ a *Actor }

var _ stat.Actor = summonStatActor{}

func (s summonStatActor) STR() int { return defaultInt(s.a.combatStats().STR, 40) }
func (s summonStatActor) CON() int { return defaultInt(s.a.combatStats().CON, 21) }
func (s summonStatActor) DEX() int { return defaultInt(s.a.combatStats().DEX, 30) }
func (s summonStatActor) INT() int { return defaultInt(s.a.combatStats().INT, 20) }
func (s summonStatActor) WIT() int { return defaultInt(s.a.combatStats().WIT, 43) }
func (s summonStatActor) MEN() int { return defaultInt(s.a.combatStats().MEN, 20) }

func (s summonStatActor) Level() int {
	if lvl := s.a.Level(); lvl > 0 {
		return lvl
	}
	return 1
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

// EffectList returns this summon's active buffs and debuffs.
func (a *Actor) EffectList() *effect.List {
	return a.effects
}

// MaxBuffCount is the number of non-toggle, non-seven-signs buffs this
// summon can hold at once: the configured base plus its template skill level.
func (a *Actor) MaxBuffCount() int {
	if a == nil {
		return baseBuffSlots
	}
	return a.maxBuffsAmount + a.skills[int(modelskill.DivineInspirationSkillID)]
}

// Playable reports whether a is player-controlled.
func (a *Actor) Playable() bool { return true }

// Invul reports whether a is currently invulnerable.
func (a *Actor) Invul() bool {
	if a == nil {
		return false
	}
	a.statusMu.RLock()
	invul := a.invul
	a.statusMu.RUnlock()
	if invul {
		return true
	}
	owner, ok := a.owner.(interface{ SpawnProtected() bool })
	return ok && owner.SpawnProtected()
}

// SetInvul sets or clears this summon's invulnerability flag.
func (a *Actor) SetInvul(v bool) {
	if a == nil {
		return
	}
	a.statusMu.Lock()
	a.invul = v
	a.statusMu.Unlock()
}

// CanGiveDamage reports whether the owner may inflict damage through this summon.
func (a *Actor) CanGiveDamage() bool {
	if a == nil {
		return false
	}
	owner, ok := a.owner.(interface{ CanGiveDamage() bool })
	return !ok || owner.CanGiveDamage()
}

// Invulnerable reports whether a ignores direct resource effects.
func (a *Actor) Invulnerable() bool { return a.Invul() }

// PAtk returns this summon's physical attack stat.
func (a *Actor) PAtk() float64 {
	return a.calcStat(stat.PowerAttack, positiveBase(a.combatStats().PAtk))
}

// PDef returns this summon's physical defence stat.
func (a *Actor) PDef() float64 {
	return a.calcStat(stat.PowerDefence, positiveBase(a.combatStats().PDef))
}

// MAtk returns this summon's magic attack stat.
func (a *Actor) MAtk() float64 {
	return a.calcStat(stat.MagicAttack, positiveBase(a.combatStats().MAtk))
}

// MDef returns this summon's magic defence stat.
func (a *Actor) MDef() float64 {
	return a.calcStat(stat.MagicDefence, positiveBase(a.combatStats().MDef))
}

// MagicCriticalRate returns this summon's magic critical rate.
func (a *Actor) MagicCriticalRate() float64 {
	return a.calcStat(stat.MCriticalRate, 8)
}

// Accuracy returns this summon's physical accuracy stat.
func (a *Actor) Accuracy() float64 {
	return a.calcStat(stat.AccuracyCombat, 0)
}

// EvasionRate returns this summon's physical evasion stat.
func (a *Actor) EvasionRate() float64 {
	return a.calcStat(stat.EvasionRate, 0)
}

// CriticalRate returns this summon's physical critical rate, given
// baseCritRate from its npc template, truncated to an int and capped at 500
// per CreatureStatus.getCriticalHit (CreatureStatus.java:551-553):
// `Math.min((int) calcStat(...), 500)`.
func (a *Actor) CriticalRate(baseCritRate float64) float64 {
	return float64(min(int(a.calcStat(stat.CriticalRate, baseCritRate)), 500))
}

// MoveSpeed returns this summon's current move speed, matching its run
// speed: a summon always moves at its run speed, mirroring
// PetStatus.getMoveSpeed()/SummonStatus's shared run-speed basis.
func (a *Actor) MoveSpeed(baseRunSpeed float64) float64 {
	return a.calcStat(stat.RunSpeed, baseRunSpeed)
}

// hungryHalved reports whether a pet's attack speed should be halved for
// being under-fed, matching Pet.checkHungryState. Servitors have no feeding
// state and are never halved.
func (a *Actor) hungryHalved() bool {
	a.statusMu.RLock()
	fed, maxMeal := a.fed, a.maxMeal
	a.statusMu.RUnlock()
	if !a.isPet || maxMeal <= 0 {
		return false
	}
	return float64(fed) < float64(maxMeal)*a.hungryLimit
}

// PAtkSpd returns this summon's physical attack speed, given baseAtkSpd from
// its npc template (Pet.getStatus().getPAtkSpd() / SummonStatus's shared
// basis), halved while hungry.
func (a *Actor) PAtkSpd(baseAtkSpd float64) float64 {
	if a.hungryHalved() {
		baseAtkSpd /= 2
	}
	return a.calcStat(stat.PowerAttackSpeed, baseAtkSpd)
}

// magicAtkSpdBase is the fixed magic-attack-speed base every pet and
// servitor uses (PetStatus.getMAtkSpd / base SummonStatus), independent of
// npc template.
const magicAtkSpdBase = 333

// MAtkSpd returns this summon's magic attack speed, halved while hungry.
func (a *Actor) MAtkSpd() float64 {
	base := float64(magicAtkSpdBase)
	if a.hungryHalved() {
		base /= 2
	}
	return a.calcStat(stat.MagicAttackSpeed, base)
}

// AttackType returns this summon's current physical attack style.
func (a *Actor) AttackType() item.WeaponType { return item.WeaponFist }

// SoulshotCharged reports whether a soulshot charge is currently active.
func (a *Actor) SoulshotCharged() bool { return a.ChargedShot(item.ShotSoul) }

// SpiritshotCharged reports whether a spiritshot charge is currently active.
func (a *Actor) SpiritshotCharged() bool { return a.ChargedShot(item.ShotSpirit) }

// BlessedSpiritshotCharged reports whether a blessed spiritshot charge is active.
func (a *Actor) BlessedSpiritshotCharged() bool { return a.ChargedShot(item.ShotBlessedSpirit) }

// ChargedShot reports whether kind is currently charged on a.
func (a *Actor) ChargedShot(kind item.ShotKind) bool {
	a.shotsMu.Lock()
	defer a.shotsMu.Unlock()
	return a.shotsMask&kind.Mask() == kind.Mask()
}

// SetChargedShot charges or discharges kind on a.
func (a *Actor) SetChargedShot(kind item.ShotKind, charged bool) {
	a.shotsMu.Lock()
	defer a.shotsMu.Unlock()
	if charged {
		a.shotsMask |= kind.Mask()
	} else {
		a.shotsMask &^= kind.Mask()
	}
}

// SSCount returns the beast soulshot count this summon consumes per charge.
func (a *Actor) SSCount() int { return a.combatStats().SSCount }

// SPSCount returns the beast spiritshot count this summon consumes per charge.
func (a *Actor) SPSCount() int { return a.combatStats().SPSCount }

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

// RandomDamageSpread returns the summon's random-damage spread, or -1
// (RandomDamageMultiplier's "use the weaponless fallback" sentinel) when no
// spread is configured.
func (a *Actor) RandomDamageSpread() int {
	spread := a.combatStats().BaseRandomDamage
	if spread <= 0 {
		return -1
	}
	return spread
}

// HP returns current HP as a floating-point skill-resource value.
func (a *Actor) HP() float64 {
	a.vitals.mu.RLock()
	defer a.vitals.mu.RUnlock()
	return a.vitals.hp
}

// MaxHPValue returns maximum HP as a floating-point skill-resource value.
func (a *Actor) MaxHPValue() float64 {
	return a.calcStat(stat.MaxHP, a.combatStats().MaxHP)
}

// MPValue returns current MP as a floating-point skill-resource value.
func (a *Actor) MPValue() float64 {
	a.vitals.mu.RLock()
	defer a.vitals.mu.RUnlock()
	return a.vitals.mp
}

// MaxMPValue returns maximum MP as a floating-point skill-resource value.
func (a *Actor) MaxMPValue() float64 {
	return a.calcStat(stat.MaxMP, a.combatStats().MaxMP)
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
func (a *Actor) ReduceHP(amount float64, attacker creature.DeathActor, _ modelskill.Definition) {
	if amount <= 0 || a.Invul() || !creature.CanDealDamage(attacker) {
		return
	}
	a.vitals.mu.Lock()
	if a.dead || a.vitals.hp <= 0 {
		a.vitals.mu.Unlock()
		return
	}
	a.vitals.hp -= amount
	if a.vitals.hp <= 0 {
		a.vitals.hp = 0
		a.dead = true
	}
	a.vitals.mu.Unlock()
	a.UpdateStatus()
	if attacker != nil {
		a.notifyDamage(attacker, amount)
	}
}

// ReduceHPByDOT applies periodic HP damage without normal-hit side effects.
func (a *Actor) ReduceHPByDOT(amount float64, attacker effect.Participant, _ bool) {
	if amount <= 0 || a.Invul() || !creature.CanDealDamage(attacker) {
		return
	}
	a.vitals.mu.Lock()
	if a.dead || a.vitals.hp <= 0 {
		a.vitals.mu.Unlock()
		return
	}
	a.vitals.hp -= amount
	if a.vitals.hp <= 0 {
		a.vitals.hp = 0
		a.dead = true
	}
	a.vitals.mu.Unlock()
	a.UpdateStatus()
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
func (a *Actor) PhysicalSkillInput(caster creature.DeathActor, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return creature.ResolvePhysicalSkillInput(caster, a, def, creature.Playable(caster) && a.Playable(), 1)
}

// MagicDamageInput resolves the damage formula input for a magic skill cast by
// caster against a.
func (a *Actor) MagicDamageInput(caster creature.DeathActor, def modelskill.Definition) (formulas.MagicDamageInput, bool) {
	return creature.ResolveMagicDamageInput(caster, a, def, creature.Playable(caster) && a.Playable())
}

// BlowInput resolves the damage formula input for a blow skill cast by caster
// against a.
func (a *Actor) BlowInput(caster creature.DeathActor, def modelskill.Definition) (formulas.BlowInput, bool) {
	return creature.ResolveBlowInput(caster, a, def, creature.Playable(caster) && a.Playable())
}

func (a *Actor) CounterSkillPhysical() float64 {
	return a.CalcStat(stat.CounterSkillPhysical, 0)
}

// CancelVulnerability returns a's CANCEL_VULN multiplier for the cancel and
// cancel-debuff success-rate formulas (Formulas.java:949-951). classification
// is unused: the reference applies CANCEL_VULN uniformly, without the
// per-classification switch it uses for the other _VULN stats.
func (a *Actor) CancelVulnerability(_ string) float64 {
	return a.CalcStat(stat.CancelVuln, 1)
}

// SkillReflectInput resolves a's reflected-skill chance for def.
func (a *Actor) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	reflectStat := stat.ReflectSkillPhysic
	if def.Magic {
		reflectStat = stat.ReflectSkillMagic
	}
	return formulas.SkillReflectInput{
		IgnoreResists:  def.IgnoreResists,
		CanBeReflected: def.CanBeReflected,
		Magic:          def.Magic,
		CastRange:      def.CastRange,
		ReflectChance:  a.CalcStat(reflectStat, 0),
	}
}

// ManaDamageInput resolves the MP-damage formula input for a magic skill cast
// by caster against a.
func (a *Actor) ManaDamageInput(caster creature.DeathActor, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return creature.ResolveManaDamageInput(caster, a, a.MaxMPValue(), def)
}

// LethalRate returns a's lethal-strike rate multiplier.
func (a *Actor) LethalRate() float64 {
	return a.calcStat(stat.LethalRate, 1)
}

// LethalInput resolves a lethal-strike roll against a.
func (a *Actor) LethalInput(caster creature.DeathActor, def modelskill.Definition) (formulas.LethalInput, bool) {
	if a.Invul() || !creature.CanDealDamage(caster) {
		return formulas.LethalInput{}, false
	}
	attacker, ok := caster.(interface {
		Level() int
		LethalRate() float64
	})
	if !ok {
		return formulas.LethalInput{}, false
	}
	return formulas.LethalInput{
		Chance1:       def.LethalChance1,
		Chance2:       def.LethalChance2,
		MagicLevel:    def.MagicLevel,
		AttackerLevel: attacker.Level(),
		TargetLevel:   a.Level(),
		LethalMul:     attacker.LethalRate(),
	}, true
}

// ApplyLethalOutcome applies a lethal-strike tier to a.
func (a *Actor) ApplyLethalOutcome(outcome formulas.LethalOutcome, caster creature.DeathActor, def modelskill.Definition) {
	switch outcome {
	case formulas.LethalFull:
		a.ReduceHP(a.HP()-1, caster, def)
	case formulas.LethalHalf:
		a.ReduceHP(a.HP()/2, caster, def)
	}
}

// Actor satisfies the identity surface SkillSuccessInput/EffectSuccessInput/
// DecreaseFusion take their caster/effected parameter as.
var _ creature.DeathActor = (*Actor)(nil)

// SkillSuccessInput returns the effect-landing roll input for def cast against a.
func (a *Actor) SkillSuccessInput(caster creature.DeathActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return creature.ResolveSkillSuccessInput(caster, a, def, bss, shield)
}

func (a *Actor) EffectSuccessInput(caster creature.DeathActor, def modelskill.Definition, tmpl modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if tmpl.EffectType == "" {
		return formulas.SkillSuccessInput{BaseChance: tmpl.EffectPower, IgnoreResists: true, Shield: shield}, true
	}
	if strings.EqualFold(tmpl.EffectType, "CANCEL") {
		return formulas.SkillSuccessInput{BaseChance: 100, IgnoreResists: true, Shield: shield}, true
	}
	def.EffectType = tmpl.EffectType
	def.IgnoreResists = false
	in, ok := a.SkillSuccessInput(caster, def, bss, shield)
	in.BaseChance = tmpl.EffectPower
	return in, ok
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
