package skill

import (
	"context"
	"sort"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// enchantedSkillMinLevel is the character level from which an enchanted
// skill — one levelled past the top of its own table — is allowed to stay
// enchanted. Below it, a level correction always pulls the skill back to the
// plain level the profession grants.
const enchantedSkillMinLevel = 76

// GiveSkills re-derives every skill a character holds purely because of its
// level, for tmpl's profession line, and reconciles what it already knows
// with what the level now supports:
//
//   - every zero-cost grant the level unlocks is handed over for free, at
//     the highest level unlocked;
//   - Lucky is taken away once the character reaches
//     modelskill.LuckySkillMaxLevel;
//   - a known profession skill the level no longer supports is dropped, and
//     one held above the level's grant is pulled back down to it.
//
// The free grants and the Lucky removal stay in memory: they follow from the
// level and are recomputed on every level change. The corrections in the
// last step are persisted, because they undo something the character really
// did learn.
//
// Callers own the client's view of the result; GiveSkills sends nothing.
func (p *Persistence) GiveSkills(ctx context.Context, c *player.Character, tmpl *player.Template) error {
	if p == nil || c == nil || tmpl == nil {
		return nil
	}
	level := c.CharLevel
	for _, grant := range tmpl.AutoGetSkillGrants(level, c.SkillLevels()) {
		if err := p.setKnownSkill(ctx, c, grant.SkillID, grant.Level, false); err != nil {
			return err
		}
	}
	lucky := int(modelskill.LuckySkillID)
	if level >= modelskill.LuckySkillMaxLevel && c.HasSkill(lucky) {
		if err := p.setKnownSkill(ctx, c, lucky, 0, false); err != nil {
			return err
		}
	}
	return p.correctInvalidSkills(ctx, c, tmpl, level)
}

// RewardSkills grants every skill the character's level unlocks. Bought
// grants are persisted while free grants remain level-derived in memory.
func (p *Persistence) RewardSkills(ctx context.Context, c *player.Character, tmpl *player.Template) error {
	if p == nil || c == nil || tmpl == nil {
		return nil
	}
	level := c.CharLevel
	for _, grant := range tmpl.AllAvailableSkillGrants(level, c.SkillLevels()) {
		if err := p.setKnownSkill(ctx, c, grant.SkillID, grant.Level, grant.Cost != 0); err != nil {
			return err
		}
	}
	lucky := int(modelskill.LuckySkillID)
	if level >= modelskill.LuckySkillMaxLevel && c.HasSkill(lucky) {
		if err := p.setKnownSkill(ctx, c, lucky, 0, false); err != nil {
			return err
		}
	}
	return p.correctInvalidSkills(ctx, c, tmpl, level)
}

// correctInvalidSkills drops or downgrades the profession skills a character
// at level may no longer hold at the level it holds them. Skills the
// profession line never grants — item, quest and temporary awards — are left
// alone.
func (p *Persistence) correctInvalidSkills(ctx context.Context, c *player.Character, tmpl *player.Template, level int) error {
	known := c.SkillLevels()
	if len(known) == 0 {
		return nil
	}
	reachable := tmpl.ReachableSkillGrants(level)

	ids := make([]int, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		knownLevel := known[id]
		if !tmpl.GrantsSkill(id) {
			continue
		}
		grant, ok := reachable[id]
		if !ok {
			if err := p.setKnownSkill(ctx, c, id, 0, true); err != nil {
				return err
			}
			continue
		}
		if knownLevel <= grant.Level {
			continue
		}
		// An enchanted skill sits above the top of its own table. It keeps
		// its enchant only while the character is high enough to hold one
		// and the profession already grants the skill at full table level;
		// otherwise it falls back to the granted level like any other.
		if maxLevel := p.MaxLevel(modelskill.ID(id)); knownLevel > maxLevel &&
			level >= enchantedSkillMinLevel && grant.Level >= maxLevel {
			continue
		}
		if err := p.setKnownSkill(ctx, c, id, grant.Level, true); err != nil {
			return err
		}
	}
	return nil
}
