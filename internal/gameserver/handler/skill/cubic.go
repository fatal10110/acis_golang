package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
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

// cubicHandler applies either branch of a SUMMON-type skill cast: a servitor
// for non-cubic skills, or a cubic refresh for cubic skills.
type cubicHandler struct{}

func (cubicHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeSummon}
}

func (cubicHandler) Use(cast Cast) {
	cubicHandler{}.UseResult(cast)
}

func (cubicHandler) UseResult(cast Cast) Result {
	// A caster dead or playing dead summons nothing and keeps its shot.
	if alikeDead(cast.Caster) {
		return Result{}
	}
	if !cast.Skill.IsCubic {
		// The servitor spawn spends the spiritshot itself, and only when
		// the servitor goes through.
		if summoner, ok := cast.Caster.(servitorSummoner); ok {
			summoner.SummonServitor(cast.Skill)
		}
		return Result{}
	}
	result := addCubics(cast)
	if cast.Caster != nil && cast.Caster.Kind() == actor.KindPlayer {
		dischargeSpiritshot(cast)
	}
	return result
}

// addCubics admits the cast's cubic: on every targeted player for a mass
// cubic, else on the caster.
func addCubics(cast Cast) Result {
	id := cubic.ID(cast.Skill.Extra().NpcID)

	if len(cast.Targets) > 1 {
		// Mass-cubic cast: every targeted player except the caster
		// receives the cubic as "given by another player". Mass Summon
		// cubics 1328/1329/1330 declare target PARTY in the datapack, so
		// this branch is exercised by shipped skills.
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
