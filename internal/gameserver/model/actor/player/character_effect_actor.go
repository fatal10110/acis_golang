package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

var _ effect.PlayerActor = (*Character)(nil)

// AbortAll stops c's movement, attack and cast, then clears its target when
// resetTarget is set.
func (c *Character) AbortAll(resetTarget bool) {
	c.emit(event.ActionsStopRequested{Move: true, Attack: true, Cast: true, AIDenied: c.aiDeniedBeforeEffect()})
	if resetTarget {
		c.SetTarget(nil)
	}
}

// StopMove stops c's movement.
func (c *Character) StopMove() { c.emit(event.ActionsStopRequested{Move: true}) }

// ClearTarget clears c's target selection without touching its intentions.
func (c *Character) ClearTarget() {
	if c.sink == nil {
		c.StoreTarget(nil)
		return
	}
	c.emit(event.ActionsStopRequested{ClearTarget: true})
}

// StopAttack stops c's attack.
func (c *Character) StopAttack() {
	c.emit(event.ActionsStopRequested{Attack: true, AIDenied: c.aiDeniedBeforeEffect()})
}

func (c *Character) aiDeniedBeforeEffect() bool {
	return c.Dead() || c.liveLocked().AIDeniedBeforeEffect()
}

// FearImmune reports false: only folk, siege flags and siege summons shrug
// off fear.
func (c *Character) FearImmune() bool { return false }

// FleeFrom asks c's controller to run distance units directly away from
// effector. The request carries whether c could take AI actions before the
// effect in progress landed: a fear's own first flee moves c, while the
// flees on its later ticks find c already afraid and are refused.
func (c *Character) FleeFrom(effector effect.Actor, distance int) {
	if effector == nil || effector.ObjectID() == c.ObjectID() || distance < 10 {
		return
	}
	x, y, z := effector.Position()
	c.emit(event.FleeRequested{
		From:     location.Location{X: x, Y: y, Z: z},
		Distance: distance,
		AIDenied: c.aiDeniedBeforeEffect(),
	})
}

// BluffExempt reports false: players are never exempt from bluff.
func (c *Character) BluffExempt() bool { return false }

// StopEffects removes every effect of type t that c holds.
func (c *Character) StopEffects(t effect.Type) { c.EffectList().StopByType(t) }

// StopSkillEffectsByID removes every effect skill id applied to c.
func (c *Character) StopSkillEffectsByID(id modelskill.ID) { c.EffectList().StopBySkillID(id) }

// The chance-trigger and blessing hooks below have no player behavior yet;
// each is a deliberate no-op so the effect runs exactly as it did before the
// player side existed.

// AddChanceTrigger does nothing yet: chance skill triggers are not wired.
func (c *Character) AddChanceTrigger(*effect.Effect) {}

// RemoveChanceTrigger does nothing yet: chance skill triggers are not wired.
func (c *Character) RemoveChanceTrigger(*effect.Effect) {}

// StopCharmOfLuck does nothing yet: Charm of Luck state is not modeled.
func (c *Character) StopCharmOfLuck(*effect.Effect) {}

// StopPhoenixBlessing does nothing yet: Phoenix Blessing state is not
// modeled.
func (c *Character) StopPhoenixBlessing(*effect.Effect) {}

// UpdateEffectIcons refreshes the player's effect icons.
func (c *Character) UpdateEffectIcons() { c.UpdateAbnormalEffect() }
