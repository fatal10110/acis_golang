package player

import "github.com/fatal10110/acis_golang/internal/gameserver/world"

// Target returns the character's currently selected target, or nil if none.
// This is the authoritative selected-target state; the network layer reads
// and writes through it instead of keeping its own copy.
func (c *Character) Target() world.Tracked {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.target
}

// SetTargetTracked records t as the character's currently selected target.
// A nil t clears the selection.
func (c *Character) SetTargetTracked(t world.Tracked) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.target = t
}

// CurrentTarget implements the retargetableOnAggression capability the
// AGGDEBUFF continuous-effect handler consults to decide whether to retarget
// or attack a playable target hit by a landed aggression-debuff effect.
func (c *Character) CurrentTarget() any {
	if t := c.Target(); t != nil {
		return t
	}
	return nil
}

// SetTarget implements retargetableOnAggression's setter. t must be a
// world.Tracked (or nil); any other type is ignored.
func (c *Character) SetTarget(t any) {
	if t == nil {
		c.SetTargetTracked(nil)
		return
	}
	tracked, ok := t.(world.Tracked)
	if !ok {
		return
	}
	c.SetTargetTracked(tracked)
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
func (c *Character) AttackTarget(t any) {
	tracked, ok := t.(world.Tracked)
	if !ok {
		return
	}
	c.stateMu.RLock()
	attack := c.attackTarget
	c.stateMu.RUnlock()
	if attack != nil {
		attack(tracked)
	}
}

// TryToAttack implements targetRedirectTarget's attack trigger.
func (c *Character) TryToAttack(t any) {
	c.AttackTarget(t)
}
