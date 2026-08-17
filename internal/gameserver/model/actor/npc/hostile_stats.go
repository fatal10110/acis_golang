package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/funcs"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// maxBuffCount is the non-toggle, non-seven-signs buff-slot count every
// NPC allows. No passive skill raises this bound for a live NPC actor, so
// it is also the permanent cap (compare player.Character's baseBuffSlots).
const maxBuffCount = 20

// AddStatFuncs attaches fns to h's live stat calculators.
func (h *Hostile) AddStatFuncs(fns []effect.Mod) {
	for _, fn := range fns {
		h.statCalcLocked(fn.Stat).AddMod(fn)
	}
}

// RemoveStatsByOwner drops every stat func previously added for owner.
func (h *Hostile) RemoveStatsByOwner(owner effect.ModOwner) {
	if owner == (effect.ModOwner{}) {
		return
	}
	h.statMu.RLock()
	calcs := h.statCalcs
	h.statMu.RUnlock()
	for _, calc := range calcs {
		if calc != nil {
			calc.RemoveOwner(owner)
		}
	}
}

// MaxBuffCount is the number of non-toggle, non-seven-signs buffs h can
// hold at once. See maxBuffCount.
func (h *Hostile) MaxBuffCount() int {
	return maxBuffCount
}

// statCalc returns s's live Calculator, creating it (with its builtin
// finalize step) on first touch. The common warm case only takes statMu's
// read lock; the slot is created at most once per Stat per Hostile.
func (h *Hostile) statCalc(s stat.Stat) *effect.Calculator {
	h.statMu.RLock()
	if calc := h.statCalcs[s]; calc != nil {
		h.statMu.RUnlock()
		return calc
	}
	h.statMu.RUnlock()
	return h.statCalcLocked(s)
}

func (h *Hostile) statCalcLocked(s stat.Stat) *effect.Calculator {
	h.statMu.Lock()
	defer h.statMu.Unlock()
	if calc := h.statCalcs[s]; calc != nil {
		return calc
	}
	calc := effect.NewCalculator(defaultBuiltin(s))
	h.statCalcs[s] = &calc
	return &calc
}

// calcStat runs s's finalization chain (the base funcs every NPC attaches
// plus any buff/debuff funcs an active effect has added) starting from
// base, clamping to zero for a stat that can't go negative.
func (h *Hostile) calcStat(s stat.Stat, base float64) float64 {
	value := h.statCalc(s).Calc(hostileStatActor{h: h}, base)
	if s.CantBeNegative() && value < 0 {
		return 0
	}
	return value
}

// CalcStat finalizes base for s through h's live stat calculator.
func (h *Hostile) CalcStat(s stat.Stat, base float64) float64 {
	return h.calcStat(s, base)
}

// defaultBuiltin returns the static, attribute-driven finalize step every
// NPC's calculation chain for s runs at order 10, or nil for a Stat with no
// builtin. Unlike a player, an NPC gets no henna or CP funcs — the
// reference AI only ever adds the shared creature set to a monster.
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

// hostileStatActor adapts a Hostile's template attributes to the surface
// the shared attack/defense/regen/speed funcs read from their effector.
type hostileStatActor struct{ h *Hostile }

var _ stat.Actor = hostileStatActor{}

func (a hostileStatActor) STR() int { return a.h.Instance.Template.STR }
func (a hostileStatActor) CON() int { return a.h.Instance.Template.CON }
func (a hostileStatActor) DEX() int { return a.h.Instance.Template.DEX }
func (a hostileStatActor) INT() int { return a.h.Instance.Template.INT }
func (a hostileStatActor) WIT() int { return a.h.Instance.Template.WIT }
func (a hostileStatActor) MEN() int { return a.h.Instance.Template.MEN }

func (a hostileStatActor) Level() int {
	if a.h.Instance.Template.Level <= 0 {
		return 1
	}
	return a.h.Instance.Template.Level
}

// LevelMod is the level-scaling factor every finalize func multiplies in;
// (89+level)/100 matches the shared creature stat pipeline.
func (a hostileStatActor) LevelMod() float64 {
	return (89 + float64(a.Level())) / 100
}

func (a hostileStatActor) IsSummon() bool { return false }

// STR returns this NPC's current STR attribute.
func (h *Hostile) STR() int { return hostileStatActor{h: h}.STR() }

// CON returns this NPC's current CON attribute.
func (h *Hostile) CON() int { return hostileStatActor{h: h}.CON() }

// DEX returns this NPC's current DEX attribute.
func (h *Hostile) DEX() int { return hostileStatActor{h: h}.DEX() }

// INT returns this NPC's current INT attribute.
func (h *Hostile) INT() int { return hostileStatActor{h: h}.INT() }

// WIT returns this NPC's current WIT attribute.
func (h *Hostile) WIT() int { return hostileStatActor{h: h}.WIT() }

// MEN returns this NPC's current MEN attribute.
func (h *Hostile) MEN() int { return hostileStatActor{h: h}.MEN() }

// LevelMod returns this NPC's level-scaling factor.
func (h *Hostile) LevelMod() float64 { return hostileStatActor{h: h}.LevelMod() }
