package player

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// maxSP is the largest SP value a character can hold, matching the 32-bit
// signed integer ceiling the persisted column was sized for.
const maxSP = math.MaxInt32

// Progression snapshots the persisted level, experience, SP and karma
// counters.
type Progression struct {
	CharLevel      int
	Exp            int64
	SP             int
	ExpBeforeDeath int64
	Karma          int
	PvPKills       int
	PKKills        int
}

// progressionHooks collects what a progression change triggers — events and
// level-derived refreshes that read progression back — so it runs in order
// after progressionMu is released.
type progressionHooks []func()

func (h *progressionHooks) add(fn func()) { *h = append(*h, fn) }

func (h progressionHooks) run() {
	for _, fn := range h {
		fn()
	}
}

// ProgressionValues returns a synchronized snapshot of c's persisted
// progression fields. CharacterStore.Save reads through this so a
// disconnect/autosave goroutine never races reward application on the live
// character (#1890).
func (c *Character) ProgressionValues() Progression {
	c.progressionMu.RLock()
	defer c.progressionMu.RUnlock()
	return Progression{
		CharLevel:      c.CharLevel,
		Exp:            c.Exp,
		SP:             c.SP,
		ExpBeforeDeath: c.ExpBeforeDeath,
		Karma:          c.KarmaPoints,
		PvPKills:       c.PvPKills,
		PKKills:        c.PKKills,
	}
}

// AddExpAndSp adds exp and sp to c independently — either amount is
// ignored if negative — resyncing c.CharLevel from the resulting experience
// via table and, on a level increase, refilling HP, MP and CP to the full
// amount tmpl's per-level tables define for the new level. tmpl may be
// nil, in which case a level increase still updates c.CharLevel and c.Exp but
// leaves HP/MP/CP untouched. It reports whether the level increased.
func (c *Character) AddExpAndSp(table *LevelTable, tmpl *Template, exp int64, sp int) bool {
	return c.addExpAndSp(table, tmpl, exp, sp)
}

// addExpAndSp applies the experience, runs what a level change triggers,
// then applies the SP: a level change's UserInfo carries the SP from before
// this add, as it does when experience and SP land one after the other.
func (c *Character) addExpAndSp(table *LevelTable, tmpl *Template, exp int64, sp int) bool {
	var hooks progressionHooks
	c.progressionMu.Lock()
	beforeExp := c.Exp
	leveledUp := false
	// The reward message follows the attempt, not the result: an experience
	// add always counts, and an SP add counts unless SP already sits at the
	// ceiling. Only an attempt where neither amount applied stays silent.
	attempted := false
	if exp >= 0 {
		leveledUp = c.addExp(table, tmpl, exp, &hooks)
		attempted = true
	}
	changed := c.Exp != beforeExp
	c.progressionMu.Unlock()
	hooks.run()

	c.progressionMu.Lock()
	if sp >= 0 {
		beforeSP := c.SP
		attempted = attempted || c.SP < maxSP
		c.addSp(sp)
		changed = changed || c.SP != beforeSP
	}
	c.progressionMu.Unlock()

	// Only an add that actually landed pushes UserInfo. Deliberate divergence
	// under review in issue #1060: the reference sends it for any non-negative
	// add, because its dropped-addition branch still reports success to the
	// caller that sends the packet, so a zero-value reward (a full
	// level-difference penalty) pushes a UserInfo describing nothing that
	// changed. Both adds here can be no-ops, and the packet is self-only and
	// purely descriptive, so the redundant one is suppressed.
	if changed {
		c.UpdateUserInfo()
	}
	if attempted {
		c.sendExpSpGain(exp, sp)
	}
	return leveledUp
}

