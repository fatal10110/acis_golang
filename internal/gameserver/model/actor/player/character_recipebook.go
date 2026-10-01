package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// RecipeBook returns c's recipe book.
func (c *Character) RecipeBook() *recipe.Book {
	return &c.recipes
}

// HasCommonCraft reports whether c knows Create Common Item, which lets it
// register common recipes.
func (c *Character) HasCommonCraft() bool {
	return c.SkillLevel(int(modelskill.CreateCommonSkillID)) > 0
}

// CreateItemLevel returns c's Create Item skill level for the dwarven or
// common page, 0 when it does not know the skill. It caps the recipe level
// c may register and craft on that page.
func (c *Character) CreateItemLevel(dwarven bool) int {
	if dwarven {
		return c.SkillLevel(int(modelskill.CreateDwarvenSkillID))
	}
	return c.SkillLevel(int(modelskill.CreateCommonSkillID))
}
