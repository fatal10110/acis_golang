package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ForSkill is c's stats read for a calculation run for skill.
func (c *Character) ForSkill(skill conditions.Skill) creature.SkillStats {
	return characterSkillStats{c: c, skill: skill}
}

type characterSkillStats struct {
	c     *Character
	skill conditions.Skill
}

func (s characterSkillStats) CalcStat(st stat.Stat, base float64) float64 {
	return s.c.calcSkillStat(st, base, s.skill)
}

func (s characterSkillStats) MAtk() float64 { return s.c.mAtk(s.skill) }

func (s characterSkillStats) MDef() float64 { return s.c.mDef(s.skill) }

func (s characterSkillStats) MagicCriticalRate() float64 {
	return s.c.calcSkillStat(stat.MCriticalRate, 8, s.skill)
}
