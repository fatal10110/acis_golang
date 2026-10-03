package summon

import (
	"math"
	"math/rand/v2"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
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
	// HPRegen and MPRegen are the npc template's base regeneration per
	// tick. A pet's growth rows carry regen values too, which nothing reads.
	HPRegen, MPRegen float64
}

// PhysicalAttackSpeed returns this summon's physical attack speed from its NPC template.
func (a *Actor) PhysicalAttackSpeed() float64 { return a.PAtkSpd(a.combatStats().AttackSpeed) }

func (a *Actor) combatStats() CombatStats {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.stats
}

type summonVitals struct {
	// mu guards hp, mp, and Actor.dead. An attacker's hit writes them
	// from the attacker's queue.
	mu     sync.RWMutex
	hp, mp float64
}

type summonStatCalcs struct {
	// mu guards calcs slot creation; each slot's own Calculator then
	// guards its own Mods independently, so a warm read only ever takes
	// mu's read lock. An attacker's formulas read these stats from the
	// attacker's queue.
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
// effect-driven adds through AttachStatFuncs).
func (a *Actor) AddStatFuncs(fns []effect.Mod) {
	a.AttachStatFuncs(fns)
	a.StatFuncsAttached(fns)
}

// AttachStatFuncs attaches fns to a's live stat calculators without
// reporting the change.
func (a *Actor) AttachStatFuncs(fns []effect.Mod) {
	for _, fn := range fns {
		a.statCalcOrCreate(fn.Stat).AddMod(fn)
	}
}

// StatFuncsAttached reports the stat change of attached fns: the movement
// takes the new move speed, and the pet window and the observers' NpcInfo
// are republished. The effect list calls it after releasing its lock, since
// those packets read the list back.
func (a *Actor) StatFuncsAttached(fns []effect.Mod) {
	if len(fns) == 0 {
		return
	}
	a.statsModified()
}

// RemoveStatsByOwner drops every stat func previously added for owner. An
// effect a stop-all is ending changes the movement speed only: the
// stop-all's caller refreshes the owner's pet window once, when it ends.
func (a *Actor) RemoveStatsByOwner(owner effect.ModOwner) {
	if owner == (effect.ModOwner{}) {
		return
	}
	a.statCalc.mu.RLock()
	calcs := a.statCalc.calcs
	a.statCalc.mu.RUnlock()
	removed := false
	for _, calc := range calcs {
		if calc != nil && calc.RemoveOwner(owner) > 0 {
			removed = true
		}
	}
	switch {
	case removed && owner.Stripped():
		a.refreshMoveSpeed()
	case removed:
		a.statsModified()
	}
}

// statsModified follows a stat func change: every position update reads
// the live move speed, so the movement gets the new one. Whatever stat
// changed, a summon its owner has seen then republishes its full view: the
// owner's pet window, its status and every observer's NpcInfo, which carry
// its P.Atk./P.Def./Max HP and the attack and movement speed multipliers
// the client animates it by.
func (a *Actor) statsModified() {
	a.refreshMoveSpeed()
	if a.ownerDiscovered.Load() {
		a.emit(event.OwnerInfoChanged{})
		a.UpdateStatus()
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
	owner := a.currentOwner()
	return owner != nil && owner.SpawnProtected()
}

// SetInvul sets or clears this summon's invulnerability flag and reports
// whether it changed.
func (a *Actor) SetInvul(v bool) bool {
	if a == nil {
		return false
	}
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	changed := a.invul != v
	a.invul = v
	return changed
}

// CanGiveDamage reports whether the owner may inflict damage through this summon.
func (a *Actor) CanGiveDamage() bool {
	if a == nil {
		return false
	}
	owner := a.currentOwner()
	return owner == nil || owner.CanGiveDamage()
}

// Invulnerable reports whether a ignores direct resource effects.
func (a *Actor) Invulnerable() bool { return a.Invul() }

// PAtk returns this summon's physical attack stat, truncated to a whole number.
func (a *Actor) PAtk() float64 {
	return math.Trunc(a.calcStat(stat.PowerAttack, positiveBase(a.combatStats().PAtk)))
}

// PDef returns this summon's physical defence stat, truncated to a whole number.
func (a *Actor) PDef() float64 {
	return math.Trunc(a.calcStat(stat.PowerDefence, positiveBase(a.combatStats().PDef)))
}

// MAtk returns this summon's magic attack stat, truncated to a whole number.
func (a *Actor) MAtk() float64 {
	return math.Trunc(a.calcStat(stat.MagicAttack, positiveBase(a.combatStats().MAtk)))
}

// MDef returns this summon's magic defence stat, truncated to a whole number.
func (a *Actor) MDef() float64 {
	return math.Trunc(a.calcStat(stat.MagicDefence, positiveBase(a.combatStats().MDef)))
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
// baseCritRate from its npc template, truncated to an int and capped at 500:
// min(int(stat), 500).
func (a *Actor) CriticalRate(baseCritRate float64) float64 {
	return float64(min(int(a.calcStat(stat.CriticalRate, baseCritRate)), 500))
}

// MoveSpeed returns this summon's current move speed from its template
// run speed: a summon stays in run stance, a pet's weight-penalty band
// scales the base, and the RUN_SPEED stat finalizes the speed, narrowed to
// float32 like the client-facing speed.
func (a *Actor) MoveSpeed(baseRunSpeed float64) float64 {
	base := float64(int(baseRunSpeed))
	if band := a.weightPenalty.Load(); band != weightPenaltyNone {
		base = float64(float32(base * weightPenaltySpeed[band]))
	}
	return float64(float32(a.calcStat(stat.RunSpeed, base)))
}

// MovementSpeedMultiplier is the current move speed over the template run
// speed, or 0 when that base is 0. The client scales the base run and walk
// speeds it is sent by this value.
func (a *Actor) MovementSpeedMultiplier(baseRunSpeed float64) float32 {
	base := int(baseRunSpeed)
	if base == 0 {
		return 0
	}
	return float32(a.MoveSpeed(baseRunSpeed)) / float32(base)
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

// SetRollSource overrides Roll's random source for deterministic tests. Call
// it on the summon's queue.
func (a *Actor) SetRollSource(f func(int) int) { a.roll = f }

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
// A maximum is a whole-point value; current HP keeps its fraction.
func (a *Actor) MaxHPValue() float64 {
	return math.Trunc(a.calcStat(stat.MaxHP, a.combatStats().MaxHP))
}

// MPValue returns current MP as a floating-point skill-resource value.
func (a *Actor) MPValue() float64 {
	a.vitals.mu.RLock()
	defer a.vitals.mu.RUnlock()
	return a.vitals.mp
}

// MaxMPValue returns maximum MP as a floating-point skill-resource value,
// in whole points like MaxHPValue.
func (a *Actor) MaxMPValue() float64 {
	return math.Trunc(a.calcStat(stat.MaxMP, a.combatStats().MaxMP))
}

// SetHP sets current HP, clamped to [0, MaxHP], and republishes a's vitals
// even when the value did not move. It has no effect on a dead summon.
func (a *Actor) SetHP(value float64) {
	maxHP := a.MaxHPValue()
	if value < 0 {
		value = 0
	}
	if value > maxHP {
		value = maxHP
	}
	a.vitals.mu.Lock()
	if a.dead {
		a.vitals.mu.Unlock()
		return
	}
	a.vitals.hp = value
	a.vitals.mu.Unlock()
	a.BroadcastStatus()
}

// HPStatusUpdate returns a's current HP and whether the players targeting a
// must be sent it. Callers invoke it only when at least one player is
// targeting a, so an unwatched summon's bar state stays where it was last
// reported.
func (a *Actor) HPStatusUpdate() (int, bool) {
	return a.hpBar.Report(a.HP, float64(int(a.MaxHPValue())))
}

// RestoreDead marks a pet restored from a save below creature.DeathHP as
// dead. Nothing killed it here, so no death sequence runs: it simply comes
// back as the corpse it was saved as, and stays one until revived.
func (a *Actor) RestoreDead() {
	a.vitals.mu.Lock()
	a.dead = true
	a.vitals.mu.Unlock()
}

// AddHP restores HP, clamped to MaxHP, and returns the applied amount. A
// restore that applied anything republishes a's status; one that applied
// nothing stays silent. A dead summon gains nothing.
func (a *Actor) AddHP(amount float64) float64 {
	return a.publishVitals(a.addHP(amount))
}

// AddMP restores MP, clamped to MaxMP, and returns the applied amount,
// republishing a's status as AddHP does.
func (a *Actor) AddMP(amount float64) float64 {
	return a.publishVitals(a.addMP(amount))
}

// ReduceMP subtracts MP, clamped at zero, and returns the applied amount,
// republishing a's status as AddHP does.
func (a *Actor) ReduceMP(amount float64) float64 {
	return a.publishVitals(a.reduceMP(amount))
}

// publishVitals republishes a's vitals when applied is non-zero, and
// returns applied.
func (a *Actor) publishVitals(applied float64) float64 {
	if applied > 0 {
		a.BroadcastStatus()
	}
	return applied
}

// addHP is AddHP without the status republish. The check against a dead
// summon shares vitals.mu with the lethal drainHP.
func (a *Actor) addHP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxHP := a.MaxHPValue()
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	if a.dead || a.vitals.hp >= maxHP {
		return 0
	}
	if a.vitals.hp+amount > maxHP {
		amount = maxHP - a.vitals.hp
	}
	a.vitals.hp += amount
	return amount
}

// addMP is AddMP without the status republish. Like addHP it leaves a dead
// summon alone, under the lock the lethal drainHP holds.
func (a *Actor) addMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxMP := a.MaxMPValue()
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	if a.dead || a.vitals.mp >= maxMP {
		return 0
	}
	if a.vitals.mp+amount > maxMP {
		amount = maxMP - a.vitals.mp
	}
	a.vitals.mp += amount
	return amount
}

// reduceMP is ReduceMP without the status republish. A dead summon loses
// nothing.
func (a *Actor) reduceMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	a.vitals.mu.Lock()
	defer a.vitals.mu.Unlock()
	if a.dead || a.vitals.mp <= 0 {
		return 0
	}
	if amount > a.vitals.mp {
		amount = a.vitals.mp
	}
	a.vitals.mp -= amount
	return amount
}

// ReduceHP applies a skill hit's HP damage; see reduceHP. The hit first
// rolls whether it breaks a's cast, whatever the damage permission.
func (a *Actor) ReduceHP(amount float64, attacker attackable.Combatant, _ modelskill.Definition) {
	if a.Dead() {
		return
	}
	a.breakCastOnDamage(amount)
	a.reduceHP(amount, attacker)
}

// ReduceHPWithoutCastBreak is ReduceHP for a skill hit whose cast-break roll
// the caller already ran through BreakCastOnDamage, ahead of other per-hit
// work.
func (a *Actor) ReduceHPWithoutCastBreak(amount float64, attacker attackable.Combatant, _ modelskill.Definition) {
	a.reduceHP(amount, attacker)
}

// TakeDamage applies a landed auto-attack hit; see reduceHP. It reports
// whether the hit killed a. The hit's cast-break roll is the attacker's to
// run, through BreakCastOnDamage, once the hit's reflected and absorbed
// damage have applied.
func (a *Actor) TakeDamage(damage int, attacker attackable.Combatant) bool {
	if a.Dead() {
		return false
	}
	return a.reduceHP(float64(damage), attacker)
}

// reduceHP applies one direct hit, from a skill or an auto-attack, and
// reports whether it killed a. A dead summon ignores it. An invulnerable
// summon, or a hit from an attacker barred from dealing damage, changes
// nothing on a; otherwise the hit wakes a sleeping summon, ends an
// immobilize-until-attacked, breaks a stun one time in ten, and then takes
// the HP, running the death sequence when it drops below
// creature.DeathHP. The owner is told of the hit whenever it has an
// attacker, including a hit that was blocked or that killed. Any hit
// interrupts the duel of an attacker from outside the owner's duel.
func (a *Actor) reduceHP(amount float64, attacker attackable.Combatant) bool {
	if a.Dead() {
		return false
	}
	creature.InterruptDuelOnSummonHit(attacker, a.ownerDuelID())
	killed := false
	if !a.Invul() && creature.CanDealDamage(attacker) {
		a.applyHitSideEffects()
		if amount > 0 && a.drainHP(amount) {
			a.die(attacker)
			killed = true
		}
	}
	if attacker != nil {
		a.notifyDamage(attacker, amount)
	}
	return killed
}

// applyHitSideEffects runs what every direct hit does to a before its HP
// changes. Damage over time and a's own HP costs never reach it.
func (a *Actor) applyHitSideEffects() {
	list := a.EffectList()
	list.StopByType(effect.TypeSleep)
	list.StopByType(effect.TypeImmobileUntilAttacked)
	if list.IsAffected(effect.FlagStunned) && a.Roll(10) == 0 {
		list.StopByType(effect.TypeStun)
	}
}

// ConsumeHP pays one of a's own skill HP costs. The summon is its own
// attacker here: an invulnerable summon still pays the cost, while an owner
// barred from dealing damage does not. The owner is told of the damage
// either way, with the summon named as its source. A lethal cost kills a,
// with a as its own killer.
func (a *Actor) ConsumeHP(amount float64) {
	if amount <= 0 || a.Dead() {
		return
	}
	if creature.CanDealDamage(a) && a.drainHP(amount) {
		a.die(a)
	}
	a.notifyDamage(a, amount)
}

// ReduceHPByDOT applies periodic HP damage without normal-hit side effects.
// A lethal tick kills a. Any tick, a zero one included, interrupts the duel
// of an attacker from outside the owner's duel.
func (a *Actor) ReduceHPByDOT(amount float64, attacker effect.Actor, _ bool) {
	killer, _ := attacker.(attackable.Combatant)
	if a.Dead() {
		return
	}
	creature.InterruptDuelOnSummonHit(killer, a.ownerDuelID())
	if amount <= 0 || a.Invul() || !creature.CanDealDamage(killer) {
		return
	}
	if a.drainHP(amount) {
		a.die(killer)
	}
}

// drainHP takes amount off a live summon's HP, marks it dead below DeathHP,
// and refreshes its status. It reports whether this call killed the summon;
// only one caller ever sees true.
func (a *Actor) drainHP(amount float64) bool {
	a.vitals.mu.Lock()
	if a.dead || a.vitals.hp <= 0 {
		a.vitals.mu.Unlock()
		return false
	}
	a.vitals.hp -= amount
	killed := false
	if a.vitals.hp < creature.DeathHP {
		a.vitals.hp = 0
		a.dead = true
		killed = true
	}
	a.vitals.mu.Unlock()
	a.BroadcastStatus()
	return killed
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

// HealInput resolves a's side of an outgoing HEAL. A summon's charged
// spiritshot scales its M.Atk term the way a mage-class player's does.
func (a *Actor) HealInput(def modelskill.Definition) (formulas.HealInput, bool) {
	return creature.ResolveHealInput(def, a.HealProficiency(), a.MAtk(), formulas.HealShotScalingMage), true
}

// PhysicalSkillInput resolves the damage formula input for a physical skill
// cast by caster against a.
func (a *Actor) PhysicalSkillInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return creature.ResolvePhysicalSkillInput(caster, a, def, creature.Playable(caster), 1)
}

// MagicDamageInput resolves the damage formula input for a magic skill cast by
// caster against a, rolling resist when magicFailures is set.
func (a *Actor) MagicDamageInput(caster creature.FormulaActor, def modelskill.Definition, magicFailures bool) (formulas.MagicDamageInput, bool) {
	return creature.ResolveMagicDamageInput(caster, a, def, creature.Playable(caster), magicFailures)
}

// BlowInput resolves the damage formula input for a blow skill cast by caster
// against a.
func (a *Actor) BlowInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.BlowInput, bool) {
	return creature.ResolveBlowInput(caster, a, def, creature.Playable(caster))
}

