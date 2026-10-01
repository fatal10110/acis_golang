package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// folkMinHP is the HP a hit leaves a civilian NPC at, at least: no damage
// kills it.
const folkMinHP = 1

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

// HPStatusUpdate returns the current HP and whether the players targeting
// the NPC must be sent it.
func (f *Folk) HPStatusUpdate() (int, bool) {
	return f.hpBar.Report(f.HP, f.MaxHPValue())
}

// BroadcastStatus offers the players targeting the NPC its HP.
func (f *Folk) BroadcastStatus() { f.emit(event.HPChanged{}) }

// TakeDamage applies a landed auto-attack hit. It never kills.
func (f *Folk) TakeDamage(damage int, attacker attackable.Combatant) bool {
	f.reduceHP(float64(damage), attacker)
	return false
}

// ReduceHP applies skill damage.
func (f *Folk) ReduceHP(amount float64, attacker attackable.Combatant, _ modelskill.Definition) {
	f.reduceHP(amount, attacker)
}

// ReduceHPByDOT applies periodic damage.
func (f *Folk) ReduceHPByDOT(amount float64, attacker effect.Actor, _ bool) {
	killer, _ := attacker.(attackable.Combatant)
	f.reduceHP(amount, killer)
}

// reduceHP is an NPC's HP reduction: a hit from a creature first puts a
// walking NPC in run stance; an invulnerable NPC, or a hit from an attacker
// without damage permission, then takes nothing. The HP left is never under
// folkMinHP. A hit has no sleep, hold or stun to break: none lands on a
// civilian NPC.
func (f *Folk) reduceHP(amount float64, attacker attackable.Combatant) {
	if attacker != nil {
		f.forceRunStance()
	}
	if f.Invul() || !creature.CanDealDamage(attacker) {
		return
	}
	// A NaN amount takes nothing, like a negative one.
	if !(amount > 0) {
		return
	}
	f.vitalsMu.Lock()
	f.hp = math.Max(f.hp-amount, folkMinHP)
	f.vitalsMu.Unlock()
	f.BroadcastStatus()
}

// SetHP sets the current HP, clamped to [0, max HP], and offers the
// targeters' health bar a refresh.
func (f *Folk) SetHP(value float64) {
	value = min(max(value, 0), f.MaxHPValue())
	f.vitalsMu.Lock()
	f.hp = value
	f.vitalsMu.Unlock()
	f.BroadcastStatus()
}

// Kill reports false: nothing kills a civilian NPC.
func (f *Folk) Kill(attackable.Combatant) bool { return false }

// AddHP restores HP, clamped to max HP, and returns the amount applied;
// anything applied refreshes the targeters' health bar.
func (f *Folk) AddHP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxHP := f.MaxHPValue()
	f.vitalsMu.Lock()
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
// targeters when anything changed.
func (f *Folk) TickRegen() {
	maxHP, maxMP := f.MaxHPValue(), f.MaxMPValue()
	f.vitalsMu.Lock()
	needHP, needMP := f.hp < maxHP, f.mp < maxMP
	f.vitalsMu.Unlock()
	if !needHP && !needMP {
		return
	}
	hpRegen, mpRegen := math.Max(1, f.HPRegenRate()), math.Max(1, f.MPRegenRate())
	f.vitalsMu.Lock()
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
	}
}

// Invul reports whether the NPC is invulnerable.
func (f *Folk) Invul() bool { return f.invul.Load() }

// Invulnerable reports whether the NPC ignores direct resource effects.
func (f *Folk) Invulnerable() bool { return f.Invul() }

// SetInvul sets invulnerability and reports whether it changed.
func (f *Folk) SetInvul(v bool) bool { return f.invul.Swap(v) != v }

// CanBeHealed reports whether the NPC may be healed.
func (f *Folk) CanBeHealed() bool { return !f.Invul() }

// HealEffectiveness returns the percentage incoming heals are scaled by.
func (f *Folk) HealEffectiveness() float64 { return f.CalcStat(stat.HealEffectiveness, 100) }

// HealProficiency returns the flat heal-power bonus.
func (f *Folk) HealProficiency() float64 { return f.CalcStat(stat.HealProficiency, 0) }

// RechargeMP applies the MP recharge multiplier to amount.
func (f *Folk) RechargeMP(amount float64) float64 { return f.CalcStat(stat.RechargeMPRate, amount) }

// HealInput resolves the NPC's side of an outgoing heal.
func (f *Folk) HealInput(def modelskill.Definition) (formulas.HealInput, bool) {
	return creature.ResolveHealInput(def, f.HealProficiency(), f.MAtk(), formulas.HealShotScalingNPC), true
}
