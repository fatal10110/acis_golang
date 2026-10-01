package npc

import (
	"math"
	"math/rand"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Attackable reports that h is an NPC-like combat target.
func (h *Hostile) Attackable() bool { return true }

// Playable reports whether h is player-controlled.
func (h *Hostile) Playable() bool { return false }

// Undead reports whether h has the undead NPC race.
func (h *Hostile) Undead() bool {
	return h != nil && h.Instance != nil && h.Instance.Template != nil && h.Instance.Template.Race == RaceUndead
}

// Invul reports whether h is currently invulnerable.
func (h *Hostile) Invul() bool { return h != nil && h.Live != nil && h.Live.Invul() }

// Invulnerable reports whether h ignores direct resource effects.
func (h *Hostile) Invulnerable() bool { return h.Invul() }

// Lethalable reports whether h may receive lethal strikes.
func (h *Hostile) Lethalable() bool {
	switch h.Instance.Template.ID {
	case 22215, 22216, 22217, 35062, 35410, 35368, 35375, 35629:
		return false
	}
	return true
}

// PAtk returns this NPC's physical attack stat, truncated to a whole number.
func (h *Hostile) PAtk() float64 {
	return math.Trunc(h.calcStat(stat.PowerAttack, h.Instance.Template.PAtk))
}

// MagicCriticalRate returns this NPC's magic critical rate.
func (h *Hostile) MagicCriticalRate() float64 {
	return h.calcStat(stat.MCriticalRate, 8)
}

// SpiritshotCharged reports whether a spiritshot charge is currently active.
func (h *Hostile) SpiritshotCharged() bool {
	h.shotsMu.RLock()
	defer h.shotsMu.RUnlock()
	return h.shotsMask&item.ShotSpirit.Mask() != 0
}

// BlessedSpiritshotCharged reports whether a blessed spiritshot charge is active.
func (h *Hostile) BlessedSpiritshotCharged() bool { return false }

// Roll draws a uniform random integer in [0, n) from h's combat random source.
func (h *Hostile) Roll(n int) int {
	if n <= 0 {
		return 0
	}
	if h.roll != nil {
		return h.roll(n)
	}
	return rand.Intn(n)
}

// RandomDamageSpread returns the template-defined random-damage spread, or
// -1 (RandomDamageMultiplier's "use the weaponless fallback" sentinel) when
// no spread is configured.
func (h *Hostile) RandomDamageSpread() int {
	if h.Instance.Template.BaseRandomDamage <= 0 {
		return -1
	}
	return h.Instance.Template.BaseRandomDamage
}

// HP returns current HP as a floating-point skill-resource value.
func (h *Hostile) HP() float64 {
	return h.health.Current()
}

// MaxHPValue returns maximum HP as a floating-point skill-resource value.
// A maximum is a whole-point value; current HP keeps its fraction.
func (h *Hostile) MaxHPValue() float64 {
	return math.Trunc(h.calcStat(stat.MaxHP, h.Instance.Template.HPMax))
}

// RunSpeed returns this NPC's final run speed.
func (h *Hostile) RunSpeed() int {
	return int(h.calcStat(stat.RunSpeed, h.Instance.Template.RunSpeed))
}

// MPValue returns current MP as a floating-point skill-resource value.
func (h *Hostile) MPValue() float64 {
	h.mpMu.RLock()
	defer h.mpMu.RUnlock()
	return h.mp
}

// MaxMPValue returns maximum MP as a floating-point skill-resource value,
// in whole points like MaxHPValue.
func (h *Hostile) MaxMPValue() float64 {
	return math.Trunc(h.calcStat(stat.MaxMP, h.Instance.Template.MPMax))
}

// SetHP sets current HP, clamped to [0, MaxHP], and offers the targeters'
// health bar a refresh even when the value did not move. It has no effect on
// a dead NPC.
func (h *Hostile) SetHP(value float64) {
	maxHP := h.MaxHPValue()
	if value < 0 {
		value = 0
	}
	if value > maxHP {
		value = maxHP
	}
	if h.health.SetCurrent(value) {
		h.BroadcastStatus()
	}
}

// AddHP restores HP, clamped to MaxHP, and returns the applied amount. A
// restore that applied anything refreshes the health bar of the players
// targeting h; one that applied nothing stays silent.
func (h *Hostile) AddHP(amount float64) float64 {
	return h.publishVitals(h.health.Add(amount, h.MaxHPValue()))
}

// AddMP restores MP, clamped to MaxMP, and returns the applied amount. Like
// every vitals change it offers the targeters' health bar a refresh, which
// the bar's own segment check usually declines since HP did not move.
func (h *Hostile) AddMP(amount float64) float64 {
	return h.publishVitals(h.addMP(amount))
}

// ReduceMP subtracts MP, clamped at zero, and returns the applied amount,
// offering the targeters' health bar a refresh as AddMP does.
func (h *Hostile) ReduceMP(amount float64) float64 {
	return h.publishVitals(h.reduceMP(amount))
}

// publishVitals reports a vitals change when applied is non-zero, and
// returns applied.
func (h *Hostile) publishVitals(applied float64) float64 {
	if applied > 0 {
		h.BroadcastStatus()
	}
	return applied
}

// addMP is AddMP without the status report. A dead NPC gains nothing.
func (h *Hostile) addMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxMP := h.MaxMPValue()
	applied := 0.0
	h.whileAliveMP(func() {
		if h.mp >= maxMP {
			return
		}
		applied = min(amount, maxMP-h.mp)
		h.mp += applied
	})
	return applied
}

