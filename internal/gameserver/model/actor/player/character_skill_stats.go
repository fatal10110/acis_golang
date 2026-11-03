package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Against is c's stats read for a calculation against target run for
// skill (either nil for none).
func (c *Character) Against(target conditions.Actor, skill conditions.Skill) creature.SkillStats {
	return characterSkillStats{c: c, target: target, skill: skill}
}

type characterSkillStats struct {
	c      *Character
	target conditions.Actor
	skill  conditions.Skill
}

func (s characterSkillStats) CalcStat(st stat.Stat, base float64) float64 {
	return s.c.calcFor(st, base, s.target, s.skill)
}

func (s characterSkillStats) PAtk() float64 { return s.c.pAtk(s.target) }

func (s characterSkillStats) PDef() float64 { return s.c.pDef(s.target) }

func (s characterSkillStats) MAtk() float64 { return s.c.mAtk(s.target, s.skill) }

func (s characterSkillStats) MDef() float64 { return s.c.mDef(s.target, s.skill) }

func (s characterSkillStats) Evasion() int { return s.c.evasion(s.target) }

func (s characterSkillStats) CriticalRate() float64 { return s.c.criticalRate(s.target) }

func (s characterSkillStats) MagicCriticalRate() float64 {
	return s.c.calcFor(stat.MCriticalRate, 8, s.target, s.skill)
}
