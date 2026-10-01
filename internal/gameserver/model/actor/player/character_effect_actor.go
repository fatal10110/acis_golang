package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

var (
	_ effect.PlayerActor = (*Character)(nil)
	_ effect.CasterActor = (*Character)(nil)
)

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

// DenyAIActionBeforeEffect is DenyAIAction leaving out the effect whose
// on-start hook is running, if any: called from inside that hook, it
// answers for the state c was in before the effect landed.
func (c *Character) DenyAIActionBeforeEffect() bool {
	return c.aiDeniedBeforeEffect() || c.ObserverMode() || c.InStoreMode()
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

// AddChanceTrigger registers a started chance-skill-trigger effect as one of
// c's chance procs.
func (c *Character) AddChanceTrigger(e *effect.Effect) { c.EffectList().AddChanceTrigger(e) }

// RemoveChanceTrigger drops an exiting chance-skill-trigger effect from c's
// chance procs.
func (c *Character) RemoveChanceTrigger(e *effect.Effect) { c.EffectList().RemoveChanceTrigger(e) }

// StopCharmOfLuck runs when a Charm of Luck ends: the player's appearance is
// refreshed for observers.
func (c *Character) StopCharmOfLuck(*effect.Effect) { c.BroadcastAbnormalEffect() }

// StopPhoenixBlessing runs when a Phoenix Blessing ends: the player's
// appearance is refreshed for observers.
func (c *Character) StopPhoenixBlessing(*effect.Effect) { c.BroadcastAbnormalEffect() }

// StopProtectionBlessing runs when a Blessing of Protection loses its stack
// group's head: the player's appearance is refreshed for observers.
func (c *Character) StopProtectionBlessing(*effect.Effect) { c.BroadcastAbnormalEffect() }

// BroadcastEtcStatus refreshes whether c holds a Charm of Courage from its
// effect list, then sends c's status-window flags to c and every player that
// sees it. The Charm of Courage hooks call it outside the effect list's lock.
func (c *Character) BroadcastEtcStatus() {
	c.charmMu.Lock()
	c.charmOfCourage.Store(c.EffectList().IsAffected(effect.FlagCharmOfCourage))
	c.charmMu.Unlock()
	c.emit(event.EtcStatusBroadcast{})
}

// CharmOfCourage reports whether c held a Charm of Courage when its last
// one started or ended. It never takes the effect list's lock, so status
// packets built from inside a stat callback can read it.
func (c *Character) CharmOfCourage() bool { return c.charmOfCourage.Load() }

// stopPhoenixBlessing uses up every Phoenix Blessing c holds, then refreshes
// its appearance once more.
func (c *Character) stopPhoenixBlessing() {
	c.EffectList().StopByType(effect.TypePhoenixBless)
	c.BroadcastAbnormalEffect()
}

// UpdateEffectIcons reports that the player's active-effect icon list must
// be resent. The effect list calls it on every add or remove attempt,
// whether or not the attempt changed anything.
func (c *Character) UpdateEffectIcons() { c.emit(event.EffectIconsChanged{}) }
