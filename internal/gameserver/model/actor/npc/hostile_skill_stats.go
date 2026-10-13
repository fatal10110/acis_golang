package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ForSkill is h's stats read for a calculation run for skill.
func (h *Hostile) ForSkill(skill conditions.Skill) creature.SkillStats {
	return hostileSkillStats{h: h, skill: skill}
}

type hostileSkillStats struct {
	h     *Hostile
	skill conditions.Skill
}

func (s hostileSkillStats) CalcStat(st stat.Stat, base float64) float64 {
	return s.h.calcSkillStat(st, base, s.skill)
}

func (s hostileSkillStats) MAtk() float64 {
	return math.Trunc(s.h.calcSkillStat(stat.MagicAttack, positiveStat(s.h.Instance.Template.MAtk), s.skill))
}

func (s hostileSkillStats) MDef() float64 {
	base := positiveStat(s.h.Instance.Template.MDef) * s.h.raidBaseMultipliers().Defence
	return math.Trunc(s.h.calcSkillStat(stat.MagicDefence, base, s.skill))
}

func (s hostileSkillStats) MagicCriticalRate() float64 {
	return s.h.calcSkillStat(stat.MCriticalRate, 8, s.skill)
}
