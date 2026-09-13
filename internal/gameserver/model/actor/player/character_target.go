package player

import (
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

// CurrentTarget implements the retargetableOnAggression capability the
// AGGDEBUFF continuous-effect handler consults to decide whether to retarget
// or attack a playable target hit by a landed aggression-debuff effect.
func (c *Character) CurrentTarget() world.Tracked { return c.Target() }

// SetTarget implements retargetableOnAggression's setter. A nil t clears
// the selection.
//
// Reference: Player.setTarget (Player.java:2439-2510) is the single packet
// funnel for every selection, click-driven or domain-driven alike — a
// non-null Creature target gets ValidateLocation (conditional)/
// MyTargetSelected/StatusUpdate/broadcast TargetSelected (:2474-2493), a
// null target gets ActionFailed and a conditional broadcast TargetUnselected
// (:2495-2503). Creature.setTarget's plain field write (Creature.java:
// 1353-1358) is only what the Summon runtime path hits, since no Playable
// subclass overrides it. retargetTarget is the network-owned hook that
// reproduces that funnel (see network.selectLiveTarget/clearLiveTarget);
// StoreTarget is the fallback for callers with no live session wired
// (e.g. tests).
func (c *Character) SetTarget(t world.Tracked) {
	c.stateMu.RLock()
	retarget := c.retargetTarget
	c.stateMu.RUnlock()
	if retarget != nil {
		retarget(t)
		return
	}
	c.StoreTarget(t)
}

// SetRetargetHook records the packet-layer hook engaged when a domain
// retarget (AGGDEBUFF, TargetMe) needs to reproduce Player.setTarget's
// packet funnel instead of a plain field write.
func (c *Character) SetRetargetHook(retarget func(world.Tracked)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.retargetTarget = retarget
}

// SetAttackTargetHook records the packet-layer hook engaged when an
// aggression-debuff effect provokes this character into attacking a target
// it was already facing.
func (c *Character) SetAttackTargetHook(attack func(world.Tracked)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.attackTarget = attack
}

// AttackTarget implements retargetableOnAggression's attack trigger.
func (c *Character) AttackTarget(t world.Tracked) {
	if t == nil {
		return
	}
	c.stateMu.RLock()
	attack := c.attackTarget
	c.stateMu.RUnlock()
	if attack != nil {
		attack(t)
	}
}

// TryToAttack implements targetRedirectTarget's attack trigger.
func (c *Character) TryToAttack(t world.Tracked) {
	c.AttackTarget(t)
}
