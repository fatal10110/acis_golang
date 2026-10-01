package skill

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// GrantTransientSkills adds each of refs to c's known-skill set without
// persisting it, for skills c holds only while something outside its own
// learning lasts (its clan's skills). A passive one's stat functions are
// attached; a skill c already knows at that level changes nothing, and at
// another level the known level's functions are dropped first. A ref
// naming no loaded skill is skipped.
func (p *Persistence) GrantTransientSkills(c *player.Character, refs []modelskill.Ref) error {
	if p == nil || c == nil {
		return nil
	}
	grants := make([]itemSkillGrant, 0, len(refs))
	for _, ref := range refs {
		g, ok, err := p.listenerSkillGrant(ref)
		if err != nil {
			return fmt.Errorf("grant skill %d level %d: %w", ref.ID, ref.Level, err)
		}
		if ok {
			grants = append(grants, g)
		}
	}
	p.grantItemSkills(c, grants)
	return nil
}

// RevokeSkills drops each of ids from c's known-skill set with the stat
// functions of the level c knows it at; an id c does not know is skipped.
func (p *Persistence) RevokeSkills(c *player.Character, ids []modelskill.ID) {
	if c == nil {
		return
	}
	for _, id := range ids {
		removeItemSkill(c, int(id))
	}
}
