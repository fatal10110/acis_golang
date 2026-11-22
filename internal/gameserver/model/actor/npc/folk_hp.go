package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// folkMinHP is the HP a hit leaves an undying civilian NPC at, at least.
const folkMinHP = 1

// folkDeathHP is the HP under which a civilian NPC dies.
const folkDeathHP = 0.5

// HP returns the current HP.
func (f *Folk) HP() float64 {
	f.vitalsMu.Lock()
	defer f.vitalsMu.Unlock()
	return f.hp
}

// CurrentHP returns the current HP in whole points.
func (f *Folk) CurrentHP() int { return int(f.HP()) }

// MPValue returns the current MP.
func (f *Folk) MPValue() float64 {
	f.vitalsMu.Lock()
	defer f.vitalsMu.Unlock()
	return f.mp
}

// CurrentMP returns the current MP in whole points.
func (f *Folk) CurrentMP() int { return int(f.MPValue()) }

// PublishHP hands send the current HP when the players targeting the NPC
// must be sent it, under the health-bar lock (creature.HPBar.Publish).
func (f *Folk) PublishHP(send func(hp int)) {
	f.hpBar.Publish(f.HP, f.MaxHPValue(), send)
}

// BroadcastStatus offers the players targeting the NPC its HP, then
// settles its regeneration task: every vitals change reports through here.
func (f *Folk) BroadcastStatus() {
	f.emit(event.HPChanged{})
	f.SettleRegen()
}

// TakeDamage applies a landed auto-attack hit and reports whether it
// killed the NPC.
func (f *Folk) TakeDamage(damage int, attacker attackable.Combatant) bool {
	return f.reduceHP(float64(damage), attacker, modelskill.Ref{}, lossHit)
}

// ReduceHP applies skill damage.
func (f *Folk) ReduceHP(amount float64, attacker attackable.Combatant, def modelskill.Definition) {
	f.reduceHP(amount, attacker, skillRef(def), lossHit)
}

// ReduceHPByDOT applies periodic damage; isDOT false takes it as a plain
// hit.
func (f *Folk) ReduceHPByDOT(amount float64, attacker effect.Actor, isDOT bool) {
	killer, _ := attacker.(attackable.Combatant)
	loss := lossDOT
	if !isDOT {
		loss = lossHit
	}
	f.reduceHP(amount, killer, modelskill.Ref{}, loss)
}

// ReduceHPBySkillDOT is ReduceHPByDOT for one damage-over-time tick of the
// skill sk: the hit it registers names sk.
func (f *Folk) ReduceHPBySkillDOT(amount float64, attacker effect.Actor, sk modelskill.Ref) {
	killer, _ := attacker.(attackable.Combatant)
	f.reduceHP(amount, killer, sk, lossDOT)
}

// SkillAttacked is caster's offensive skill def landing on this NPC, once
// its effects applied, for a skill that is a debuff or carries aggro
// points: the attacked hooks run with max(120, aggro points) as the damage,
// then the party is called as for a hit, with no built-in assist. A dead
// NPC is called too. Unlike a hostile NPC, a civilian NPC makes no clan
// call for a skill: a clan member is never told of one.
func (f *Folk) SkillAttacked(caster attackable.Combatant, def modelskill.Definition) {
	if caster == nil {
		return
	}
	value := max(skillAggroFloor, def.AggroPoints)
	f.raiseAttacked(caster, int32(value), skillRef(def))
	propagatePartyAttacked(f, caster, value, false)
}

// raiseAttacked runs f's attacked hooks.
func (f *Folk) raiseAttacked(attacker attackable.Combatant, damage int32, sk modelskill.Ref) {
	if f.scripts != nil {
		f.scripts.FolkAttacked(f, attacker, damage, sk)
	}
}

// reduceHP is an NPC's HP reduction: a dead NPC takes nothing; a hit from a
// creature first puts a walking NPC in run stance and adds the damage to its
// aggro list, at no hate, then runs its attacked hooks with the damage
// truncated to an int and sk the skill that dealt it, then calls its party,
// then makes its clan calls: itself, then every NPC of its clan in range. An
// invulnerable NPC then takes nothing but its own damage-over-time ticks and
// HP costs, and a hit from an attacker without damage permission nothing at
// all. A hit that is no damage-over-time tick then wakes the
// NPC (wakeOnHit).
// The HP left is never under folkMinHP for an undying template, nor under
// zero otherwise, and an NPC left under folkDeathHP dies. It reports
// whether this call killed the NPC. A holy artifact takes no hit at all;
// a siege headquarters runs its own hit first (headquartersHit), and a
// Swoop Cannon's hit on it stops there; a control tower runs its own after
// (towerHit).
func (f *Folk) reduceHP(amount float64, attacker attackable.Combatant, sk modelskill.Ref, loss hpLoss) bool {
	if f.HolyThing() {
		return false
	}
	if f.SiegeFlag() && !f.headquartersHit(attacker) {
		return false
	}
	killed := f.loseHP(amount, attacker, sk, loss)
	if f.Tower() {
		f.towerHit()
	}
	return killed
}