// RewardExpAndSp applies a kill reward using this live character's runtime
// template for level-up stat refills.
func (c *Character) RewardExpAndSp(table *LevelTable, exp int64, sp int) bool {
	if table != nil {
		return c.addExpAndSp(table, c.runtimeTemplate, exp, sp)
	}
	if sp < 0 {
		return false
	}
	c.progressionMu.Lock()
	beforeSP := c.SP
	c.addSp(sp)
	changed := c.SP != beforeSP
	c.progressionMu.Unlock()
	// Same deliberate divergence as AddExpAndSp: no packet when the add
	// changed nothing.
	if changed {
		c.UpdateUserInfo()
	}
	c.sendExpSpGain(0, sp)
	return false
}

// AddExp adds delta experience to c. An addition that would overflow
// c.Exp negative is silently dropped, and an addition that would reach the
// top of the highest level's experience band is clamped just below it. It
// resyncs c.CharLevel from the new experience via table, applying the same
// HP/MP/CP refill as AddLevel on an increase. It reports whether the level
// increased.
func (c *Character) AddExp(table *LevelTable, tmpl *Template, delta int64) bool {
	var hooks progressionHooks
	c.progressionMu.Lock()
	leveledUp := c.addExp(table, tmpl, delta, &hooks)
	c.progressionMu.Unlock()
	hooks.run()
	return leveledUp
}

func (c *Character) addExp(table *LevelTable, tmpl *Template, delta int64, hooks *progressionHooks) bool {
	if c.Exp+delta < 0 {
		return false
	}

	capExp := table.RequiredExpForHighestLevel()
	if c.Exp+delta >= capExp {
		delta = capExp - 1 - c.Exp
	}
	c.Exp += delta

	level := table.levelForExp(c.Exp)
	if level == c.CharLevel {
		return false
	}
	return c.addLevel(table, tmpl, level-c.CharLevel, hooks)
}

// AddSp adds delta sp to c.SP, saturating at the 32-bit signed integer
// maximum the persisted column was sized for. A negative delta is a no-op.
func (c *Character) AddSp(delta int) {
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	c.addSp(delta)
}

func (c *Character) addSp(delta int) {
	if delta < 0 || c.SP >= maxSP {
		return
	}
	if delta > maxSP-c.SP {
		delta = maxSP - c.SP
	}
	c.SP += delta
}

// RemoveExpAndSp removes exp and sp from c independently — either amount
// is ignored unless positive — resyncing c.CharLevel the same way AddExpAndSp
// does. A level drop never refills HP/MP/CP, matching AddLevel.
func (c *Character) RemoveExpAndSp(table *LevelTable, tmpl *Template, exp int64, sp int) {
	// The experience comes off first and a level change's hooks run before
	// the SP does, so their UserInfo carries the SP from before this removal.
	var hooks progressionHooks
	c.progressionMu.Lock()
	beforeLevel := c.CharLevel
	if exp > 0 {
		c.removeExp(table, tmpl, exp, &hooks)
	}
	c.progressionMu.Unlock()
	hooks.run()

	hooks = nil
	c.progressionMu.Lock()
	c.finishExpSpLoss(beforeLevel, exp, sp, &hooks)
	c.progressionMu.Unlock()
	hooks.run()
}

// removeExpAndSp is RemoveExpAndSp for a caller already holding
// progressionMu.
func (c *Character) removeExpAndSp(table *LevelTable, tmpl *Template, exp int64, sp int, hooks *progressionHooks) {
	beforeLevel := c.CharLevel
	if exp > 0 {
		c.removeExp(table, tmpl, exp, hooks)
	}
	c.finishExpSpLoss(beforeLevel, exp, sp, hooks)
}

// finishExpSpLoss removes sp and queues the loss notification. The caller
// holds progressionMu.
func (c *Character) finishExpSpLoss(beforeLevel int, exp int64, sp int, hooks *progressionHooks) {
	if sp > 0 {
		c.removeSp(sp)
	}
	if exp <= 0 && sp <= 0 {
		return
	}
	spLeft := c.SP
	hooks.add(func() { c.sendExpSpLoss(exp, sp, spLeft) })
	// A removal deep enough to drop a level changes max HP and MP, so the
	// observers' health bars need the new values, not only this client.
	if c.CharLevel < beforeLevel {
		hooks.add(c.BroadcastStatus)
	}
}

