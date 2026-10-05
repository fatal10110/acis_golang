package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Target returns the character's currently selected target, or nil if none.
// This is the authoritative selected-target state; the network layer reads
// and writes through it instead of keeping its own copy.
func (c *Character) Target() world.Tracked {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.target
}

// StoreTarget records t as the character's currently selected target and
// sends no packets. A nil t clears the selection. It is the raw state write
// behind the network target funnel; domain retargets use SetTarget so the
// client sees the selection change.
func (c *Character) StoreTarget(t world.Tracked) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.target = t
}

// TakeTarget clears the selection and returns the one it held, or nil. The
// read and the clear are one step, so a selection made by another goroutine
// in between is never dropped unannounced.
func (c *Character) TakeTarget() world.Tracked {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	old := c.target
	c.target = nil
	return old
}

// ClearTargetIf clears the selection when it is t and reports whether it
// did. The check and the clear are one step, so a selection made by the
// character's own goroutine in between is never wiped by another goroutine.
func (c *Character) ClearTargetIf(t world.Tracked) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if t == nil || c.target != t {
		return false
	}
	c.target = nil
	return true
}

// CurrentTarget implements the retargetableOnAggression capability the
// AGGDEBUFF continuous-effect handler consults to decide whether to retarget
// or attack a playable target hit by a landed aggression-debuff effect.
func (c *Character) CurrentTarget() world.Tracked { return c.Target() }

// SetTarget implements retargetableOnAggression's setter. A nil t clears
// the selection.
//
// Setting a player's target is the single packet funnel for every
// selection, click-driven or domain-driven alike — a non-null Creature
// target gets ValidateLocation (conditional)/MyTargetSelected/StatusUpdate/
// broadcast TargetSelected, a null target gets ActionFailed and a
// conditional broadcast TargetUnselected. A plain target field write is
// only what the Summon runtime path hits, since no other playable kind
// adds the funnel. A Retargeted event carries the selection to the
// network layer, which reproduces that funnel; StoreTarget is the fallback
// for a character with no sink attached (e.g. tests).
func (c *Character) SetTarget(t world.Tracked) {
	if c.sink == nil {
		c.StoreTarget(t)
		return
	}
	c.emit(event.Retargeted{Target: t})
}

// AttackTarget implements retargetableOnAggression's attack trigger.
func (c *Character) AttackTarget(t world.Tracked) {
	if t == nil {
		return
	}
	c.emit(event.AttackRequested{Target: t})
}

// TryToAttack implements targetRedirectTarget's attack trigger.
func (c *Character) TryToAttack(t world.Tracked) {
	c.AttackTarget(t)
}
