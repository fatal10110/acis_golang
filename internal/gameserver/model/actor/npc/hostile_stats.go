package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/funcs"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// maxBuffCount is the non-toggle, non-seven-signs buff-slot count every
// NPC allows. No passive skill raises this bound for a live NPC actor, so
// it is also the permanent cap (compare player.Character's baseBuffSlots).
const maxBuffCount = 20

// AddStatFuncs attaches fns to h's live stat calculators.
func (h *Hostile) AddStatFuncs(fns []basefunc.Func) {
	if len(fns) == 0 {
		return
	}
	h.statMu.Lock()
	defer h.statMu.Unlock()
	for _, fn := range fns {
		if fn == nil {
			continue
		}
		h.statCalcLocked(fn.Stat()).AddFunc(fn)
	}
}

// RemoveStatsByOwner drops every stat func previously added for owner.
func (h *Hostile) RemoveStatsByOwner(owner any) {
	if owner == nil {
		return
	}
	h.statMu.Lock()
	defer h.statMu.Unlock()
	for _, calc := range h.statCalcs {
		calc.RemoveOwner(owner)
	}
}

// MaxBuffCount is the number of non-toggle, non-seven-signs buffs h can
// hold at once. See maxBuffCount.
func (h *Hostile) MaxBuffCount() int {
	return maxBuffCount
}

func (h *Hostile) statCalc(s stat.Stat) *basefunc.Calculator {
	h.statMu.Lock()
	defer h.statMu.Unlock()
	return h.statCalcLocked(s)
}

func (h *Hostile) statCalcLocked(s stat.Stat) *basefunc.Calculator {
	if h.statCalcs == nil {
		h.statCalcs = make(map[stat.Stat]*basefunc.Calculator)
	}
	if calc := h.statCalcs[s]; calc != nil {
		return calc
	}
	calc := &basefunc.Calculator{}
	for _, fn := range defaultStatFuncs(s) {
		calc.AddFunc(fn)
	}
	h.statCalcs[s] = calc
	return calc
}

// calcStat runs s's finalization chain (the base funcs every NPC attaches
// plus any buff/debuff funcs an active effect has added) starting from
// base, clamping to zero for a stat that can't go negative.
func (h *Hostile) calcStat(s stat.Stat, base float64) float64 {
	value := h.statCalc(s).Calc(hostileStatActor{h: h}, h, nil, base)
	if s.CantBeNegative() && value < 0 {
		return 0
	}
	return value
}

// defaultStatFuncs returns the base stat-finalization funcs every NPC
// attaches for s. Unlike a player, an NPC gets no henna or CP funcs — the
// reference AI only ever adds the shared creature set to a monster.
func defaultStatFuncs(s stat.Stat) []basefunc.Func {
	switch s {
	case stat.MaxHP:
		return []basefunc.Func{funcs.MaxHpMul}
	case stat.MaxMP:
		return []basefunc.Func{funcs.MaxMpMul}
	case stat.RegenerateHPRate:
		return []basefunc.Func{funcs.RegenHpMul}
	case stat.RegenerateMPRate:
		return []basefunc.Func{funcs.RegenMpMul}
	case stat.PowerAttack:
		return []basefunc.Func{funcs.PAtkMod}
	case stat.PowerDefence:
		return []basefunc.Func{funcs.PDefMod}
	case stat.MagicAttack:
		return []basefunc.Func{funcs.MAtkMod}
	case stat.MagicDefence:
		return []basefunc.Func{funcs.MDefMod}
	case stat.PowerAttackSpeed:
		return []basefunc.Func{funcs.PAtkSpeed}
	case stat.MagicAttackSpeed:
		return []basefunc.Func{funcs.MAtkSpeed}
	case stat.AccuracyCombat:
		return []basefunc.Func{funcs.AtkAccuracy}
	case stat.EvasionRate:
		return []basefunc.Func{funcs.AtkEvasion}
	case stat.CriticalRate:
		return []basefunc.Func{funcs.AtkCritical}
	case stat.MCriticalRate:
		return []basefunc.Func{funcs.MAtkCritical}
	case stat.RunSpeed:
		return []basefunc.Func{funcs.MoveSpeed}
	default:
		return nil
	}
}

// hostileStatActor adapts a Hostile's template attributes to the surface
// the shared attack/defense/regen/speed funcs read from their effector.
type hostileStatActor struct{ h *Hostile }

var _ funcs.Actor = hostileStatActor{}

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