// reduceMP is ReduceMP without the status report. A dead NPC loses nothing.
func (h *Hostile) reduceMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	applied := 0.0
	h.whileAliveMP(func() {
		applied = min(amount, h.mp)
		h.mp -= applied
	})
	return applied
}

// whileAliveMP runs write, which changes mp, only while h is alive. Death
// is decided by HP, which a killing blow zeroes under the health lock
// before the dead flag follows, so the write holds that lock too: no MP
// change lands on a corpse, whichever queue the killing blow came from.
func (h *Hostile) whileAliveMP(write func()) {
	h.health.WhileAlive(func() {
		h.mpMu.Lock()
		defer h.mpMu.Unlock()
		write()
	})
}

// ReduceHP applies skill HP damage and runs the once-only death path. The
// hit first rolls whether it breaks h's cast, whatever the damage
// permission. Hate, the shot-recharge roll, and the party/minion attacked
// call always run for a live hit, a zero-damage one included, one layer
// above the invul/damage-permission guard: an invulnerable NPC, or one hit
// by an attacker without damage permission, still aggroes and calls its
// party, but takes no damage. See reduceHP.
func (h *Hostile) ReduceHP(amount float64, attacker attackable.Combatant, _ modelskill.Definition) {
	if h.AlikeDead() {
		return
	}
	h.breakCastOnDamage(amount)
	h.reduceHP(amount, attacker)
}

// ReduceHPWithoutCastBreak is ReduceHP for a skill hit whose cast-break roll
// the caller already ran through BreakCastOnDamage, ahead of other per-hit
// work.
func (h *Hostile) ReduceHPWithoutCastBreak(amount float64, attacker attackable.Combatant, _ modelskill.Definition) {
	h.reduceHP(amount, attacker)
}

// reduceHP is ReduceHP without the cast-break roll, for HP loss that is not
// a damage hit of its own. A hit that works out to no damage (a
// damage-denied or non-player CHARGEDAM, a countered hit whose countered
// share is zero, a lethal strike on an NPC at 1 HP) still registers the hit
// and, for a permitted attacker on a vulnerable NPC, still wakes it and
// rolls the stun break; only the HP write and its status report need a
// positive amount.
func (h *Hostile) reduceHP(amount float64, attacker attackable.Combatant) {
	if h.AlikeDead() {
		return
	}
	// A NaN amount (a zero-defence hit scaled by a zero multiplier) takes
	// nothing, like a negative one.
	if !(amount > 0) {
		amount = 0
	}
	h.testOverhit(attacker, amount)
	h.registerHit(attacker, amount, false)
	if h.Invul() || !creature.CanDealDamage(attacker) {
		return
	}
	h.applyNonConsumptionDamageEffects(false)
	if amount == 0 {
		return
	}
	newlyDead := h.health.DamageValue(amount)
	h.BroadcastStatus()
	if !newlyDead {
		return
	}
	h.Die(attacker, h.rewards)
}

