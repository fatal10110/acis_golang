package skill

// RecipeBookOpened asks for the caster's dwarven or common recipe book page
// to be shown.
type RecipeBookOpened struct{ Dwarven bool }

// CraftWhileOperatingMessage refuses a craft skill cast while the caster
// runs a private store or workshop.
type CraftWhileOperatingMessage struct{}

type craftHandler struct{}

func (craftHandler) Types() []string { return []string{"COMMON_CRAFT", "DWARVEN_CRAFT"} }

// Use opens a player caster's recipe book on the page the skill names.
func (craftHandler) Use(cast Cast) {
	p, ok := asPlayer(cast.Caster)
	if !ok {
		return
	}
	if p.Operating() {
		cast.record(CraftWhileOperatingMessage{})
		return
	}
	cast.record(RecipeBookOpened{Dwarven: skillTypeKey(cast.Skill.SkillType) == "DWARVEN_CRAFT"})
}