// hpLoss is what takes an NPC's HP.
type hpLoss uint8

const (
	// lossHit is a hit: it adds aggro, runs the attacked hooks and the clan
	// calls, and wakes the NPC.
	lossHit hpLoss = iota
	// lossDOT is a damage-over-time tick: a hit that wakes nothing.
	lossDOT
	// lossCost is a skill's HP cost: no hit at all.
	lossCost
)

// loseHP is reduceHP; lossCost leaves out the run stance, the aggro, the
// attacked hooks and the clan calls. An invulnerable NPC still takes its own
// damage-over-time ticks and pays its own HP costs.
func (f *Folk) loseHP(amount float64, attacker attackable.Combatant, sk modelskill.Ref, loss hpLoss) bool {
	if f.Dead() {
		return false
	}
	creature.InterruptDuelOnNPCHit(attacker)
	if attacker != nil && loss != lossCost {
		f.forceRunStance()
		f.fight.aggro.AddDamage(attacker, amount, 0)
		damage := commons.TruncInt32Sat(amount)
		f.raiseAttacked(attacker, damage, sk)
		propagatePartyAttacked(f, attacker, int(damage), false)
		raiseClanAttacked(f.scripts, f.world, f, attacker, damage, sk, clanAttackedByHit)
	}
	if f.Invul() && (loss == lossHit || attacker != attackable.Combatant(f)) {
		return false
	}
	if !creature.CanDealDamage(attacker) {
		return false
	}
	if loss == lossHit {
		f.wakeOnHit()
	}
	floor := 0.0
	if f.Instance.Template.Undying {
		floor = folkMinHP
	}
	// A NaN amount takes nothing, like a negative one.
	damaged := amount > 0
	f.vitalsMu.Lock()
	if f.dead {
		f.vitalsMu.Unlock()
		return false
	}
	if damaged {
		f.hp = math.Max(f.hp-amount, floor)
	}
	dying := f.hp < folkDeathHP
	f.vitalsMu.Unlock()
	if damaged {
		f.BroadcastStatus()
	}
	return dying && f.die(attacker)
}

// wakeOnHit ends the sleep and the hold-until-attacked effects the NPC
// holds, and one time in ten its stun. Only a control tower holds any.
func (f *Folk) wakeOnHit() {
	f.effects.StopByType(effect.TypeSleep)
	f.effects.StopByType(effect.TypeImmobileUntilAttacked)
	if f.Stunned() && f.Roll(10) == 0 {
		f.effects.StopByType(effect.TypeStun)
	}
}

// SetHP sets the current HP, clamped to [0, max HP], and offers the
// targeters' health bar a refresh. A dead NPC's HP stays as it is.
func (f *Folk) SetHP(value float64) {
	value = min(max(value, 0), f.MaxHPValue())
	f.vitalsMu.Lock()
	if f.dead {
		f.vitalsMu.Unlock()
		return
	}
	f.hp = value
	f.vitalsMu.Unlock()
	f.BroadcastStatus()
}

// ShiftHP adds delta, which may be negative, to the current HP, capped at
// max HP, and offers the targeters' health bar a refresh. A change that would
// leave no HP writes nothing and reports true, leaving the death to the
// caller. A dead NPC stays as it is and reports false.
func (f *Folk) ShiftHP(delta float64) (lethal bool) {
	maxHP := f.MaxHPValue()
	f.vitalsMu.Lock()
	if f.dead {
		f.vitalsMu.Unlock()
		return false
	}
	hp := f.hp + delta
	if hp <= 0 {
		f.vitalsMu.Unlock()
		return true
	}
	f.hp = min(hp, maxHP)
	f.vitalsMu.Unlock()
	f.BroadcastStatus()
	return false
}

// SetMaxHpMp fills the NPC's MP, then its HP, to their maxima and offers
// the targeters' health bar one refresh. A dead NPC keeps its values and
// reports nothing.
func (f *Folk) SetMaxHpMp() {
	maxHP, maxMP := f.MaxHPValue(), f.MaxMPValue()
	f.vitalsMu.Lock()
	if f.dead {
		f.vitalsMu.Unlock()
		return
	}
	f.mp = maxMP
	f.hp = maxHP
	f.vitalsMu.Unlock()
	f.BroadcastStatus()
}

// Kill puts the NPC to death whatever its HP, undying or not, and reports
// whether this call killed it.
func (f *Folk) Kill(killer attackable.Combatant) bool { return f.die(killer) }

