package player

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Vitals snapshots the character's current visible resources.
type Vitals struct {
	HP int
	MP int
}

// Resources snapshots the character's maximum and current visible resources.
type Resources struct {
	MaxHP, CurrentHP float64
	MaxMP, CurrentMP float64
	MaxCP, CurrentCP float64
}

// VitalsChange describes which player resources changed and their new values.
type VitalsChange struct {
	HP        int
	HPChanged bool
	MP        int
	MPChanged bool
}

// Vitals returns the character's current HP and MP as integer resource values.
func (c *Character) Vitals() Vitals {
	return Vitals{HP: c.CurrentHP(), MP: c.CurrentMP()}
}

// ResourceValues returns a synchronized HP/MP/CP resource snapshot. The
// maxima are whole points (see finalMax); the current values keep their
// fractions. Once the client has been shown c's maxima (AdoptMaxima), a
// living character's current value never reads above its maximum: a
// maximum that dropped bounds the value the read returns at once, while
// the stored value drops, with its report, on c's queue (SettleMaxima).
// Until then the restored equipment and effects that raise the maxima may
// not apply yet, and the stored values read as they are.
func (c *Character) ResourceValues() Resources {
	c.vitalsMu.RLock()
	res := Resources{
		MaxHP: c.maxHP, CurrentHP: c.curHP,
		MaxMP: c.maxMP, CurrentMP: c.curMP,
		MaxCP: c.maxCP, CurrentCP: c.curCP,
	}
	bound := c.maximaShown && !c.dead.Load()
	c.vitalsMu.RUnlock()
	res.MaxHP = c.finalMax(stat.MaxHP, res.MaxHP)
	res.MaxMP = c.finalMax(stat.MaxMP, res.MaxMP)
	res.MaxCP = c.finalMax(stat.MaxCP, res.MaxCP)
	c.noteMaxima(res)
	if bound {
		res.CurrentHP = min(res.CurrentHP, res.MaxHP)
		res.CurrentMP = min(res.CurrentMP, res.MaxMP)
		res.CurrentCP = min(res.CurrentCP, res.MaxCP)
	}
	return res
}

// boundedLocked returns cur, a current value whose maximum is maxValue, as
// ResourceValues reads it: bounded by the maximum once the client has been
// shown c's maxima, for a living character. A write that starts from the
// current value starts from the bounded one, so a maximum that dropped
// takes effect for it before SettleMaxima runs. The caller holds vitalsMu.
func (c *Character) boundedLocked(cur, maxValue float64) float64 {
	if c.maximaShown && !c.dead.Load() {
		return min(cur, maxValue)
	}
	return cur
}

// noteMaxima settles c's maxima (SettleMaxima) when a read finds one
// changed since the last read. A read runs under the regeneration task's
// own lock (regenShort) and under other actors' locks, so the settle is
// posted to c's queue. A character not in the world has no queue; it
// settles as it enters (AdoptMaxima, SettleEnteredRegen).
func (c *Character) noteMaxima(res Resources) {
	hp, mp, cp := int64(res.MaxHP), int64(res.MaxMP), int64(res.MaxCP)
	if c.seenMaxHP.Load() == hp && c.seenMaxMP.Load() == mp && c.seenMaxCP.Load() == cp {
		return
	}
	changed := c.seenMaxHP.Swap(hp) != hp
	changed = c.seenMaxMP.Swap(mp) != mp || changed
	changed = c.seenMaxCP.Swap(cp) != cp || changed
	if !changed {
		return
	}
	if q := c.liveLocked().Queue(); q != nil {
		q.Post(c.SettleMaxima)
	}
}

// finalMax finalizes a raw HP/MP/CP maximum through c's calculator and drops
// the fraction. A maximum is a whole-point value: every restore, refill,
// full check and percent-of-max formula measures against the truncated
// number, so a pool the client shows as full cannot absorb a sub-point
// "restore" (and the status packet it would trigger). Current values stay
// fractional.
func (c *Character) finalMax(s stat.Stat, base float64) float64 {
	if c.template() != nil {
		base = c.calcStat(s, base)
	}
	return math.Trunc(base)
}

// SetResourceValues replaces c's persisted HP/MP/CP resource values.
func (c *Character) SetResourceValues(res Resources) {
	defer c.SettleRegen() // after the unlock below
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	c.maxHP = res.MaxHP
	c.writeHPLocked(res.CurrentHP)
	c.maxMP, c.curMP = res.MaxMP, res.CurrentMP
	c.maxCP, c.curCP = res.MaxCP, res.CurrentCP
}

