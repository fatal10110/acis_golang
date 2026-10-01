package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

var _ effect.StatOwner = (*Folk)(nil)

// statCalc returns s's live Calculator, creating it on first touch.
func (f *Folk) statCalc(s stat.Stat) *effect.Calculator {
	f.statMu.RLock()
	if calc := f.statCalcs[s]; calc != nil {
		f.statMu.RUnlock()
		return calc
	}
	f.statMu.RUnlock()
	f.statMu.Lock()
	defer f.statMu.Unlock()
	if calc := f.statCalcs[s]; calc != nil {
		return calc
	}
	calc := effect.NewCalculator(defaultBuiltin(s))
	f.statCalcs[s] = &calc
	return &calc
}

// CalcStat finalizes base for s through the NPC's builtin funcs, template
// passives and the funcs of the effects it holds, flooring a non-positive
// stat that can't go negative at one.
func (f *Folk) CalcStat(s stat.Stat, base float64) float64 {
	value := f.statCalc(s).Calc(templateStatActor{t: f.Instance.Template}, base)
	if s.CantBeNegative() && value <= 0 {
		return 1
	}
	return value
}

// AttachStatFuncs attaches fns to the NPC's stat calculators without
// reporting the change.
func (f *Folk) AttachStatFuncs(fns []effect.Mod) {
	for _, fn := range fns {
		f.statCalc(fn.Stat).AddMod(fn)
	}
}

// StatFuncsAttached shows observers the stats fns changed.
func (f *Folk) StatFuncsAttached(fns []effect.Mod) {
	stats := make([]stat.Stat, len(fns))
	for i, fn := range fns {
		stats[i] = fn.Stat
	}
	f.broadcastModifiedStats(stats)
}

// RemoveStatsByOwner drops every stat func owner added. A stripped owner
// (an effect a stop-all ends) changes the movement speed only and shows
// observers nothing.
func (f *Folk) RemoveStatsByOwner(owner effect.ModOwner) {
	if owner == (effect.ModOwner{}) {
		return
	}
	f.statMu.RLock()
	calcs := f.statCalcs
	f.statMu.RUnlock()
	var modified []stat.Stat
	for s, calc := range calcs {
		if calc != nil && calc.RemoveOwner(owner) > 0 {
			modified = append(modified, stat.Stat(s))
		}
	}
	if owner.Stripped() {
		if len(modified) > 0 {
			f.refreshMoveSpeed()
		}
		return
	}
	f.broadcastModifiedStats(modified)
}

// broadcastModifiedStats shows observers the changed stats of a non-
// attackable NPC: its attack and casting speeds in a status update, or its
// whole info when its run speed changed. Its max HP is not shown.
func (f *Folk) broadcastModifiedStats(stats []stat.Stat) {
	if len(stats) == 0 {
		return
	}
	f.refreshMoveSpeed()
	var attrs []event.StatusAttr
	for _, s := range stats {
		switch s {
		case stat.PowerAttackSpeed:
			attrs = append(attrs, event.StatusAttr{Kind: event.StatusPhysicalSpeed, Value: f.AttackSpeed()})
		case stat.MagicAttackSpeed:
			attrs = append(attrs, event.StatusAttr{Kind: event.StatusMagicSpeed, Value: f.MagicAttackSpeed()})
		case stat.RunSpeed:
			f.emit(event.NPCInfoChanged{ServerObject: f.MoveSpeed() == 0})
			return
		}
	}
	if len(attrs) > 0 {
		f.emit(event.Status{Attrs: attrs})
	}
}

// MaxBuffCount is the configured buff-slot count plus the template's
// Divine Inspiration.
func (f *Folk) MaxBuffCount() int {
	return int(f.maxBuffs.Load()) + f.Instance.Template.Skills[int(modelskill.DivineInspirationSkillID)]
}

// UpdateEffectIcons does nothing: NPCs show no effect icons.
func (f *Folk) UpdateEffectIcons() {}

// NotifyEffectWornOff does nothing: effect messages go to players.
func (f *Folk) NotifyEffectWornOff(modelskill.ID, int) {}

// NotifyEffectDisappeared does nothing: effect messages go to players.
func (f *Folk) NotifyEffectDisappeared(modelskill.ID, int) {}

// NotifyEffectAborted does nothing: effect messages go to players.
func (f *Folk) NotifyEffectAborted(modelskill.ID, int) {}

// NotifyEffectFelt does nothing: effect messages go to players.
func (f *Folk) NotifyEffectFelt(modelskill.ID, int) {}

// STR returns the template STR.
func (f *Folk) STR() int { return f.Instance.Template.STR }

// CON returns the template CON.
func (f *Folk) CON() int { return f.Instance.Template.CON }

// DEX returns the template DEX.
func (f *Folk) DEX() int { return f.Instance.Template.DEX }

// INT returns the template INT.
func (f *Folk) INT() int { return f.Instance.Template.INT }

// WIT returns the template WIT.
func (f *Folk) WIT() int { return f.Instance.Template.WIT }