// AddHP restores HP, clamped to max HP, and returns the amount applied;
// anything applied refreshes the targeters' health bar.
func (f *Folk) AddHP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxHP := f.MaxHPValue()
	f.vitalsMu.Lock()
	if f.dead {
		f.vitalsMu.Unlock()
		return 0
	}
	applied := max(min(amount, maxHP-f.hp), 0)
	f.hp += applied
	f.vitalsMu.Unlock()
	if applied > 0 {
		f.BroadcastStatus()
	}
	return applied
}

// AddMP restores MP, clamped to max MP, and returns the amount applied.
func (f *Folk) AddMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxMP := f.MaxMPValue()
	f.vitalsMu.Lock()
	if f.dead {
		f.vitalsMu.Unlock()
		return 0
	}
	applied := max(min(amount, maxMP-f.mp), 0)
	f.mp += applied
	f.vitalsMu.Unlock()
	if applied > 0 {
		f.BroadcastStatus()
	}
	return applied
}

// ReduceMP removes MP, clamped at zero, and returns the amount removed.
func (f *Folk) ReduceMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	f.vitalsMu.Lock()
	if f.dead {
		f.vitalsMu.Unlock()
		return 0
	}
	applied := min(amount, f.mp)
	f.mp -= applied
	f.vitalsMu.Unlock()
	if applied > 0 {
		f.BroadcastStatus()
	}
	return applied
}

// TickRegen applies one HP/MP regeneration step: each resource short of
// its max gains its regen rate, at least 1, and the HP is offered to the
// targeters when anything changed. A dead NPC regenerates nothing. Either
// way the task is then settled, so a tick that fills the NPC, or finds it
// dead, stops it.
func (f *Folk) TickRegen() {
	maxHP, maxMP := f.MaxHPValue(), f.MaxMPValue()
	f.vitalsMu.Lock()
	needHP, needMP := f.hp < maxHP, f.mp < maxMP
	dead := f.dead
	f.vitalsMu.Unlock()
	if dead || (!needHP && !needMP) {
		f.SettleRegen()
		return
	}
	hpRegen, mpRegen := math.Max(1, f.HPRegenRate()), math.Max(1, f.MPRegenRate())
	f.vitalsMu.Lock()
	if f.dead {
		f.vitalsMu.Unlock()
		f.SettleRegen()
		return
	}
	changed := false
	if f.hp < maxHP {
		f.hp = math.Min(f.hp+hpRegen, maxHP)
		changed = true
	}
	if f.mp < maxMP {
		f.mp = math.Min(f.mp+mpRegen, maxMP)
		changed = true
	}
	f.vitalsMu.Unlock()
	if changed {
		f.BroadcastStatus()
		return
	}
	f.SettleRegen()
}

// Regen returns the NPC's regeneration phase, which the regeneration sweep
// polls and claims.
func (f *Folk) Regen() *creature.Regen { return &f.regen }

// SettleRegen arms the NPC's regeneration task when it is spawned, alive
// and short of HP or MP, its first tick one period from now, and disarms
// it otherwise; every HP or MP change settles it so.
func (f *Folk) SettleRegen() {
	f.regen.Settle(f.Queue(), f.RegenShort)
}

// RegenShort reports whether the NPC regenerates: spawned (on the grid or
// off it mid-relocation), not dead, and below its maximum HP or MP.
func (f *Folk) RegenShort() bool {
	if !f.Spawned() {
		return false
	}
	maxHP, maxMP := f.MaxHPValue(), f.MaxMPValue()
	f.vitalsMu.Lock()
	defer f.vitalsMu.Unlock()
	return !f.dead && (f.hp < maxHP || f.mp < maxMP)
}

// Invul reports whether the NPC is invulnerable.
func (f *Folk) Invul() bool { return f.invul.Load() }

// Invulnerable reports whether the NPC ignores direct resource effects.
func (f *Folk) Invulnerable() bool { return f.Invul() }

// SetInvul sets invulnerability and reports whether it changed.
func (f *Folk) SetInvul(v bool) bool { return f.invul.Swap(v) != v }

// CanBeHealed reports whether the NPC may be healed.
func (f *Folk) CanBeHealed() bool { return !f.Invul() && !f.SiegeFlag() }

// HealEffectiveness returns the percentage incoming heals are scaled by.
func (f *Folk) HealEffectiveness() float64 { return f.CalcStat(stat.HealEffectiveness, 100) }

// HealProficiency returns the flat heal-power bonus.
func (f *Folk) HealProficiency() float64 { return f.CalcStat(stat.HealProficiency, 0) }

// RechargeMP applies the MP recharge multiplier to amount.
func (f *Folk) RechargeMP(amount float64) float64 { return f.CalcStat(stat.RechargeMPRate, amount) }

// HealInput resolves the NPC's side of an outgoing heal.
func (f *Folk) HealInput(def modelskill.Definition) (formulas.HealInput, bool) {
	return creature.ResolveHealInput(def, f.HealProficiency(), f.Against(f.ConditionActor(), nil).MAtk(), formulas.HealShotScalingNPC), true
}
