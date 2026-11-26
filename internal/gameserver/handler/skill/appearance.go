package skill

import modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"

// appearanceChanger is a player whose face and hair a skill can change.
type appearanceChanger interface {
	ChangeAppearance(face, hairColor, hairStyle int)
}

// appearanceHandler changes the look of every player a CHANGE_APPEARANCE
// skill lands on.
type appearanceHandler struct{}

func (appearanceHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeChangeAppearance}
}

// Use gives every player target the skill's face, hair color and hair
// style, each one the skill leaves negative unchanged, and shows it anew to
// itself and the players around it. Other targets are passed over.
func (appearanceHandler) Use(cast Cast) {
	def := cast.Skill
	for _, target := range cast.Targets {
		if target == nil || target.Kind() != actor.KindPlayer {
			continue
		}
		if p, ok := target.(appearanceChanger); ok {
			p.ChangeAppearance(def.Extra().AppearanceFace, def.Extra().AppearanceHairColor, def.Extra().AppearanceHairStyle)
		}
	}
}