func (a *Actor) CounterSkillPhysical() float64 {
	return a.CalcStat(stat.CounterSkillPhysical, 0)
}

// CancelVulnerability returns a's CANCEL_VULN multiplier for the cancel and
// cancel-debuff success-rate formulas. classification is unused:
// CANCEL_VULN applies uniformly, without the per-classification switch the
// other _VULN stats use.
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
func (a *Actor) ManaDamageInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return creature.ResolveManaDamageInput(caster, a, a.MaxMPValue(), def)
}

// LethalRate returns a's lethal-strike rate multiplier.
func (a *Actor) LethalRate() float64 {
	return a.calcStat(stat.LethalRate, 1)
}

// LethalInput resolves a lethal-strike roll against a.
func (a *Actor) LethalInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.LethalInput, bool) {
	if a.Invul() || !creature.CanDealDamage(caster) {
		return formulas.LethalInput{}, false
	}
	if caster == nil {
		return formulas.LethalInput{}, false
	}
	attacker := caster
	return formulas.LethalInput{
		Chance1:       def.LethalChance1,
		Chance2:       def.LethalChance2,
		MagicLevel:    def.MagicLevel,
		AttackerLevel: attacker.Level(),
		TargetLevel:   a.Level(),
		LethalMul:     attacker.LethalRate(),
	}, true
}

// ApplyLethalOutcome applies a lethal-strike tier to a. The HP loss rolls
// no cast break of its own.
func (a *Actor) ApplyLethalOutcome(outcome formulas.LethalOutcome, caster attackable.Combatant, _ modelskill.Definition) {
	switch outcome {
	case formulas.LethalFull:
		a.reduceHP(a.HP()-1, caster)
	case formulas.LethalHalf:
		a.reduceHP(a.HP()/2, caster)
	}
}

// Actor satisfies the identity surface SkillSuccessInput/EffectSuccessInput/
// DecreaseFusion take their caster/effected parameter as.
var _ attackable.Combatant = (*Actor)(nil)

// SkillSuccessInput returns the effect-landing roll input for def cast against a.
func (a *Actor) SkillSuccessInput(caster creature.FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return creature.ResolveSkillSuccessInput(caster, a, def, bss, shield)
}

func (a *Actor) EffectSuccessInput(caster creature.FormulaActor, def modelskill.Definition, tmpl modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return creature.ResolveEffectSuccessInput(caster, a, def, tmpl, bss, shield)
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

// WeaponGradePenalty reports false: summons carry no weapon grade to be
// under-skilled for.
func (a *Actor) WeaponGradePenalty() bool { return false }
