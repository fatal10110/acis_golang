package skill

import "strings"

// LandingPower is the base percent chance a skill's own landing roll starts
// from: the first positive effect power among its effect templates, else
// its own positive effectPower, else 20 for PDAM and MDAM, else its power
// when that lies in (0, 100], else 20.
func (d Definition) LandingPower() float64 {
	for _, t := range d.Effects {
		if t.EffectPower > 0 {
			return t.EffectPower
		}
	}
	if d.Extra().EffectPower > 0 {
		return float64(d.Extra().EffectPower)
	}
	switch normalizedSkillType(d.SkillType) {
	case SkillTypePDam, SkillTypeMDam:
		return 20
	}
	if d.Power <= 0 || d.Power > 100 {
		return 20
	}
	return float64(d.Power)
}

// LandingEffectType is the skill's resolved effect type, the type its own
// landing roll resists as and a negate matches its effects by: the first
// effect type among its effect templates, else its own effectType, else
// STUN for PDAM and PARALYZE for MDAM, else its skill type.
func (d Definition) LandingEffectType() string {
	for _, t := range d.Effects {
		if t.EffectType != "" {
			return t.EffectType
		}
	}
	if d.Extra().EffectType != "" {
		return d.Extra().EffectType
	}
	switch normalizedSkillType(d.SkillType) {
	case SkillTypePDam:
		return string(SkillTypeStun)
	case SkillTypeMDam:
		return string(SkillTypeParalyze)
	}
	return string(d.SkillType)
}

// normalizedSkillType is t upper-cased and trimmed.
func normalizedSkillType(t SkillType) SkillType {
	return SkillType(strings.ToUpper(strings.TrimSpace(string(t))))
}