// RemoveExp subtracts delta experience from c, flooring at 1 experience
// (never 0) rather than going negative, and resyncs c.CharLevel from the
// result via table.
func (c *Character) RemoveExp(table *LevelTable, tmpl *Template, delta int64) {
	var hooks progressionHooks
	c.progressionMu.Lock()
	c.removeExp(table, tmpl, delta, &hooks)
	c.progressionMu.Unlock()
	hooks.run()
}

func (c *Character) removeExp(table *LevelTable, tmpl *Template, delta int64, hooks *progressionHooks) {
	if c.Exp-delta < 0 {
		delta = c.Exp - 1
	}
	c.Exp -= delta

	if level := table.levelForExp(c.Exp); level != c.CharLevel {
		c.addLevel(table, tmpl, level-c.CharLevel, hooks)
	}
}

// RemoveSp subtracts delta sp from c.SP, flooring at 0.
func (c *Character) RemoveSp(delta int) {
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	c.removeSp(delta)
}

func (c *Character) removeSp(delta int) {
	c.SP = max(0, c.SP-delta)
}

// AddLevel changes c.CharLevel by delta levels (positive to level up,
// negative to level down), refusing entirely — leaving c untouched — if
// that would put the level above table's real max. It resyncs c.Exp to
// stay inside the resulting level's experience band, and only when the
// level actually increases, refills HP, MP and CP to the full amount
// tmpl's per-level tables define for the new level (skipped if tmpl is nil
// or has no row for it). It reports whether the level increased.
//
// A level change in either direction then runs the level-dependent refresh
// and pushes UserInfo: what a character's level entitles it to is re-derived
// from the new level, never remembered, so a drop has to revoke exactly what
// a gain would have granted.
func (c *Character) AddLevel(table *LevelTable, tmpl *Template, delta int) bool {
	var hooks progressionHooks
	c.progressionMu.Lock()
	increased := c.addLevel(table, tmpl, delta, &hooks)
	c.progressionMu.Unlock()
	hooks.run()
	return increased
}

func (c *Character) addLevel(table *LevelTable, tmpl *Template, delta int, hooks *progressionHooks) bool {
	if c.CharLevel+delta > table.RealMaxLevel() {
		return false
	}

	increased := delta > 0
	c.CharLevel += delta

	lower := table.RequiredExpForLevel(c.CharLevel)
	upper := table.RequiredExpForLevel(c.CharLevel + 1)
	if c.Exp >= upper || lower > c.Exp {
		c.Exp = lower
	}

	if increased {
		if idx := c.CharLevel - 1; tmpl != nil && idx >= 0 && idx < len(tmpl.HPTable) && idx < len(tmpl.MPTable) && idx < len(tmpl.CPTable) {
			hp, mp, cp := tmpl.HPTable[idx], tmpl.MPTable[idx], tmpl.CPTable[idx]
			hooks.add(func() { c.refillResources(hp, mp, cp) })
		}
		hooks.add(c.announceLevelUp)
	}

	hooks.add(c.refreshForLevel)
	// PlayerStatus.addLevel calls _actor.refreshWeightPenalty() directly
	// on every level change (PlayerStatus.java:644), before the UserInfo
	// send below (:648) — the weight limit is CON-derived and therefore
	// level-dependent.
	hooks.add(c.RefreshWeightPenalty)
	hooks.add(c.UpdateUserInfo)
	return increased
}

func (c *Character) sendExpSpGain(exp int64, sp int) {
	c.emit(event.ExpSPGained{Exp: exp, SP: sp})
}

func (c *Character) sendExpSpLoss(exp int64, sp, spLeft int) {
	c.emit(event.ExpSPLost{Exp: exp, SP: sp, SPLeft: spLeft})
}

func (c *Character) refreshForLevel() {
	c.emit(event.LevelChanged{})
}

func (c *Character) announceLevelUp() {
	c.emit(event.LeveledUp{})
}
