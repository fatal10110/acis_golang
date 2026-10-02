package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// skillKeyLevels is how many levels one skill id spans in the skill table's
// combined (id, level) key: the key is id*skillKeyLevels+level.
const skillKeyLevels = 256

// TopLevelDefinitions returns every loaded skill at its highest regular
// (non-enchant) level, ordered by id.
func (p *Persistence) TopLevelDefinitions() []modelskill.Definition {
	if p == nil {
		return nil
	}
	return p.skills.TopLevels()
}

// KeyedDefinition resolves id and level the way the skill table's combined
// key does: the two fold into one id*256+level key (32-bit, wrapping), so a
// level outside 0-255 lands on another id's level, and the definition at
// that key is returned.
func (p *Persistence) KeyedDefinition(id, level int32) (modelskill.Definition, bool) {
	key := id*skillKeyLevels + level
	if key < 0 {
		return modelskill.Definition{}, false
	}
	return p.definition(modelskill.Ref{ID: modelskill.ID(key / skillKeyLevels), Level: int(key % skillKeyLevels)})
}

// ForgetSkill takes skillID away from c with its passive stats. With store
// set, its character_skills row is deleted too.
func (p *Persistence) ForgetSkill(c *player.Character, skillID int, store bool) {
	// A removal attaches nothing, so it cannot fail.
	_ = p.setKnownSkill(c, skillID, 0, store)
}
