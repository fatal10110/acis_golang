package cast

import (
	"fmt"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// CanCastToggle validates the one pre-cast check a toggle skill keeps: its
// reuse-delay cooldown. A toggle skips every other check CanCast applies to
// an ordinary cast — MP/HP, mute state, required items — because those
// only ever apply when CastToggle activates it, never when it deactivates
// an already-running instance.
func (c *Controller) CanCastToggle(def modelskill.Definition) error {
	if c.actor == nil {
		return ErrInvalidTarget
	}
	if def.Activation != modelskill.ActivationToggle {
		return fmt.Errorf("cast: skill %d level %d is not a toggle skill", def.ID, def.Level)
	}
	if c.actor.SkillDisabled(ReuseKey(def)) {
		return ErrSkillDisabled
	}
	return nil
}

// CastToggle applies casting a toggle skill. alreadyActive reports whether
// the caller's live effect state currently holds a running instance of
// def's skill — resolving that lookup is the caller's job, matching a
// toggle skill's on/off rule: recasting an active toggle turns it off at
// no cost, while casting an inactive one activates it by paying its MP/HP
// cost up front. activated reports which branch ran; when true, the caller
// still owns applying the skill's actual effects, exactly as it would for
// any other successfully started cast.
//
// Unlike Start, activating a toggle never installs a reuse delay and has
// no separate Hit/Finish phase — its whole cost applies immediately, and
// this method does not touch the Controller's casting state, since a
// toggle's cast window is effectively instantaneous.
//
// MP is checked and paid before HP is checked at all: a toggle that has
// enough MP but not enough HP still loses the MP, uncredited, when
// activation then fails on the HP check. This mirrors the exact order the
// two costs are validated in and is not a transactional all-or-nothing
// charge.
func (c *Controller) CastToggle(alreadyActive bool, def modelskill.Definition) (activated bool, err error) {
	if err := c.CanCastToggle(def); err != nil {
		return false, err
	}
	if alreadyActive {
		return false, nil
	}

	if mp := c.actor.MPCost(def); mp > 0 {
		if mp > c.actor.MP() {
			return false, ErrNotEnoughMP
		}
		c.actor.ReduceMP(mp)
	}
	if hp := def.HPConsume; hp > 0 {
		if hp > c.actor.HP() {
			return false, ErrNotEnoughHP
		}
		c.actor.ReduceHP(hp)
	}
	return true, nil
}
