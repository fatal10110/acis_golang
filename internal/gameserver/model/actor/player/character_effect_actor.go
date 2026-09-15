package player

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

var _ effect.PlayerActor = (*Character)(nil)

// The effect hooks below have no player behavior yet; each is a deliberate
// no-op (or false) so the effect runs exactly as it did before the player
// side existed.

// AbortAll does nothing yet: aborting every in-progress action is not wired.
func (c *Character) AbortAll(bool) {}

// StopMove does nothing yet: effect-driven movement stops are not wired.
func (c *Character) StopMove() {}

// ClearTarget does nothing yet: effect-driven target clearing is not wired.
func (c *Character) ClearTarget() {}

// StopAttack does nothing yet: effect-driven attack stops are not wired.
func (c *Character) StopAttack() {}

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
