package npc

import (
	"math"
	"math/rand"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Category reports h as an attackable actor for skill target resolution.
func (h *Hostile) Category() skilltarget.Category {
	return skilltarget.CategoryAttackable
}

// Attackable reports that h is an NPC-like combat target.
func (h *Hostile) Attackable() bool { return true }

// Playable reports whether h is player-controlled.
func (h *Hostile) Playable() bool { return false }

// Invul reports whether h is currently invulnerable.
func (h *Hostile) Invul() bool { return false }

// Invulnerable reports whether h ignores direct resource effects.
func (h *Hostile) Invulnerable() bool { return h.Invul() }

// PAtk returns this NPC's physical attack stat.
func (h *Hostile) PAtk() float64 {
	return h.calcStat(stat.PowerAttack, h.Instance.Template.PAtk)
}

// MagicCriticalRate returns this NPC's magic critical rate.
func (h *Hostile) MagicCriticalRate() float64 {
	return h.calcStat(stat.MCriticalRate, 8)
}

// SpiritshotCharged reports whether a spiritshot charge is currently active.
func (h *Hostile) SpiritshotCharged() bool { return false }

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

// RandomDamageSpread returns the template-defined random-damage spread.
func (h *Hostile) RandomDamageSpread() int {
	return h.Instance.Template.BaseRandomDamage
}

// HP returns current HP as a floating-point skill-resource value.
func (h *Hostile) HP() float64 {
	return h.health.Current()
}

// MaxHPValue returns maximum HP as a floating-point skill-resource value.
func (h *Hostile) MaxHPValue() float64 {
	return h.calcStat(stat.MaxHP, h.Instance.Template.HPMax)
}

// MPValue returns current MP as a floating-point skill-resource value.
func (h *Hostile) MPValue() float64 {
	h.mpMu.RLock()
	defer h.mpMu.RUnlock()
	return h.mp
}

// MaxMPValue returns maximum MP as a floating-point skill-resource value.
func (h *Hostile) MaxMPValue() float64 {
	return h.calcStat(stat.MaxMP, h.Instance.Template.MPMax)
}

// SetHP sets current HP, clamped to [0, MaxHP].
func (h *Hostile) SetHP(value float64) {
	maxHP := h.MaxHPValue()
	if value < 0 {
		value = 0
	}
	if value > maxHP {
		value = maxHP
	}
	h.health.SetCurrent(value)
}

// AddHP restores HP, clamped to MaxHP, and returns the applied amount.
func (h *Hostile) AddHP(amount float64) float64 {
	return h.health.Add(amount, h.MaxHPValue())
}

// AddMP restores MP, clamped to MaxMP, and returns the applied amount.
func (h *Hostile) AddMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxMP := h.MaxMPValue()
	h.mpMu.Lock()
	defer h.mpMu.Unlock()
	if h.mp >= maxMP {
		return 0
	}
	if h.mp+amount > maxMP {
		amount = maxMP - h.mp
	}
	h.mp += amount
	return amount
}

// ReduceMP subtracts MP, clamped at zero, and returns the applied amount.
func (h *Hostile) ReduceMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	h.mpMu.Lock()
	defer h.mpMu.Unlock()
	if h.mp <= 0 {
		return 0
	}
	if amount > h.mp {
		amount = h.mp
	}
	h.mp -= amount
	return amount
}

// ReduceHP applies skill HP damage and runs the once-only death path.
func (h *Hostile) ReduceHP(amount float64, attacker any, _ modelskill.Definition) {
	if amount <= 0 || h.AlikeDead() {
		return
	}
	if combatant, ok := attacker.(attackable.Combatant); ok {
		h.AddDamageHate(combatant, amount, amount)
	}
	newlyDead := h.health.DamageValue(amount)
	h.BroadcastStatus()
	if !newlyDead {
		return
	}
	killer, _ := attacker.(creature.DeathActor)
	h.Die(killer, h.rewards)
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

// HealAmount resolves h's outgoing HEAL amount before target effectiveness.
func (h *Hostile) HealAmount(def modelskill.Definition) (float64, bool) {
	amount := float64(def.Power) + h.HealProficiency()
	if creature.SkillTypeKey(def.SkillType) == "HEAL_STATIC" {
		return amount, true
	}
	return amount + math.Sqrt(float64(int(h.MAtk()))), true
}

// PhysicalSkillInput resolves the damage formula input for a physical skill
// cast by caster against h.
func (h *Hostile) PhysicalSkillInput(caster any, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	attacker, _ := caster.(creature.FormulaActor)
	raceMul := h.raceMultiplier(attacker)
	return creature.ResolvePhysicalSkillInput(caster, h, def, creature.Playable(caster) && h.Playable(), raceMul)
}

// MagicDamageInput resolves the damage formula input for a magic skill cast by
// caster against h.
func (h *Hostile) MagicDamageInput(caster any, def modelskill.Definition) (formulas.MagicDamageInput, bool) {
	return creature.ResolveMagicDamageInput(caster, h, def, creature.Playable(caster) && h.Playable())
}

// BlowInput resolves the damage formula input for a blow skill cast by caster
// against h.
func (h *Hostile) BlowInput(caster any, def modelskill.Definition) (formulas.BlowInput, bool) {
	return creature.ResolveBlowInput(caster, h, def, creature.Playable(caster) && h.Playable())
}

// ManaDamageInput resolves the MP-damage formula input for a magic skill cast
// by caster against h.
func (h *Hostile) ManaDamageInput(caster any, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return creature.ResolveManaDamageInput(caster, h, h.MaxMPValue(), def)
}

func (h *Hostile) raceMultiplier(attacker creature.FormulaActor) float64 {
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
