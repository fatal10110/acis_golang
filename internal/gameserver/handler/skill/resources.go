package skill

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type restoredResource uint8

const (
	restoredHP restoredResource = iota
	restoredMP
	restoredCP
)

// healHandler restores HP after landing the skill's effects through the
// BUFF handler, so a heal's heal-over-time, negate or buff reaches its
// targets the way a buff's would.
type healHandler struct {
	buff    continuousHandler
	healSps *modelskill.HealSpsTable
}

func (healHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeHeal, modelskill.SkillTypeHealStatic}
}

func (h healHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (h healHandler) UseResult(cast Cast) Result {
	if cast.Caster == nil {
		return Result{messages: cast.messages}
	}
	// The shot state is sampled before the BUFF pass, which may spend the
	// shot itself; the heal amount and the discharge below both use it.
	sps, bsps := spiritshotCharges(cast.Caster)
	result := h.buff.UseResult(cast)

	if in, ok := cast.Caster.HealInput(cast.Skill); ok {
		in.Spiritshot, in.BlessedSpiritshot = sps, bsps
		if (sps || bsps) && !in.Static {
			in.SpsCorrection = h.healSps.Calculate(cast.Skill.ID, cast.Skill.Level, cast.Skill.MagicLevel, in.MAtk)
		}
		amount := formulas.HealAmount(in)
		for _, obj := range cast.Targets {
			target, ok := asEffected(obj)
			if !ok || !target.CanBeHealed() {
				continue
			}
			restored := target.AddHP(amount * target.HealEffectiveness() / 100)
			sendRestoreStatus(obj, restored)
			notifyRestored(obj, cast.Caster, restored, restoredHP, false)
		}
	}
	// Heal's own discharge skips a static heal and a potion; any spend for
	// those comes from the BUFF pass above.
	if skillTypeKey(cast.Skill.SkillType) != modelskill.SkillTypeHealStatic && !cast.Skill.Potion {
		spendSpiritshot(cast, bsps)
	}
	return result
}

// healPercentHandler restores a percentage of each target's HP or MP after
// the BUFF pass has landed the skill's effects and spent its spiritshot.
type healPercentHandler struct {
	buff continuousHandler
}

func (healPercentHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeHealPercent, modelskill.SkillTypeManaHealPercent}
}

func (h healPercentHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (h healPercentHandler) UseResult(cast Cast) Result {
	result := h.buff.UseResult(cast)
	isHP := skillTypeKey(cast.Skill.SkillType) == modelskill.SkillTypeHealPercent
	for _, obj := range cast.Targets {
		target, ok := asCreature(obj)
		if !ok || !target.CanBeHealed() {
			continue
		}
		if isHP {
			restored := target.AddHP(target.MaxHPValue() * float64(cast.Skill.Power) / 100)
			sendRestoreStatus(obj, restored)
			notifyRestored(obj, cast.Caster, restored, restoredHP, false)
			continue
		}
		restored := target.AddMP(target.MaxMPValue() * float64(cast.Skill.Power) / 100)
		sendRestoreStatus(obj, restored)
		notifyRestored(obj, cast.Caster, restored, restoredMP, false)
	}
	return result
}

type manaHealHandler struct{}

func (manaHealHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeManaHeal, modelskill.SkillTypeManaRecharge}
}

func (manaHealHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		target, ok := asEffected(obj)
		if !ok || !target.CanBeHealed() {
			continue
		}
		mp := float64(cast.Skill.Power)
		if skillTypeKey(cast.Skill.SkillType) == modelskill.SkillTypeManaRecharge {
			mp = target.RechargeMP(mp)
		}
		restored := target.AddMP(mp)
		sendRestoreStatus(obj, restored)
		// A summon target's message reaches its owner.
		if sink, ok := restoreSink(obj); ok {
			notifyRestoredTo(sink, obj, cast.Caster, restored, restoredMP, true)
		}
	}
	applySelfEffects(cast, cast.Skill)
	if !cast.Skill.Potion {
		dischargeSpiritshot(cast)
	}
}

// combatPointHealHandler restores a flat amount of each player target's CP
// after the BUFF pass has landed the skill's effects and spent its
// spiritshot.
type combatPointHealHandler struct {
	buff continuousHandler
}

func (combatPointHealHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeCombatPointHeal}
}

func (h combatPointHealHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (h combatPointHealHandler) UseResult(cast Cast) Result {
	result := h.buff.UseResult(cast)
	for _, obj := range cast.Targets {
		target, ok := asPlayer(obj)
		if !ok || target.Dead() || target.Invulnerable() {
			continue
		}
		applied := target.AddCP(float64(cast.Skill.Power))
		// The status goes out even when nothing was missing.
		target.BroadcastStatus()
		notifyRestored(obj, cast.Caster, applied, restoredCP, true)
	}
	return result
}

// sendRestoreStatus sends a player target its own status for a restore that
// applied anything, ahead of the restored message; a restore that applied
// nothing sends none. A summon or NPC already republished its status from
// its own vitals mutator.
func sendRestoreStatus(target Actor, applied float64) {
	if p, ok := asPlayer(target); ok && applied > 0 {
		p.BroadcastStatus()
	}
}

// restoreSink returns the player that receives the messages sent to target:
// a player itself, or a summon's owner.
func restoreSink(target Actor) (Player, bool) {
	if s, ok := asSummon(target); ok {
		return asPlayer(s.SummonOwner())
	}
	return asPlayer(target)
}

// notifyRestored tells a player target what a restore gave it; any other
// target is told nothing.
func notifyRestored(target, caster Actor, amount float64, resource restoredResource, playerCasterOnly bool) {
	if notifier, ok := asPlayer(target); ok {
		notifyRestoredTo(notifier, target, caster, amount, resource, playerCasterOnly)
	}
}

// notifyRestoredTo sends notifier the restored message for target: the
// caster's name and the amount when another actor restored it, else the
// amount alone. With playerCasterOnly only a player caster counts as
// another actor.
func notifyRestoredTo(notifier Player, target, caster Actor, amount float64, resource restoredResource, playerCasterOnly bool) {
	name := actorName(caster)
	byOther := !sameObject(caster, target)
	if playerCasterOnly {
		_, casterIsPlayer := asPlayer(caster)
		byOther = casterIsPlayer && byOther
	}
	switch resource {
	case restoredMP:
		notifier.NotifyMPRestored(name, int(amount), byOther)
	case restoredCP:
		notifier.NotifyCPRestored(name, int(amount), byOther)
	default:
		notifier.NotifyHPRestored(name, int(amount), byOther)
	}
}

type cpDamagePercentHandler struct{}

func (cpDamagePercentHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeCPDamPercent}
}

