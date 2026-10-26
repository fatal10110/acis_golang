package cast

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// CallSkill makes the caster use ref on target at once, as a script asks:
// the players around the caster see the cast start, with ref's hit time and
// no reuse delay, then ref's effects land on target alone through its skill
// handler (CallSkill). The cast controller takes no part: no cast time,
// cost, reuse, target resolution or check, and a cast in flight goes on.
// It runs on the caller's goroutine. No target, or a skill with no
// definition, does nothing.
func (a *AIController) CallSkill(target attackable.Combatant, ref modelskill.Ref) {
	if a.Caster == nil || target == nil {
		return
	}
	def, ok := a.definition(ref)
	if !ok {
		return
	}
	caster, ok := a.Caster.(handlerskill.Creature)
	if !ok {
		return
	}
	actor, ok := target.(handlerskill.Actor)
	if !ok {
		return
	}
	tx, ty, tz := target.Position()
	a.Caster.BroadcastSkillUse(target.ObjectID(), tx, ty, tz, int32(def.ID), int32(def.Level), def.HitTime, 0)
	handlers := a.Effects
	if a.OnHitResult != nil && handlers.Sink == nil {
		handlers.Sink = func(message any) {
			a.OnHitResult(EffectResult{Messages: []any{message}})
		}
	}
	result := CallSkill(handlers, caster, actor, def)
	if a.OnHitResult != nil {
		a.OnHitResult(result)
	}
}