// MEN returns the template MEN.
func (f *Folk) MEN() int { return f.Instance.Template.MEN }

// PAtk returns the physical attack, truncated to a whole number.
func (f *Folk) PAtk() float64 {
	return math.Trunc(f.CalcStat(stat.PowerAttack, f.Instance.Template.PAtk))
}

// PDef returns the physical defence, truncated to a whole number.
func (f *Folk) PDef() float64 {
	return math.Trunc(f.CalcStat(stat.PowerDefence, f.Instance.Template.PDef))
}

// MAtk returns the magic attack, truncated to a whole number.
func (f *Folk) MAtk() float64 {
	return math.Trunc(f.CalcStat(stat.MagicAttack, positiveStat(f.Instance.Template.MAtk)))
}

// MDef returns the magic defence, truncated to a whole number.
func (f *Folk) MDef() float64 {
	return math.Trunc(f.CalcStat(stat.MagicDefence, positiveStat(f.Instance.Template.MDef)))
}

// MagicCriticalRate returns the magic critical rate.
func (f *Folk) MagicCriticalRate() float64 { return f.CalcStat(stat.MCriticalRate, 8) }

// Evasion returns the physical evasion.
func (f *Folk) Evasion() int { return int(f.CalcStat(stat.EvasionRate, 0)) }

// LethalRate returns the lethal-strike rate multiplier.
func (f *Folk) LethalRate() float64 { return f.CalcStat(stat.LethalRate, 1) }

// AttackType reports a fist: a civilian NPC never attacks, so its weapon
// takes part in no formula.
func (f *Folk) AttackType() item.WeaponType { return item.WeaponFist }

// SoulshotCharged reports false: a civilian NPC never attacks.
func (f *Folk) SoulshotCharged() bool { return false }

// SpiritshotCharged reports false: a civilian NPC never casts.
func (f *Folk) SpiritshotCharged() bool { return false }

// BlessedSpiritshotCharged reports false: a civilian NPC never casts.
func (f *Folk) BlessedSpiritshotCharged() bool { return false }

// WeaponGradePenalty reports false: NPCs carry no weapon grade.
func (f *Folk) WeaponGradePenalty() bool { return false }

// RandomDamageSpread returns the template random-damage spread, or -1 when
// none is set.
func (f *Folk) RandomDamageSpread() int {
	if f.Instance.Template.BaseRandomDamage <= 0 {
		return -1
	}
	return f.Instance.Template.BaseRandomDamage
}

// AttackSpeed returns the physical attack speed.
func (f *Folk) AttackSpeed() int {
	return int(f.CalcStat(stat.PowerAttackSpeed, f.Instance.Template.AtkSpd))
}

// MagicAttackSpeed returns the casting speed.
func (f *Folk) MagicAttackSpeed() int {
	return int(f.CalcStat(stat.MagicAttackSpeed, magicAttackSpeedBase))
}

// MaxHPValue returns the maximum HP in whole points.
func (f *Folk) MaxHPValue() float64 {
	return math.Trunc(f.CalcStat(stat.MaxHP, f.Instance.Template.HPMax))
}

// MaxHP returns the maximum HP.
func (f *Folk) MaxHP() int { return int(f.MaxHPValue()) }

// MaxMPValue returns the maximum MP in whole points.
func (f *Folk) MaxMPValue() float64 {
	return math.Trunc(f.CalcStat(stat.MaxMP, f.Instance.Template.MPMax))
}

// HPRegenRate returns the HP regenerated per tick.
func (f *Folk) HPRegenRate() float64 {
	return f.CalcStat(stat.RegenerateHPRate, f.Instance.Template.HPRegen)
}

// MPRegenRate returns the MP regenerated per tick.
func (f *Folk) MPRegenRate() float64 {
	return f.CalcStat(stat.RegenerateMPRate, f.Instance.Template.MPRegen)
}

// MoveSpeed is the current move speed: the template run or walk speed the
// stance picks, through the run-speed stat, narrowed to float32.
func (f *Folk) MoveSpeed() float64 {
	speed, _ := f.moveSpeedAndBase()
	return speed
}

// MovementSpeedMultiplier is the move speed over the stance's base speed,
// or 0 when that base is 0.
func (f *Folk) MovementSpeedMultiplier() float32 {
	speed, base := f.moveSpeedAndBase()
	if base == 0 {
		return 0
	}
	return float32(speed) / float32(base)
}

func (f *Folk) moveSpeedAndBase() (float64, int) {
	base := int(f.Instance.Template.WalkSpeed)
	if f.Running() {
		base = int(f.Instance.Template.RunSpeed)
	}
	return float64(float32(f.CalcStat(stat.RunSpeed, float64(base)))), base
}

// refreshMoveSpeed hands a walking NPC's movement its current speed.
func (f *Folk) refreshMoveSpeed() {
	if f.motion == nil {
		return
	}
	f.speedMu.Lock()
	defer f.speedMu.Unlock()
	f.motion.move.SetSpeed(f.MoveSpeed())
}
