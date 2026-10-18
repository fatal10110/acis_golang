package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Against is h's stats read for a calculation against target run for
// skill (either nil for none).
func (h *Hostile) Against(target conditions.Actor, skill conditions.Skill) creature.SkillStats {
	return hostileSkillStats{h: h, target: target, skill: skill}
}

type hostileSkillStats struct {
	h      *Hostile
	target conditions.Actor
	skill  conditions.Skill
}

func (s hostileSkillStats) CalcStat(st stat.Stat, base float64) float64 {
	return s.h.calcFor(st, base, s.target, s.skill)
}

func (s hostileSkillStats) PAtk() float64 {
	return math.Trunc(s.h.calcFor(stat.PowerAttack, s.h.Instance.Template.PAtk, s.target, nil))
}

func (s hostileSkillStats) PDef() float64 {
	base := s.h.Instance.Template.PDef * s.h.raidBaseMultipliers().Defence
	return math.Trunc(s.h.calcFor(stat.PowerDefence, base, s.target, nil))
}

func (s hostileSkillStats) MAtk() float64 {
	return math.Trunc(s.h.calcFor(stat.MagicAttack, positiveStat(s.h.Instance.Template.MAtk), s.target, s.skill))
}

func (s hostileSkillStats) MDef() float64 {
	base := positiveStat(s.h.Instance.Template.MDef) * s.h.raidBaseMultipliers().Defence
	return math.Trunc(s.h.calcFor(stat.MagicDefence, base, s.target, s.skill))
}

// Evasion is the physical evasion rating (per-mille), finalized through the
// stat calculator (base DEX/level plus any active buff/debuff).
func (s hostileSkillStats) Evasion() int {
	return int(s.h.calcFor(stat.EvasionRate, 0, s.target, nil))
}

// CriticalRate is the physical critical rate, capped at 500.
func (s hostileSkillStats) CriticalRate() float64 {
	return float64(min(int(s.h.calcFor(stat.CriticalRate, s.h.Instance.Template.CritRate, s.target, nil)), 500))
}

func (s hostileSkillStats) MagicCriticalRate() float64 {
	return s.h.calcFor(stat.MCriticalRate, 8, s.target, s.skill)
}
