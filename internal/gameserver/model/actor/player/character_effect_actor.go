package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
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

// The effect hooks below have no player behavior yet; each is a deliberate
// no-op (or false) so the effect runs exactly as it did before the player
// side existed.

// FearImmune reports false: fear immunity is not modeled yet.
func (c *Character) FearImmune() bool { return false }

// FleeFrom reports false: fleeing movement is not modeled yet.
func (c *Character) FleeFrom(effect.Actor, int) bool { return false }

// BluffExempt reports false: players are never exempt from bluff.
func (c *Character) BluffExempt() bool { return false }

// StopEffects does nothing yet: stopping effects by type is not wired.
func (c *Character) StopEffects(effect.Type) {}

// StopSkillEffectsByID does nothing yet: stopping effects by skill is not
// wired.
func (c *Character) StopSkillEffectsByID(modelskill.ID) {}

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
