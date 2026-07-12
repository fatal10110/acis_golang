package skill

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type hpDamageTarget interface {
	Dead() bool
	ReduceHP(amount float64, attacker any, skill modelskill.Definition)
}

type physicalSkillTarget interface {
	hpDamageTarget
	PhysicalSkillInput(caster any, skill modelskill.Definition) (formulas.PhysicalSkillInput, bool)
}

type magicDamageTarget interface {
	hpDamageTarget
	MagicDamageInput(caster any, skill modelskill.Definition) (formulas.MagicDamageInput, bool)
}

type blowDamageTarget interface {
	hpDamageTarget
	BlowInput(caster any, skill modelskill.Definition) (formulas.BlowInput, bool)
}

type manaDamageTarget interface {
	Dead() bool
	MP() float64
	ReduceMP(float64) float64
	ManaDamageInput(caster any, skill modelskill.Definition) (formulas.ManaDamageInput, bool)
}

type pdamHandler struct{}

func (pdamHandler) Types() []string { return []string{"PDAM", "FATAL"} }

func (pdamHandler) Use(cast Cast) {
	if alikeDead(cast.Caster) {
		return
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(physicalSkillTarget)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.PhysicalSkillInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		damage := formulas.PhysicalSkillDamage(in)
		if damage > 0 {
			target.ReduceHP(damage, cast.Caster, cast.Skill)
		}
	}
}

type mdamHandler struct{}

func (mdamHandler) Types() []string { return []string{"MDAM", "DEATHLINK"} }

func (mdamHandler) Use(cast Cast) {
	if alikeDead(cast.Caster) {
		return
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(magicDamageTarget)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.MagicDamageInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		damage := int(formulas.MagicDamage(in))
		if damage > 0 {
			target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
		}
	}
}

type blowHandler struct{}

func (blowHandler) Types() []string { return []string{"BLOW"} }

func (blowHandler) Use(cast Cast) {
	if alikeDead(cast.Caster) {
		return
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(blowDamageTarget)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.BlowInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		damage := int(formulas.BlowDamage(in))
		if damage > 0 {
			target.ReduceHP(float64(damage), cast.Caster, cast.Skill)
		}
	}
}

type manaDamageHandler struct{}

func (manaDamageHandler) Types() []string { return []string{"MANADAM"} }

func (manaDamageHandler) Use(cast Cast) {
	if alikeDead(cast.Caster) {
		return
	}
	for _, obj := range cast.Targets {
		target, ok := obj.(manaDamageTarget)
		if !ok || target.Dead() {
			continue
		}
		in, ok := target.ManaDamageInput(cast.Caster, cast.Skill)
		if !ok {
			continue
		}
		damage := formulas.ManaDamage(in)
		if damage > target.MP() {
			damage = target.MP()
		}
		if damage > 0 {
			target.ReduceMP(damage)
		}
	}
}
