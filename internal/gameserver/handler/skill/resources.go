package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type restoredResource uint8

const (
	restoredHP restoredResource = iota
	restoredMP
	restoredCP
)

// expSPTarget is a creature that gains experience and SP: a player, or a
// pet. Any other creature ignores a grant.
type expSPTarget interface {
	AddExpAndSp(exp int64, sp int)
}

var (
	_ expSPTarget = (*player.Character)(nil)
	_ expSPTarget = (*summon.Actor)(nil)
)

// healHandler restores HP after landing the skill's effects through the
// BUFF handler, so a heal's heal-over-time, negate or buff reaches its
// targets the way a buff's would.
type healHandler struct {
	buff    continuousHandler
	healSps *modelskill.HealSpsTable
}

func (healHandler) Types() []string { return []string{"HEAL", "HEAL_STATIC"} }

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
	if skillTypeKey(cast.Skill.SkillType) != "HEAL_STATIC" && !cast.Skill.Potion {
		spendSpiritshot(cast, bsps)
	}
	return result
}

// healPercentHandler restores a percentage of each target's HP or MP after
// the BUFF pass has landed the skill's effects and spent its spiritshot.
type healPercentHandler struct {
	buff continuousHandler
}

func (healPercentHandler) Types() []string { return []string{"HEAL_PERCENT", "MANAHEAL_PERCENT"} }

func (h healPercentHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (h healPercentHandler) UseResult(cast Cast) Result {
	result := h.buff.UseResult(cast)
	isHP := skillTypeKey(cast.Skill.SkillType) == "HEAL_PERCENT"
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

func (manaHealHandler) Types() []string { return []string{"MANAHEAL", "MANARECHARGE"} }

func (manaHealHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		target, ok := asEffected(obj)
		if !ok || !target.CanBeHealed() {
			continue
		}
		mp := float64(cast.Skill.Power)
		if skillTypeKey(cast.Skill.SkillType) == "MANARECHARGE" {
			mp = target.RechargeMP(mp)
		}
		restored := target.AddMP(mp)
		sendRestoreStatus(obj, restored)
		notifyRestored(obj, cast.Caster, restored, restoredMP, true)
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

func (combatPointHealHandler) Types() []string { return []string{"COMBATPOINTHEAL"} }

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
		amount := float64(cast.Skill.Power)
		if target.CP()+amount > target.MaxCPValue() {
			amount = target.MaxCPValue() - target.CP()
		}
		// Setting CP reports the status even when nothing was missing.
		target.SetCP(target.CP() + amount)
		target.BroadcastStatus()
		notifyRestored(obj, cast.Caster, amount, restoredCP, true)
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

func notifyRestored(target, caster Actor, amount float64, resource restoredResource, playerCasterOnly bool) {
	notifier, ok := asPlayer(target)
	if !ok {
		return
	}
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

func (cpDamagePercentHandler) Types() []string { return []string{"CPDAMPERCENT"} }

func (h cpDamagePercentHandler) Use(cast Cast) {
	h.UseResult(cast)
}

// UseResult takes Power percent of each player target's CP. Per target the
// cast break rolls first, the caster hears of the damage, the CP drops and
// the target's status reports it, and last the target hears who dealt the
// damage — all of it even for a target left with no CP to lose.
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
		target.SetCP(target.CP() - float64(damage))
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

func (balanceLifeHandler) Types() []string { return []string{"BALANCE_LIFE"} }

func (h balanceLifeHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (h balanceLifeHandler) UseResult(cast Cast) Result {
	result := h.buff.UseResult(cast)
	targets := make([]Creature, 0, len(cast.Targets))
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
		fullHP += target.MaxHPValue()
		currentHP += target.HP()
		targets = append(targets, target)
	}

	if len(targets) == 0 || fullHP == 0 {
		return result
	}

	ratio := currentHP / fullHP
	for _, target := range targets {
		target.SetHP(target.MaxHPValue() * ratio)
		// A player's HP set reports its status whether or not the value
		// moved; a summon's or NPC's SetHP republishes its own.
		if p, ok := asPlayer(target); ok {
			p.BroadcastStatus()
		}
	}
	return result
}

type giveSPHandler struct{}

func (giveSPHandler) Types() []string { return []string{"GIVE_SP"} }

func (giveSPHandler) Use(cast Cast) {
	sp := int(cast.Skill.Power)
	for _, obj := range cast.Targets {
		if target, ok := obj.(expSPTarget); ok {
			target.AddExpAndSp(0, sp)
		}
	}
}

type realDamageHandler struct{}

func (realDamageHandler) Types() []string { return []string{"REAL_DAMAGE"} }

// Use takes the skill's power straight off each live creature target's HP.
// It is not a hit: CP, invulnerability, damage permission, hate and the
// wake-up side effects of damage play no part. A target the loss leaves at
// or below zero HP dies to the caster; any other target has its HP set and
// its status reported.
func (realDamageHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		target, ok := asCreature(obj)
		if !ok || target.Dead() {
			continue
		}
		hpLeft := target.HP() - float64(cast.Skill.Power)
		if hpLeft <= 0 {
			target.Kill(cast.Caster)
			continue
		}
		target.SetHP(hpLeft)
		// A summon's or NPC's SetHP reports its own status; a player's does
		// not.
		if p, ok := asPlayer(target); ok {
			p.BroadcastStatus()
		}
	}
}