func (h cpDamagePercentHandler) Use(cast Cast) {
	h.UseResult(cast)
}

// UseResult takes Power percent of each player target's CP. Per target the
// cast break rolls first, the caster hears of the damage, the CP drops and
// the target's status reports it, and last the target hears who dealt the
// damage — all of it even for a target left with no CP to lose.
//
// No shield roll is made and no effect template lands on the target.
// Accepted simplification: the only CPDAMPERCENT skill shipped, Wrath (320),
// carries no effect, so neither is observable. A CPDAMPERCENT skill that
// gains an effect needs both, the shield result passed to its landing.
func (cpDamagePercentHandler) UseResult(cast Cast) Result {
	result := Result{messages: cast.messages}
	if alikeDead(cast.Caster) {
		return result
	}
	for _, obj := range cast.Targets {
		target, ok := asPlayer(obj)
		if !ok || target.Dead() || target.Invulnerable() {
			continue
		}
		damage := int(target.CP() * float64(cast.Skill.Power) / 100)
		target.BreakCastOnDamage(float64(damage))
		recordDamage(&result, cast.Caster, target, damage, false, false)
		// The loss lands as a delta, so a hit since the read keeps its share.
		target.ReduceCP(float64(damage))
		if sameObject(cast.Caster, target) {
			// A player caster's own status goes out in order among the
			// cast's messages.
			result.record(CasterVitalsChanged{})
		} else {
			target.BroadcastStatus()
		}
		result.record(DamageReceived{TargetID: target.ObjectID(), AttackerName: actorName(cast.Caster), Amount: int32(damage)})
	}
	// Spent even when no target was accepted.
	dischargeSoulshot(cast)
	return result
}

// balanceLifeHandler shares the living targets' pooled HP ratio after the
// BUFF pass has landed the skill's effects and spent its spiritshot.
type balanceLifeHandler struct {
	buff continuousHandler
}

func (balanceLifeHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeBalanceLife}
}

func (h balanceLifeHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (h balanceLifeHandler) UseResult(cast Cast) Result {
	result := h.buff.UseResult(cast)
	targets := make([]Creature, 0, len(cast.Targets))
	snapshot := make([]float64, 0, len(cast.Targets))
	var fullHP, currentHP float64
	casterCursed := cursed(cast.Caster)

	for _, obj := range cast.Targets {
		target, ok := asCreature(obj)
		if !ok || target.Dead() {
			continue
		}
		if !sameObject(obj, cast.Caster) && (casterCursed || cursed(obj)) {
			continue
		}
		hp := target.HP()
		fullHP += target.MaxHPValue()
		currentHP += hp
		targets = append(targets, target)
		snapshot = append(snapshot, hp)
	}

	if len(targets) == 0 || fullHP == 0 {
		return result
	}

	// The pooled ratio comes from the snapshot above; each member then moves
	// by its own share as a delta under its own vitals lock, so a hit landing
	// since the snapshot keeps its effect. A share that would leave a member
	// no HP writes nothing: the skill never kills.
	ratio := currentHP / fullHP
	for i, target := range targets {
		target.ShiftHP(target.MaxHPValue()*ratio - snapshot[i])
		// A player's status goes out whether or not the value moved; a
		// summon's or NPC's ShiftHP republishes its own.
		if p, ok := asPlayer(target); ok {
			p.BroadcastStatus()
		}
	}
	return result
}

type giveSPHandler struct{}

func (giveSPHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeGiveSP}
}

func (giveSPHandler) Use(cast Cast) {
	sp := int(cast.Skill.Power)
	// Only a player or a summon gains SP; any other creature ignores a grant.
	for _, obj := range cast.Targets {
		if p, ok := asPlayer(obj); ok {
			p.AddExpAndSp(0, sp)
		} else if s, ok := asSummon(obj); ok {
			s.AddExpAndSp(0, sp)
		}
	}
}

type realDamageHandler struct{}

func (realDamageHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeRealDamage}
}

// Use takes the skill's power straight off each live creature target's HP,
// a door's included. It is not a hit: CP, invulnerability, damage
// permission, hate and the wake-up side effects of damage play no part. The
// loss lands as a delta under the target's vitals lock, so a hit landing
// alongside it keeps its share. A target the loss would leave at or below
// zero HP dies to the caster; any other target has its status reported.
func (realDamageHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		if d, ok := asDoor(obj); ok {
			d.TakeHP(float64(cast.Skill.Power))
			continue
		}
		target, ok := asCreature(obj)
		if !ok || target.Dead() {
			continue
		}
		if target.ShiftHP(-float64(cast.Skill.Power)) {
			target.Kill(cast.Caster)
			continue
		}
		// A summon's or NPC's ShiftHP reports its own status; a player's
		// does not.
		if p, ok := asPlayer(target); ok {
			p.BroadcastStatus()
		}
	}
}
