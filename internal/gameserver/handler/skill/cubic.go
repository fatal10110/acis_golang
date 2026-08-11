package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
)

// cubicSummoner is the narrow surface a cubic-granting SUMMON skill needs
// from a target: admit or refresh the cubic on that target's own list.
type cubicSummoner interface {
	AddOrRefreshCubic(id cubic.ID, givenByOther bool) (touched, added bool)
}

// cubicHandler applies the cubic branch of a SkillType.SUMMON cast: adding
// or refreshing a cubic on the caster's cubic list, or on every targeted
// player's for a mass-cubic skill. The servitor branch of the same skill
// type (def.IsCubic false) is out of this handler's scope — tracked by
// #960 — and is a silent no-op here, matching how an unregistered skill
// type produces no effect elsewhere in this registry.
type cubicHandler struct{}

func (cubicHandler) Types() []string { return []string{"SUMMON"} }

func (cubicHandler) Use(cast Cast) {
	cubicHandler{}.UseResult(cast)
}

func (cubicHandler) UseResult(cast Cast) Result {
	if !cast.Skill.IsCubic {
		return Result{}
	}
	id := cubic.ID(cast.Skill.NpcID)

	if len(cast.Targets) > 1 {
		// Mass-cubic cast: every targeted player except the caster
		// receives the cubic as "given by another player". No shipped
		// cubic skill in the reference data actually resolves more than
		// one target (every one declares target SELF).
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
