package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// cubicSummoner is the narrow surface a cubic-granting SUMMON skill needs
// from a target: admit or refresh the cubic on that target's own list.
type cubicSummoner interface {
	AddOrRefreshCubic(id cubic.ID, givenByOther bool) (touched, added bool)
}

type servitorSummoner interface {
	SummonServitor(modelskill.Definition)
}

// cubicHandler applies either branch of a SkillType.SUMMON cast: a servitor
// for non-cubic skills, or a cubic refresh for cubic skills.
type cubicHandler struct{}

func (cubicHandler) Types() []string { return []string{"SUMMON"} }

func (cubicHandler) Use(cast Cast) {
	cubicHandler{}.UseResult(cast)
}

func (cubicHandler) UseResult(cast Cast) Result {
	if !cast.Skill.IsCubic {
		if summoner, ok := cast.Caster.(servitorSummoner); ok {
			summoner.SummonServitor(cast.Skill)
		}
		return Result{}
	}
	id := cubic.ID(cast.Skill.NpcID)

	if len(cast.Targets) > 1 {
		// Mass-cubic cast: every targeted player except the caster
		// receives the cubic as "given by another player". Mass Summon
		// cubics 1328/1329/1330 declare target PARTY in the reference
		// data, so this branch is exercised by shipped skills.
		result := Result{CubicID: id}
		for _, target := range cast.Targets {
			summoner, ok := target.(cubicSummoner)
			if !ok {
				continue
			}
			givenByOther := !sameObject(cast.Caster, target)
			touched, added := summoner.AddOrRefreshCubic(id, givenByOther)
			if !touched {
				continue
			}
			if givenByOther {
				result.CubicTargets = append(result.CubicTargets, target)
				if added {
					result.CubicAddedTargets = append(result.CubicAddedTargets, target)
				}
				continue
			}
			result.CubicTouched = true
			result.CubicID = id
			result.CubicAdded = added
		}
		return result
	}

	summoner, ok := cast.Caster.(cubicSummoner)
	if !ok {
		return Result{}
	}
	touched, added := summoner.AddOrRefreshCubic(id, false)
	return Result{CubicTouched: touched, CubicID: id, CubicAdded: added}
}
