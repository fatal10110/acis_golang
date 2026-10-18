package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door/doorhit"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// SiegeAssaultCheck runs the siege check of a STRIDER_SIEGE_ASSAULT cast
// def by the player caster on target as the skill lands, telling the
// caster a refusal, and returns the door the cast hits.
type SiegeAssaultCheck func(caster, target Actor, def modelskill.Definition) (*door.Object, bool)

// striderSiegeAssaultHandler lands a STRIDER_SIEGE_ASSAULT skill: a player
// riding a strider whose clan attacks the siege under way strikes a door
// with a physical skill hit. It never evades, carries no shield and lands no
// effect; a hit dealing nothing is answered ATTACK_FAILED.
type striderSiegeAssaultHandler struct{ check SiegeAssaultCheck }

// NewStriderSiegeAssault returns the STRIDER_SIEGE_ASSAULT handler, which
// runs check again as each cast lands.
func NewStriderSiegeAssault(check SiegeAssaultCheck) Handler {
	return striderSiegeAssaultHandler{check: check}
}

func (striderSiegeAssaultHandler) Types() []string { return []string{"STRIDER_SIEGE_ASSAULT"} }

func (h striderSiegeAssaultHandler) Use(cast Cast) { h.UseResult(cast) }

func (h striderSiegeAssaultHandler) UseResult(cast Cast) Result {
	result := Result{messages: cast.messages}
	if _, ok := asPlayer(cast.Caster); !ok || len(cast.Targets) == 0 || h.check == nil {
		return result
	}
	d, ok := h.check(cast.Caster, cast.Targets[0], cast.Skill)
	if !ok || d.Dead() {
		return result
	}
	target := doorhit.Of(d)
	in, ok := target.PhysicalSkillInput(cast.Caster, cast.Skill)
	if !ok {
		return result
	}
	if damage := javaInt(formulas.PhysicalSkillDamage(in)); damage > 0 {
		recordDamage(&result, cast.Caster, target, damage, false, false)
		target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
	} else {
		result.AttackFailed++
		result.record(AttackFailedMessage{})
	}
	dischargeSoulshot(cast)
	return result
}