// ConsumeHP pays one of h's own skill HP costs. The NPC is its own attacker
// here, and the cost is a consumption rather than a hit: it adds no hate,
// calls no party, and never wakes or stun-breaks the caster. An
// invulnerable NPC still pays it and still dies from it, with itself as the
// killer. The overhit check still runs against the NPC itself, so a lethal
// cost replaces an overhit a player armed and that player loses the bonus.
func (h *Hostile) ConsumeHP(amount float64) {
	if h.AlikeDead() {
		return
	}
	h.testOverhit(h, amount)
	if amount <= 0 {
		return
	}
	newlyDead := h.health.DamageValue(amount)
	h.BroadcastStatus()
	if !newlyDead {
		return
	}
	h.Die(h, h.rewards)
}

// ReduceHPByDOT applies periodic damage and records it in the threat table
// at zero hate weight — every HP reduction feeds the hate list, DOT
// included (there is no isDOT gate on that path). A zero-damage tick
// registers the hit the same way and writes no HP.
func (h *Hostile) ReduceHPByDOT(amount float64, attacker effect.Actor, isDOT bool) {
	if h.AlikeDead() {
		return
	}
	amount = max(amount, 0)
	killer, _ := attacker.(attackable.Combatant)
	h.testOverhit(killer, amount)
	h.registerHit(killer, amount, true)
	if h.Invul() || !creature.CanDealDamage(killer) {
		return
	}
	h.applyNonConsumptionDamageEffects(isDOT)
	if amount == 0 {
		return
	}
	newlyDead := h.health.DamageValue(amount)
	h.BroadcastStatus()
	if !newlyDead {
		return
	}
	h.Die(killer, h.rewards)
}

// applyNonConsumptionDamageEffects applies the creature-wide HP-reduction
// side effects an NPC keeps: non-DOT HP reduction stops SLEEP and
// IMMOBILE_UNTIL_ATTACKED, and has a 1-in-10 chance to break STUN. An NPC
// adds only a duel-interrupt check on the attacker and otherwise keeps the
// creature rule unchanged, including the isDOT gate on the whole block —
// unlike a player, whose gate is !isHPConsumption alone and who
// stun-breaks separately on !isDOT. HP consumption never reaches this
// block: it goes through ConsumeHP instead. There is no sit/stand-up clause
// (Player-only). Callers must run this after AddDamageHate: the hate lands
// before the HP reduction reaches this block, so a sleep-stop's synchronous
// wake-think (the sleep effect's exit -> hooks_cc.go's
// thinkAndRefreshExit) always sees the hit's hate already in the threat
// table.
func (h *Hostile) applyNonConsumptionDamageEffects(isDOT bool) {
	if isDOT {
		return
	}
	list := h.EffectList()
	list.StopByType(effect.TypeSleep)
	list.StopByType(effect.TypeImmobileUntilAttacked)

	if h.Stunned() && h.Roll(10) == 0 {
		list.StopByType(effect.TypeStun)
	}
}

// CanBeHealed reports whether h may receive HP/MP restoration.
func (h *Hostile) CanBeHealed() bool {
	return !h.Dead() && !h.Invul()
}

// HealEffectiveness returns the percentage multiplier applied to incoming heals.
func (h *Hostile) HealEffectiveness() float64 {
	return h.calcStat(stat.HealEffectiveness, 100)
}

// HealProficiency returns the flat heal-power bonus h contributes.
func (h *Hostile) HealProficiency() float64 {
	return h.calcStat(stat.HealProficiency, 0)
}

// RechargeMP applies h's MP recharge multiplier to amount.
func (h *Hostile) RechargeMP(amount float64) float64 {
	return h.calcStat(stat.RechargeMPRate, amount)
}

// HealInput resolves h's side of an outgoing HEAL. Any charged spiritshot
// quadruples an NPC's M.Atk term.
func (h *Hostile) HealInput(def modelskill.Definition) (formulas.HealInput, bool) {
	return creature.ResolveHealInput(def, h.HealProficiency(), h.MAtk(), formulas.HealShotScalingNPC), true
}

// PhysicalSkillInput resolves the damage formula input for a physical skill
// cast by caster against h.
func (h *Hostile) PhysicalSkillInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	raceMul := h.RaceMultiplier(caster)
	return creature.ResolvePhysicalSkillInput(caster, h, def, creature.Playable(caster) && h.Kind().Playable(), raceMul)
}

