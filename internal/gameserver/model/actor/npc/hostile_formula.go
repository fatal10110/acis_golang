package npc

import (
	"math"
	"math/rand"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
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

// PAtk returns this NPC's physical attack stat.
func (h *Hostile) PAtk() float64 {
	return h.calcStat(stat.PowerAttack, h.Instance.Template.PAtk)
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
func (h *Hostile) MaxHPValue() float64 {
	return h.calcStat(stat.MaxHP, h.Instance.Template.HPMax)
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
func (h *Hostile) ReduceHP(amount float64, attacker creature.DeathActor, _ modelskill.Definition) {
	if h.AlikeDead() {
		return
	}
	h.testOverhit(attacker, amount)
	if amount <= 0 || h.Invul() || !creature.CanDealDamage(attacker) {
		return
	}
	if combatant, ok := attacker.(attackable.Combatant); ok {
		h.AddCombatDamageHate(combatant, amount)
		h.RollAttackedShotRecharge()
	}
	h.applyNonConsumptionDamageEffects(false)
	newlyDead := h.health.DamageValue(amount)
	if err := h.BroadcastStatus(); err != nil {
		h.log.Warn().Err(err).Msg("npc: status broadcast")
	}
	if !newlyDead {
		return
	}
	killer, _ := attacker.(creature.DeathActor)
	h.Die(killer, h.rewards)
}

// ReduceHPByDOT applies periodic damage and records it in the threat table
// at zero hate weight, matching Npc.reduceCurrentHp's unconditional
// addDamageHate(attacker, damage, 0) — every HP reduction feeds the
// AggroList, DOT included (Npc.java:390-395; no isDOT gate in the chain
// Creature.reduceCurrentHpByDOT -> Npc.reduceCurrentHp -> reduceHp).
func (h *Hostile) ReduceHPByDOT(amount float64, attacker effect.Participant, isDOT bool) {
	if h.AlikeDead() {
		return
	}
	var killer creature.DeathActor
	if a, ok := attacker.(creature.DeathActor); ok {
		killer = a
	}
	h.testOverhit(killer, amount)
	if amount <= 0 || h.Invul() || !creature.CanDealDamage(attacker) {
		return
	}
	if combatant, ok := attacker.(attackable.Combatant); ok {
		h.AddDamageHate(combatant, amount, 0)
		h.AddAttackDesire(combatant, 200)
		h.RollAttackedShotRecharge()
	}
	h.applyNonConsumptionDamageEffects(isDOT)
	newlyDead := h.health.DamageValue(amount)
	if err := h.BroadcastStatus(); err != nil {
		h.log.Warn().Err(err).Msg("npc: status broadcast")
	}
	if !newlyDead {
		return
	}
	h.Die(killer, h.rewards)
}

// applyNonConsumptionDamageEffects mirrors NpcStatus.reduceHp's inherited
// CreatureStatus block (CreatureStatus.java:228-248): non-DOT HP reduction
// stops SLEEP and IMMOBILE_UNTIL_ATTACKED, and has a 1-in-10 chance to break
// STUN. NpcStatus.reduceHp (NpcStatus.java:21-35) adds only a duel-interrupt
// check on the attacker and otherwise delegates unchanged, including the
// isDOT gate on the whole block — unlike PlayerStatus, which overrides the
// gate to !isHPConsumption alone and stun-breaks separately on !isDOT
// (PlayerStatus.java:118-134). There is no isHPConsumption concept for NPCs
// and no sit/stand-up clause (Player-only, PlayerStatus.java:124). Callers
// must run this after AddDamageHate, matching Npc.reduceCurrentHp
// (Npc.java:395, 468): the hate lands before super.reduceCurrentHp reaches
// this block, so a sleep-stop's synchronous wake-think (EffectSleep.java:37-44
// -> hooks_cc.go's thinkAndRefreshExit) always sees the hit's hate already
// in the threat table.
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
func (h *Hostile) PhysicalSkillInput(caster creature.DeathActor, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	attacker, _ := caster.(creature.FormulaActor)
	raceMul := h.raceMultiplier(attacker)
	return creature.ResolvePhysicalSkillInput(caster, h, def, creature.Playable(caster) && h.Playable(), raceMul)
}

// MagicDamageInput resolves the damage formula input for a magic skill cast by
// caster against h.
func (h *Hostile) MagicDamageInput(caster creature.DeathActor, def modelskill.Definition) (formulas.MagicDamageInput, bool) {
	return creature.ResolveMagicDamageInput(caster, h, def, creature.Playable(caster) && h.Playable())
}

// BlowInput resolves the damage formula input for a blow skill cast by caster
// against h.
func (h *Hostile) BlowInput(caster creature.DeathActor, def modelskill.Definition) (formulas.BlowInput, bool) {
	return creature.ResolveBlowInput(caster, h, def, creature.Playable(caster) && h.Playable())
}

func (h *Hostile) CounterSkillPhysical() float64 {
	return h.CalcStat(stat.CounterSkillPhysical, 0)
}

// CancelVulnerability returns h's CANCEL_VULN multiplier for the cancel and
// cancel-debuff success-rate formulas (Formulas.java:949-951). classification
// is unused: the reference applies CANCEL_VULN uniformly, without the
// per-classification switch it uses for the other _VULN stats.
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
func (h *Hostile) ManaDamageInput(caster creature.DeathActor, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return creature.ResolveManaDamageInput(caster, h, h.MaxMPValue(), def)
}

// LethalRate returns h's lethal-strike rate multiplier.
func (h *Hostile) LethalRate() float64 {
	return h.calcStat(stat.LethalRate, 1)
}

// LethalInput resolves a lethal-strike roll against h.
func (h *Hostile) LethalInput(caster creature.DeathActor, def modelskill.Definition) (formulas.LethalInput, bool) {
	if h.Invul() || !creature.CanDealDamage(caster) {
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
		TargetLevel:   h.Level(),
		LethalMul:     attacker.LethalRate(),
	}, true
}

// ApplyLethalOutcome applies a lethal-strike tier to h.
func (h *Hostile) ApplyLethalOutcome(outcome formulas.LethalOutcome, caster creature.DeathActor, def modelskill.Definition) {
	switch outcome {
	case formulas.LethalFull:
		h.ReduceHP(h.HP()-1, caster, def)
	case formulas.LethalHalf:
		h.ReduceHP(h.HP()/2, caster, def)
	}
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
