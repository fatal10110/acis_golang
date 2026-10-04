package skill

// FishingCast asks for a player caster's Fishing skill to cast its line, or
// to take it out of the water when one is already cast.
type FishingCast struct{}

// FishingAction asks for a player caster's pumping or reeling skill to act
// on the fish it fights: Power and Level are the skill's.
type FishingAction struct {
	Reeling bool
	Power   float32
	Level   int
}

type fishingHandler struct{}

func (fishingHandler) Types() []string { return []string{"FISHING"} }

// Use hands a player caster's Fishing cast to the fishing stance.
func (fishingHandler) Use(cast Cast) {
	if _, ok := asPlayer(cast.Caster); ok {
		cast.record(FishingCast{})
	}
}

type fishingSkillHandler struct{}

func (fishingSkillHandler) Types() []string { return []string{"PUMPING", "REELING"} }

// Use hands a player caster's pumping or reeling cast to the fishing
// stance.
func (fishingSkillHandler) Use(cast Cast) {
	if _, ok := asPlayer(cast.Caster); !ok {
		return
	}
	cast.record(FishingAction{
		Reeling: skillTypeKey(cast.Skill.SkillType) == "REELING",
		Power:   cast.Skill.Power,
		Level:   cast.Skill.Level,
	})
}