// MagicDamageInput resolves the damage formula input for a magic skill cast by
// caster against h, rolling resist when magicFailures is set.
func (h *Hostile) MagicDamageInput(caster creature.FormulaActor, def modelskill.Definition, magicFailures bool) (formulas.MagicDamageInput, bool) {
	return creature.ResolveMagicDamageInput(caster, h, def, creature.Playable(caster) && h.Kind().Playable(), magicFailures)
}

// BlowInput resolves the damage formula input for a blow skill cast by caster
// against h.
func (h *Hostile) BlowInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.BlowInput, bool) {
	return creature.ResolveBlowInput(caster, h, def, creature.Playable(caster) && h.Kind().Playable())
}

func (h *Hostile) CounterSkillPhysical() float64 {
	return h.CalcStat(stat.CounterSkillPhysical, 0)
}

// CancelVulnerability returns h's CANCEL_VULN multiplier for the cancel and
// cancel-debuff success-rate formulas. classification is unused:
// CANCEL_VULN applies uniformly, without the per-classification switch the
// other _VULN stats use.
func (h *Hostile) CancelVulnerability(_ string) float64 {
	return h.CalcStat(stat.CancelVuln, 1)
}

// SkillReflectInput resolves h's reflected-skill chance for def.
func (h *Hostile) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	reflectStat := stat.ReflectSkillPhysic
	if def.Magic {
		reflectStat = stat.ReflectSkillMagic
	}
	return formulas.SkillReflectInput{
		IgnoreResists:  def.IgnoreResists,
		CanBeReflected: def.CanBeReflected,
		Magic:          def.Magic,
		CastRange:      def.CastRange,
		ReflectChance:  h.CalcStat(reflectStat, 0),
	}
}

// ManaDamageInput resolves the MP-damage formula input for a magic skill cast
// by caster against h.
func (h *Hostile) ManaDamageInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return creature.ResolveManaDamageInput(caster, h, h.MaxMPValue(), def)
}

// LethalRate returns h's lethal-strike rate multiplier.
func (h *Hostile) LethalRate() float64 {
	return h.calcStat(stat.LethalRate, 1)
}

// LethalInput resolves a lethal-strike roll against h.
func (h *Hostile) LethalInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.LethalInput, bool) {
	if h.Invul() || !creature.CanDealDamage(caster) {
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
		TargetLevel:   h.Level(),
		LethalMul:     attacker.LethalRate(),
	}, true
}

// ApplyLethalOutcome applies a lethal-strike tier to h. The HP loss rolls
// no cast break of its own.
func (h *Hostile) ApplyLethalOutcome(outcome formulas.LethalOutcome, caster attackable.Combatant, _ modelskill.Definition) {
	switch outcome {
	case formulas.LethalFull:
		h.reduceHP(h.HP()-1, caster)
	case formulas.LethalHalf:
		h.reduceHP(h.HP()/2, caster)
	}
}

// RaceMultiplier returns the physical-attack race term when h's template
// race has a paired attack/resist stat, or 1 otherwise.
func (h *Hostile) RaceMultiplier(attacker creature.FormulaActor) float64 {
	if attacker == nil {
		return 1
	}
	atk, res, ok := raceStats(h.Instance.Template.Race)
	if !ok {
		return 1
	}
	return 1 + ((attacker.CalcStat(atk, 1) - h.calcStat(res, 1)) / 100)
}

func raceStats(r Race) (atk, res stat.Stat, ok bool) {
	switch r {
	case RaceMagicCreature:
		return stat.PAtkMCreatures, stat.PDefMCreatures, true
	case RaceBeast:
		return stat.PAtkBeasts, stat.PDefBeasts, true
	case RaceAnimal:
		return stat.PAtkAnimals, stat.PDefAnimals, true
	case RacePlant:
		return stat.PAtkPlants, stat.PDefPlants, true
	case RaceDragon:
		return stat.PAtkDragons, stat.PDefDragons, true
	case RaceGiant:
		return stat.PAtkGiants, stat.PDefGiants, true
	case RaceBug:
		return stat.PAtkInsects, stat.PDefInsects, true
	default:
		return 0, 0, false
	}
}

// WeaponGradePenalty reports false: NPCs carry no weapon grade to be
// under-skilled for.
func (h *Hostile) WeaponGradePenalty() bool { return false }
