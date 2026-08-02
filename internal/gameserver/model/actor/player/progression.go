package player

import "math"

// maxSP is the largest SP value a character can hold, matching the 32-bit
// signed integer ceiling the persisted column was sized for.
const maxSP = math.MaxInt32

// AddExpAndSp adds exp and sp to c independently — either amount is
// ignored if negative — resyncing c.CharLevel from the resulting experience
// via table and, on a level increase, refilling HP, MP and CP to the full
// amount tmpl's per-level tables define for the new level. tmpl may be
// nil, in which case a level increase still updates c.CharLevel and c.Exp but
// leaves HP/MP/CP untouched. It reports whether the level increased.
func (c *Character) AddExpAndSp(table *LevelTable, tmpl *Template, exp int64, sp int) bool {
	beforeExp, beforeSP := c.Exp, c.SP
	leveledUp := false
	// The reward message follows the attempt, not the result: an experience
	// add always counts, and an SP add counts unless SP already sits at the
	// ceiling. Only an attempt where neither amount applied stays silent.
	attempted := false
	if exp >= 0 {
		leveledUp = c.AddExp(table, tmpl, exp)
		attempted = true
	}
	if sp >= 0 {
		attempted = attempted || c.SP < maxSP
		c.AddSp(sp)
	}
	// Only an add that actually landed pushes UserInfo. Deliberate divergence
	// under review in issue #1060: the reference sends it for any non-negative
	// add, because its dropped-addition branch still reports success to the
	// caller that sends the packet, so a zero-value reward (a full
	// level-difference penalty) pushes a UserInfo describing nothing that
	// changed. Both adds here can be no-ops, and the packet is self-only and
	// purely descriptive, so the redundant one is suppressed.
	if c.Exp != beforeExp || c.SP != beforeSP {
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
	if table == nil {
		beforeSP := c.SP
		if sp >= 0 {
			c.AddSp(sp)
			// Same deliberate divergence as AddExpAndSp: no packet when the
			// add changed nothing.
			if c.SP != beforeSP {
				c.UpdateUserInfo()
			}
			c.sendExpSpGain(0, sp)
		}
		return false
	}
	return c.AddExpAndSp(table, c.runtimeTemplate, exp, sp)
}

// AddExp adds delta experience to c. An addition that would overflow
// c.Exp negative is silently dropped, and an addition that would reach the
// top of the highest level's experience band is clamped just below it. It
// resyncs c.CharLevel from the new experience via table, applying the same
// HP/MP/CP refill as AddLevel on an increase. It reports whether the level
// increased.
func (c *Character) AddExp(table *LevelTable, tmpl *Template, delta int64) bool {
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
	return c.AddLevel(table, tmpl, level-c.CharLevel)
}

// AddSp adds delta sp to c.SP, saturating at the 32-bit signed integer
// maximum the persisted column was sized for. A negative delta is a no-op.
func (c *Character) AddSp(delta int) {
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
	beforeLevel := c.CharLevel
	if exp > 0 {
		c.RemoveExp(table, tmpl, exp)
	}
	if sp > 0 {
		c.RemoveSp(sp)
	}
	if exp <= 0 && sp <= 0 {
		return
	}
	c.sendExpSpLoss(exp, sp)
	// A removal deep enough to drop a level changes max HP and MP, so the
	// observers' health bars need the new values, not only this client.
	if c.CharLevel < beforeLevel {
		c.BroadcastStatus()
	}
}

// RemoveExp subtracts delta experience from c, flooring at 1 experience
// (never 0) rather than going negative, and resyncs c.CharLevel from the
// result via table.
func (c *Character) RemoveExp(table *LevelTable, tmpl *Template, delta int64) {
	if c.Exp-delta < 0 {
		delta = c.Exp - 1
	}
	c.Exp -= delta

	if level := table.levelForExp(c.Exp); level != c.CharLevel {
		c.AddLevel(table, tmpl, level-c.CharLevel)
	}
}

// RemoveSp subtracts delta sp from c.SP, flooring at 0.
func (c *Character) RemoveSp(delta int) {
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
			c.refillResources(tmpl.HPTable[idx], tmpl.MPTable[idx], tmpl.CPTable[idx])
		}
		c.announceLevelUp()
	}

	c.refreshForLevel()
	// PlayerStatus.addLevel calls _actor.refreshWeightPenalty() directly
	// on every level change (PlayerStatus.java:644), before the UserInfo
	// send below (:648) — the weight limit is CON-derived and therefore
	// level-dependent.
	c.RefreshWeightPenalty()
	c.UpdateUserInfo()
	return increased
}

// SetExpSpGainNotifier records the packet-layer hook that tells this
// character's own client how much experience and SP an addition granted. It
// fires once per addition attempt that was not fully rejected, including one
// granting zero of both.
func (c *Character) SetExpSpGainNotifier(notify func(exp int64, sp int)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.notifyExpSpGain = notify
}

// SetExpSpLossNotifier records the packet-layer hook that tells this
// character's own client how much experience and SP a removal took.
func (c *Character) SetExpSpLossNotifier(notify func(exp int64, sp int)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.notifyExpSpLoss = notify
}

// SetLevelRefresher records the hook that re-derives everything a
// character's level entitles it to — the skills the new level grants or
// revokes, and the client's view of them — after any level change, up or
// down. It runs before the level change's UserInfo, so the packet describes
// the already-refreshed character.
func (c *Character) SetLevelRefresher(refresh func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.refreshLevel = refresh
}

// SetLevelUpBroadcaster records the packet-layer hook that plays this
// character's level-up animation for every observer and tells its own client
// the level went up.
func (c *Character) SetLevelUpBroadcaster(broadcast func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastLevelUp = broadcast
}

func (c *Character) sendExpSpGain(exp int64, sp int) {
	c.stateMu.RLock()
	notify := c.notifyExpSpGain
	c.stateMu.RUnlock()
	if notify != nil {
		notify(exp, sp)
	}
}

func (c *Character) sendExpSpLoss(exp int64, sp int) {
	c.stateMu.RLock()
	notify := c.notifyExpSpLoss
	c.stateMu.RUnlock()
	if notify != nil {
		notify(exp, sp)
	}
}

func (c *Character) refreshForLevel() {
	c.stateMu.RLock()
	refresh := c.refreshLevel
	c.stateMu.RUnlock()
	if refresh != nil {
		refresh()
	}
}

func (c *Character) announceLevelUp() {
	c.stateMu.RLock()
	broadcast := c.broadcastLevelUp
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast()
	}
}
