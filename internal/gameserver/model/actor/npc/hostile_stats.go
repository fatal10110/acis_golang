package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/funcs"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// maxBuffCount is the shipped players.properties MaxBuffsAmount default.
const maxBuffCount = 20

// AddStatFuncs attaches fns to h's live stat calculators and reports the
// change. Each Mod is published independently under its own Calculator's
// lock — the batch is not atomic against a concurrent CalcStat, which may
// observe fns partially applied. Callers that need a batch to appear
// all-or-nothing to readers must serialize at a higher level (see
// effect.List, which does this for effect-driven adds through
// AttachStatFuncs and StatFuncsAttached).
//
// A func whose attached condition fails for h is left out.
func (h *Hostile) AddStatFuncs(fns []effect.Mod) {
	fns = effect.Attachable(fns, h.ConditionActor())
	h.AttachStatFuncs(fns)
	h.StatFuncsAttached(fns)
}

// attachPassives gives h the template passives that attach to it and
// reports the change. It runs at construction, before any stat chain is
// changed, so every slot created afterwards starts from them.
func (h *Hostile) attachPassives(passives *compiledPassives) {
	fns, byStat := passives.attached(h.ConditionActor())
	h.statMu.Lock()
	h.passives = byStat
	h.statMu.Unlock()
	h.StatFuncsAttached(fns)
}

// AttachStatFuncs attaches fns to h's live stat calculators without
// reporting the change.
func (h *Hostile) AttachStatFuncs(fns []effect.Mod) {
	for _, fn := range fns {
		h.statCalcOrCreate(fn.Stat).AddMod(fn)
	}
}

// StatFuncsAttached reports the stat change of attached fns.
func (h *Hostile) StatFuncsAttached(fns []effect.Mod) {
	h.broadcastModifiedStats(fns)
}

// RemoveStatsByOwner drops every stat func previously added for owner. An
// effect a stop-all is ending changes the movement speed only; its
// observers are sent nothing.
func (h *Hostile) RemoveStatsByOwner(owner effect.ModOwner) {
	if owner == (effect.ModOwner{}) {
		return
	}
	var modified []stat.Stat
	for s, calc := range h.ownerCalcs(owner) {
		if calc != nil && calc.RemoveOwner(owner) > 0 {
			modified = append(modified, stat.Stat(s))
		}
	}
	if owner.Stripped() {
		if len(modified) > 0 {
			h.refreshMoveSpeed()
		}
		return
	}
	h.broadcastModifiedStatsFor(modified)
}

func (h *Hostile) broadcastModifiedStats(fns []effect.Mod) {
	stats := make([]stat.Stat, len(fns))
	for i, fn := range fns {
		stats[i] = fn.Stat
	}
	h.broadcastModifiedStatsFor(stats)
}

func (h *Hostile) broadcastModifiedStatsFor(stats []stat.Stat) {
	// Every position update reads the live move speed; hand the movement
	// simulation the new one after any stat func change.
	if len(stats) > 0 {
		h.refreshMoveSpeed()
	}
	if h.sink == nil {
		return
	}
	full := false
	attrs := make([]event.StatusAttr, 0, len(stats))
	for _, s := range stats {
		switch s {
		case stat.PowerAttackSpeed:
			attrs = append(attrs, event.StatusAttr{Kind: event.StatusPhysicalSpeed, Value: h.AttackSpeed()})
		case stat.MagicAttackSpeed:
			attrs = append(attrs, event.StatusAttr{Kind: event.StatusMagicSpeed, Value: h.MagicAttackSpeed()})
		case stat.MaxHP:
			attrs = append(attrs, event.StatusAttr{Kind: event.StatusMaxHP, Value: int(h.MaxHPValue())})
		case stat.RunSpeed:
			full = true
		}
	}
	if full {
		h.emit(event.NPCInfoChanged{ServerObject: h.RunSpeed() == 0})
		return
	}
	if len(attrs) == 0 {
		return
	}
	h.emit(event.Status{Attrs: attrs})
}

// SetMaxBuffsAmount records the players.properties base buff-slot count.
func (h *Hostile) SetMaxBuffsAmount(amount int) {
	if h != nil {
		h.maxBuffsAmount.Store(int32(amount))
	}
}

// MaxBuffCount is the number of non-toggle, non-seven-signs buffs h can
// hold at once: the configured base plus its template's Divine Inspiration.
func (h *Hostile) MaxBuffCount() int {
	if h == nil {
		return maxBuffCount
	}
	return int(h.maxBuffsAmount.Load()) + h.Instance.Template.Skills[int(modelskill.DivineInspirationSkillID)]
}

// ownerCalcs returns h's stat slots, with one created for every Stat a
// template passive of owner modifies so that its Mods can be dropped.
func (h *Hostile) ownerCalcs(owner effect.ModOwner) [stat.Count]*effect.Calculator {
	h.statMu.RLock()
	passives := h.passives
	h.statMu.RUnlock()
	for s := range passives.StatsOwnedBy(owner) {
		h.statCalcOrCreate(s)
	}
	h.statMu.RLock()
	defer h.statMu.RUnlock()
	if h.statCalcs == nil {
		return [stat.Count]*effect.Calculator{}
	}
	return *h.statCalcs
}

// statCalcOrCreate returns s's Calculator, creating it from s's template
// passives and builtin finalize step on first use; the slot is created at
// most once per Stat per Hostile.
func (h *Hostile) statCalcOrCreate(s stat.Stat) *effect.Calculator {
	h.statMu.Lock()
	defer h.statMu.Unlock()
	if h.statCalcs == nil {
		h.statCalcs = new([stat.Count]*effect.Calculator)
	}
	if calc := h.statCalcs[s]; calc != nil {
		return calc
	}
	calc := h.passives.NewCalculator(s, defaultBuiltin(s))
	h.statCalcs[s] = &calc
	return &calc
}

// calcStat runs s's finalization chain (the base funcs every NPC attaches
// plus any buff/debuff funcs an active effect has added) starting from
// base, flooring a non-positive stat that can't go negative at one.
func (h *Hostile) calcStat(s stat.Stat, base float64) float64 {
	return h.calcFor(s, base, nil, nil)
}

// calcFor is calcStat for a calculation against target run for skill (nil
// for none).
func (h *Hostile) calcFor(s stat.Stat, base float64, target conditions.Actor, skill conditions.Skill) float64 {
	h.statMu.RLock()
	var calc *effect.Calculator
	if h.statCalcs != nil {
		calc = h.statCalcs[s]
	}
	passives := h.passives
	h.statMu.RUnlock()
	actor := hostileStatActor{h: h}
	var value float64
	if calc != nil {
		value = calc.Calc(actor, base, target, skill)
	} else {
		value = passives.Calc(s, defaultBuiltin(s), actor, base, target, skill)
	}
	if s.CantBeNegative() && value <= 0 {
		return 1
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
// a monster only ever gets the shared creature set.
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