// RestoreVitals re-seeds the raw max calculator bases from tmpl's per-level
// tables at c's stored level, keeping the current HP/MP/CP values as loaded.
// The max columns persisted on the characters row are finalized snapshots
// written through ResourceValues, not bases, so restoring them into the base
// fields would re-apply the CON/MEN finalize on every read and compound
// across save→load cycles. Maxima are always recomputed from the live
// template; only the current values survive a round trip unchanged. Levels
// without a table row (matching AddLevel's refill convention) leave the
// stored bases untouched.
func (c *Character) RestoreVitals(tmpl *Template) {
	if tmpl == nil {
		return
	}
	idx := c.CharLevel - 1
	if idx < 0 || idx >= len(tmpl.HPTable) || idx >= len(tmpl.MPTable) || idx >= len(tmpl.CPTable) {
		return
	}
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	c.maxHP = tmpl.HPTable[idx]
	c.maxMP = tmpl.MPTable[idx]
	c.maxCP = tmpl.CPTable[idx]
}

// CurrentHP returns current HP as an integer resource value.
func (c *Character) CurrentHP() int {
	return int(c.ResourceValues().CurrentHP)
}

// CurrentMP returns current MP as an integer resource value.
func (c *Character) CurrentMP() int {
	return int(c.ResourceValues().CurrentMP)
}

// CurrentCP returns current CP as an integer resource value.
func (c *Character) CurrentCP() int {
	return int(c.ResourceValues().CurrentCP)
}

// ReduceCurrentHP subtracts non-negative HP and reports whether this call
// newly killed the character.
//
// Death is a half-point threshold, not a zero crossing: HP is set and the
// character then dies on HP < 0.5, so any remainder under half a point is
// already a death. A zero crossing would miss it, and the gap is reachable
// rather than theoretical because the callers that gate on affordability
// compare against truncated HP: a character on 10.4 HP reports 10, pays a
// cost of 10 in full, and lands on 0.4 — dead, displayed
// at 0 HP either way. The remainder is then cleared so a repeat call sees
// an already-dead character.
//
// Every damage route applies the same threshold (creature.DeathHP).
func (c *Character) ReduceCurrentHP(amount int) bool {
	if amount < 0 {
		amount = 0
	}
	maxHP := c.MaxHPValue()
	defer c.settleRegenTask() // after the unlock below
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.Dead() {
		return false
	}
	hp := c.boundedLocked(c.curHP, maxHP) - float64(amount)
	if hp >= creature.DeathHP {
		c.writeHPLocked(hp)
		return false
	}
	c.writeHPLocked(0)
	return true
}

// ReduceCurrentMP subtracts MP and clamps at zero. A dead character loses
// nothing.
func (c *Character) ReduceCurrentMP(amount int) {
	if amount <= 0 {
		return
	}
	maxMP := c.MaxMPValue()
	defer c.SettleRegen() // after the unlock below
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.dead.Load() {
		return
	}
	c.curMP = c.boundedLocked(c.curMP, maxMP) - float64(amount)
	if c.curMP < 0 {
		c.curMP = 0
	}
}

// refillResources installs new base maxima and fills current HP/MP/CP to
// the calculated maxima. A dead character takes the new maxima but keeps its
// current values: a level gained while dead must not raise the corpse. The
// check shares vitalsMu with MarkDead. A refill to full stops the
// regeneration task, so the next drop starts a fresh phase.
func (c *Character) refillResources(maxHP, maxMP, maxCP float64) {
	currentHP := c.finalMax(stat.MaxHP, maxHP)
	currentMP := c.finalMax(stat.MaxMP, maxMP)
	currentCP := c.finalMax(stat.MaxCP, maxCP)
	defer c.SettleRegen() // after the unlock below
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	c.maxHP, c.maxMP, c.maxCP = maxHP, maxMP, maxCP
	if c.dead.Load() {
		return
	}
	c.writeHPLocked(currentHP)
	c.curMP, c.curCP = currentMP, currentCP
}

// ChangesTo reports which resources differ in next.
func (v Vitals) ChangesTo(next Vitals) VitalsChange {
	return VitalsChange{
		HP:        next.HP,
		HPChanged: v.HP != next.HP,
		MP:        next.MP,
		MPChanged: v.MP != next.MP,
	}
}

// Changed reports whether any resource changed.
func (c VitalsChange) Changed() bool {
	return c.HPChanged || c.MPChanged
}
