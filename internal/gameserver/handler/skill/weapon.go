package skill

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// WeaponSkillLands rolls a weapon-triggered skill def against target: the
// target's shield block against def, then def's landing roll without a
// blessed spiritshot. It reports the shield outcome and whether def landed;
// a target with no landing-rate source never lands it.
func WeaponSkillLands(caster Creature, target Actor, def modelskill.Definition) (formulas.ShieldDefense, bool) {
	shield := resolveShieldDefense(caster, target, def)
	landed, ok := checkSkillSuccessBSSWithShield(caster, target, def, false, shield)
	return shield, ok && landed
}

// LandCritSkill lands def, the skill a weapon casts on a critical hit, from
// caster on target once WeaponSkillLands has passed with shield: target's
// current effect of def gives way, then def's effects land with that shield
// outcome and no blessed spiritshot. No handler runs. The result reports
// each icon effect target resisted.
func LandCritSkill(caster Creature, target Actor, def modelskill.Definition, shield formulas.ShieldDefense) Result {
	messages := &messageLog{}
	result := Result{messages: messages}
	cast := Cast{Caster: caster, Skill: def, Targets: []Actor{target}, resisted: &result, messages: messages}
	if t, ok := asCreature(target); ok {
		if e := firstEffectByID(t.EffectList(), def.ID); e != nil {
			t.EffectList().Remove(e)
		}
	}
	applyCastEffects(cast, target, def, def.Effects, shield, false)
	result.messages = nil
	result.Messages = messages.pending
	return result
}
